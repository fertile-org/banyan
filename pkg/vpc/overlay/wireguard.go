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
	defaultWGName     = "banyan-wg"
	defaultWGPort     = 51820
	defaultKeepalive  = 25
)

// WireGuardDriver implements OverlayDriver using Linux WireGuard interfaces.
type WireGuardDriver struct {
	bridgeName string
	wgName     string
	listenPort int
	privateKey string // base64
	publicKey  string // base64
	wgOps      WireGuardOps
	linkOps    LinkOperations
	knownPeers map[string]bool // peerSubnet → true
}

// NewWireGuardDriver creates a WireGuardDriver with exec-based operations.
func NewWireGuardDriver(privateKey, publicKey string) *WireGuardDriver {
	return &WireGuardDriver{
		bridgeName: defaultBridge,
		wgName:     defaultWGName,
		listenPort: defaultWGPort,
		privateKey: privateKey,
		publicKey:  publicKey,
		wgOps:      &ExecWireGuardOps{},
		linkOps:    &ExecLinkOps{},
		knownPeers: make(map[string]bool),
	}
}

// NewWireGuardDriverWithOps creates a WireGuardDriver with custom operations (for testing).
func NewWireGuardDriverWithOps(linkOps LinkOperations, wgOps WireGuardOps, privateKey, publicKey string) *WireGuardDriver {
	return &WireGuardDriver{
		bridgeName: defaultBridge,
		wgName:     defaultWGName,
		listenPort: defaultWGPort,
		privateKey: privateKey,
		publicKey:  publicKey,
		wgOps:      wgOps,
		linkOps:    linkOps,
		knownPeers: make(map[string]bool),
	}
}

// Init creates the WireGuard interface, bridge, and sets up networking.
// The WireGuard tunnel interface is always recreated (containers don't connect to it).
// The bridge is preserved if it already exists, since running containers have
// veth pairs attached to it — deleting it would break their networking.
func (d *WireGuardDriver) Init(ctx context.Context, subnet net.IPNet, hostIP net.IP) error {
	// Always recreate the WireGuard tunnel interface (safe — no container veths attached)
	if exists, _ := d.linkOps.LinkExists(d.wgName); exists {
		_ = d.linkOps.DeleteLink(d.wgName)
	}

	// 1. Create WireGuard interface
	if err := d.wgOps.CreateInterface(d.wgName); err != nil {
		return fmt.Errorf("create WireGuard interface: %w", err)
	}

	// 2. Configure WireGuard (private key + listen port)
	if err := d.wgOps.ConfigureInterface(d.wgName, d.privateKey, d.listenPort); err != nil {
		return fmt.Errorf("configure WireGuard interface: %w", err)
	}

	// 3. Create bridge if it doesn't exist (containers connect via bridge;
	//    preserving it keeps existing container veth pairs connected)
	bridgeExists, _ := d.linkOps.LinkExists(d.bridgeName)
	if !bridgeExists {
		if err := d.linkOps.CreateBridge(d.bridgeName); err != nil {
			return fmt.Errorf("create bridge: %w", err)
		}

		// 4. Set bridge MAC to deterministic VTEP MAC
		vtepMAC := DeterministicMAC(subnet)
		if err := d.linkOps.SetLinkAddress(d.bridgeName, vtepMAC); err != nil {
			return fmt.Errorf("set bridge MAC: %w", err)
		}

		// 5. Assign VTEP IP to bridge (gateway IP for this agent's subnet)
		vtepIP := VTEPIP(subnet)
		bridgeAddr := &net.IPNet{
			IP:   vtepIP,
			Mask: subnet.Mask,
		}
		if err := d.linkOps.AddAddress(d.bridgeName, bridgeAddr); err != nil {
			return fmt.Errorf("assign IP to bridge: %w", err)
		}
	}

	// 6. Bring up both interfaces
	if err := d.linkOps.SetLinkUp(d.wgName); err != nil {
		return fmt.Errorf("bring up WireGuard: %w", err)
	}
	if err := d.linkOps.SetLinkUp(d.bridgeName); err != nil {
		return fmt.Errorf("bring up bridge: %w", err)
	}

	return nil
}

// ReconcilePeers configures WireGuard peers and adds routes for their subnets.
// WireGuard handles encryption at L3 — no FDB or ARP entries needed (unlike VXLAN).
func (d *WireGuardDriver) ReconcilePeers(ctx context.Context, peers []Peer) error {
	for _, peer := range peers {
		peerKey := peer.Subnet.String()

		if peer.PublicKey == "" {
			continue
		}

		// Add/update WireGuard peer
		endpoint := fmt.Sprintf("%s:%d", peer.HostIP.String(), d.listenPort)
		if err := d.wgOps.AddPeer(d.wgName, peer.PublicKey, endpoint, []string{peerKey}, defaultKeepalive); err != nil {
			return fmt.Errorf("add WireGuard peer for %s: %w", peerKey, err)
		}

		// Add route: peer's subnet via WireGuard interface
		if err := d.linkOps.AddRoute(peer.Subnet, peer.VTEPIP, d.wgName); err != nil {
			return fmt.Errorf("add route for %s: %w", peerKey, err)
		}

		d.knownPeers[peerKey] = true
	}

	return nil
}

// WriteCNIConfig writes the CNI configuration for nerdctl to use the banyan bridge.
// Same as VXLAN — containers still connect to the bridge via CNI.
func (d *WireGuardDriver) WriteCNIConfig(subnet net.IPNet) error {
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

// Cleanup removes the WireGuard and bridge interfaces.
func (d *WireGuardDriver) Cleanup(ctx context.Context) error {
	// Delete WireGuard interface only — the bridge must be preserved
	// because running containers have veth pairs attached to it.
	// Deleting the bridge would orphan those veths and break networking.
	if exists, _ := d.linkOps.LinkExists(d.wgName); exists {
		if err := d.linkOps.DeleteLink(d.wgName); err != nil {
			return fmt.Errorf("delete WireGuard interface: %w", err)
		}
	}

	return nil
}
