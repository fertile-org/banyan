package overlay

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/fertile-org/banyan/pkg/logging"
)

const (
	defaultVNI       = 1
	defaultVXLANPort = 8472
	defaultBridge    = "banyan0"
	defaultVXLAN     = "banyan.1"
	cniConfigDir     = "/etc/cni/net.d"
	cniConfigFile    = "10-banyan.conf"
)

// LinkOperations abstracts netlink/exec operations for testability.
type LinkOperations interface {
	CreateVXLAN(name string, vni int, port int, srcIP net.IP) error
	CreateBridge(name string) error
	SetLinkMaster(slave, master string) error
	SetLinkUp(name string) error
	SetLinkAddress(name string, mac net.HardwareAddr) error
	AddAddress(name string, addr *net.IPNet) error
	AddRoute(dst net.IPNet, gw net.IP, dev string) error
	AddFDBEntry(mac net.HardwareAddr, ip net.IP, dev string) error
	AddARPEntry(ip net.IP, mac net.HardwareAddr, dev string) error
	DeleteLink(name string) error
	LinkExists(name string) (bool, error)
}

// VXLANDriver implements OverlayDriver using Linux VXLAN interfaces.
type VXLANDriver struct {
	vni        int
	port       int
	bridgeName string
	vxlanName  string
	linkOps    LinkOperations
	// Track known peers to avoid duplicate FDB/ARP entries
	knownPeers map[string]bool // peerSubnet → true
}

// NewVXLANDriver creates a VXLANDriver with the default exec-based LinkOperations.
func NewVXLANDriver() *VXLANDriver {
	return &VXLANDriver{
		vni:        defaultVNI,
		port:       defaultVXLANPort,
		bridgeName: defaultBridge,
		vxlanName:  defaultVXLAN,
		linkOps:    &ExecLinkOps{},
		knownPeers: make(map[string]bool),
	}
}

// NewVXLANDriverWithOps creates a VXLANDriver with custom LinkOperations (for testing).
func NewVXLANDriverWithOps(ops LinkOperations) *VXLANDriver {
	return &VXLANDriver{
		vni:        defaultVNI,
		port:       defaultVXLANPort,
		bridgeName: defaultBridge,
		vxlanName:  defaultVXLAN,
		linkOps:    ops,
		knownPeers: make(map[string]bool),
	}
}

// Init creates the VXLAN interface, bridge, and sets up networking.
// The VXLAN tunnel interface is always recreated (containers don't connect to it).
// The bridge is preserved if it already exists, since running containers have
// veth pairs attached to it — deleting it would break their networking.
func (d *VXLANDriver) Init(ctx context.Context, subnet net.IPNet, hostIP net.IP) error {
	// Always recreate the VXLAN tunnel interface (safe — no container veths attached)
	if exists, _ := d.linkOps.LinkExists(d.vxlanName); exists {
		_ = d.linkOps.DeleteLink(d.vxlanName)
	}

	// 1. Create VXLAN interface
	if err := d.linkOps.CreateVXLAN(d.vxlanName, d.vni, d.port, hostIP); err != nil {
		return fmt.Errorf("create VXLAN interface: %w", err)
	}

	// 2. Create bridge if it doesn't exist (containers connect via bridge;
	//    preserving it keeps existing container veth pairs connected)
	bridgeExists, _ := d.linkOps.LinkExists(d.bridgeName)
	if !bridgeExists {
		if err := d.linkOps.CreateBridge(d.bridgeName); err != nil {
			return fmt.Errorf("create bridge: %w", err)
		}

		// 3. Set bridge MAC to deterministic VTEP MAC so the bridge recognizes
		// incoming VXLAN-decapsulated frames (whose dst MAC is the VTEP MAC)
		// as "for me" and delivers them to the IP stack for L3 forwarding.
		// Without this, the bridge floods unknown-unicast frames but never
		// passes them up to the kernel for routing to containers.
		vtepMAC := DeterministicMAC(subnet)
		if err := d.linkOps.SetLinkAddress(d.bridgeName, vtepMAC); err != nil {
			return fmt.Errorf("set bridge MAC: %w", err)
		}

		// 4. Assign VTEP IP to bridge (gateway IP for this agent's subnet)
		vtepIP := VTEPIP(subnet)
		bridgeAddr := &net.IPNet{
			IP:   vtepIP,
			Mask: subnet.Mask,
		}
		if err := d.linkOps.AddAddress(d.bridgeName, bridgeAddr); err != nil {
			return fmt.Errorf("assign IP to bridge: %w", err)
		}
	}

	// 5. Attach VXLAN to bridge (always needed after VXLAN recreation)
	if err := d.linkOps.SetLinkMaster(d.vxlanName, d.bridgeName); err != nil {
		return fmt.Errorf("attach VXLAN to bridge: %w", err)
	}

	// 6. Bring up both interfaces
	if err := d.linkOps.SetLinkUp(d.vxlanName); err != nil {
		return fmt.Errorf("bring up VXLAN: %w", err)
	}
	if err := d.linkOps.SetLinkUp(d.bridgeName); err != nil {
		return fmt.Errorf("bring up bridge: %w", err)
	}

	return nil
}

// ReconcilePeers updates FDB, ARP, and route entries for remote peers.
// All operations are idempotent (replace/append semantics), so we always
// re-apply them to recover from external modifications (e.g., CNI bridge
// plugin resetting routes during container creation).
func (d *VXLANDriver) ReconcilePeers(ctx context.Context, peers []Peer) error {
	for _, peer := range peers {
		peerKey := peer.Subnet.String()

		// Add FDB entry: bridge fdb append <peerMAC> dev banyan.1 dst <peerHostIP>
		if err := d.linkOps.AddFDBEntry(peer.MAC, peer.HostIP, d.vxlanName); err != nil {
			return fmt.Errorf("add FDB entry for %s: %w", peerKey, err)
		}

		// Add ARP entry: ip neigh replace <peerVTEPIP> lladdr <peerMAC> dev banyan.1
		if err := d.linkOps.AddARPEntry(peer.VTEPIP, peer.MAC, d.vxlanName); err != nil {
			return fmt.Errorf("add ARP entry for %s: %w", peerKey, err)
		}

		// Add route: ip route replace <peerSubnet> via <peerVTEPIP> dev banyan.1 onlink
		// Route via the VXLAN interface (not bridge) so ARP resolves against static
		// neighbor entries on banyan.1. This follows standard VXLAN overlay practice.
		if err := d.linkOps.AddRoute(peer.Subnet, peer.VTEPIP, d.vxlanName); err != nil {
			return fmt.Errorf("add route for %s: %w", peerKey, err)
		}

		d.knownPeers[peerKey] = true
	}

	return nil
}

// cniConfigData is the CNI configuration for the banyan bridge network.
type cniConfigData struct {
	CNIVersion string      `json:"cniVersion"`
	Name       string      `json:"name"`
	Type       string      `json:"type"`
	Bridge     string      `json:"bridge"`
	IsGateway  bool        `json:"isGateway"`
	IPMasq     bool        `json:"ipMasq"`
	HairpinMode bool       `json:"hairpinMode"`
	IPAM       cniIPAMData `json:"ipam"`
}

type cniIPAMData struct {
	Type   string         `json:"type"`
	Subnet string         `json:"subnet"`
	Routes []cniRouteData `json:"routes"`
}

type cniRouteData struct {
	Dst string `json:"dst"`
}

// WriteCNIConfig writes the CNI configuration for nerdctl to use the banyan bridge.
func (d *VXLANDriver) WriteCNIConfig(subnet net.IPNet) error {
	config := cniConfigData{
		CNIVersion:  "0.3.1",
		Name:        "banyan",
		Type:        "bridge",
		Bridge:      d.bridgeName,
		IsGateway:   true,
		IPMasq:      false,
		HairpinMode: true,
		IPAM: cniIPAMData{
			Type:   "host-local",
			Subnet: subnet.String(),
			Routes: []cniRouteData{{Dst: "0.0.0.0/0"}},
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal CNI config: %w", err)
	}

	if mkErr := os.MkdirAll(cniConfigDir, 0o755); mkErr != nil {
		return fmt.Errorf("create CNI config dir: %w", mkErr)
	}

	configPath := filepath.Join(cniConfigDir, cniConfigFile)
	if writeErr := os.WriteFile(configPath, data, 0o644); writeErr != nil {
		return fmt.Errorf("write CNI config to %s: %w", configPath, writeErr)
	}

	logging.Info("CNI config written", "path", configPath)
	return nil
}

// Cleanup removes the VXLAN and bridge interfaces.
func (d *VXLANDriver) Cleanup(ctx context.Context) error {
	// Delete VXLAN interface only — the bridge must be preserved
	// because running containers have veth pairs attached to it.
	// Deleting the bridge would orphan those veths and break networking.
	if exists, _ := d.linkOps.LinkExists(d.vxlanName); exists {
		if err := d.linkOps.DeleteLink(d.vxlanName); err != nil {
			return fmt.Errorf("delete VXLAN interface: %w", err)
		}
	}

	return nil
}
