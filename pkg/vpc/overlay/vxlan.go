package overlay

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
func (d *VXLANDriver) Init(ctx context.Context, subnet net.IPNet, hostIP net.IP) error {
	// 1. Create VXLAN interface
	if err := d.linkOps.CreateVXLAN(d.vxlanName, d.vni, d.port, hostIP); err != nil {
		return fmt.Errorf("create VXLAN interface: %w", err)
	}

	// 2. Create bridge
	if err := d.linkOps.CreateBridge(d.bridgeName); err != nil {
		return fmt.Errorf("create bridge: %w", err)
	}

	// 3. Attach VXLAN to bridge
	if err := d.linkOps.SetLinkMaster(d.vxlanName, d.bridgeName); err != nil {
		return fmt.Errorf("attach VXLAN to bridge: %w", err)
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

	// 5. Bring up both interfaces
	if err := d.linkOps.SetLinkUp(d.vxlanName); err != nil {
		return fmt.Errorf("bring up VXLAN: %w", err)
	}
	if err := d.linkOps.SetLinkUp(d.bridgeName); err != nil {
		return fmt.Errorf("bring up bridge: %w", err)
	}

	return nil
}

// ReconcilePeers updates FDB, ARP, and route entries for remote peers.
func (d *VXLANDriver) ReconcilePeers(ctx context.Context, peers []Peer) error {
	for _, peer := range peers {
		peerKey := peer.Subnet.String()
		if d.knownPeers[peerKey] {
			continue
		}

		// Add FDB entry: bridge fdb append <peerMAC> dev banyan.1 dst <peerHostIP>
		if err := d.linkOps.AddFDBEntry(peer.MAC, peer.HostIP, d.vxlanName); err != nil {
			return fmt.Errorf("add FDB entry for %s: %w", peerKey, err)
		}

		// Add ARP entry: ip neigh replace <peerVTEPIP> lladdr <peerMAC> dev banyan.1
		if err := d.linkOps.AddARPEntry(peer.VTEPIP, peer.MAC, d.vxlanName); err != nil {
			return fmt.Errorf("add ARP entry for %s: %w", peerKey, err)
		}

		// Add route: ip route replace <peerSubnet> via <peerVTEPIP> dev banyan0
		if err := d.linkOps.AddRoute(peer.Subnet, peer.VTEPIP, d.bridgeName); err != nil {
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

	fmt.Printf("  CNI config written to %s\n", configPath)
	return nil
}

// Cleanup removes the VXLAN and bridge interfaces.
func (d *VXLANDriver) Cleanup(ctx context.Context) error {
	// Delete VXLAN first (it's attached to the bridge)
	if exists, _ := d.linkOps.LinkExists(d.vxlanName); exists {
		if err := d.linkOps.DeleteLink(d.vxlanName); err != nil {
			return fmt.Errorf("delete VXLAN interface: %w", err)
		}
	}

	if exists, _ := d.linkOps.LinkExists(d.bridgeName); exists {
		if err := d.linkOps.DeleteLink(d.bridgeName); err != nil {
			return fmt.Errorf("delete bridge: %w", err)
		}
	}

	return nil
}
