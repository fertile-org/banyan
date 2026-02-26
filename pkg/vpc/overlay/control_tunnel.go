package overlay

import (
	"fmt"
	"net"
)

const (
	controlTunnelCIDR = 16
	controlKeepalive  = 25
)

// SetupControlTunnel creates and configures a WireGuard control tunnel interface
// for control plane encryption. The interface is assigned myIP with a /16 mask.
// If the interface already exists, it is removed and recreated.
func SetupControlTunnel(wgOps WireGuardOps, linkOps LinkOperations, iface, privateKey string, myIP net.IP, listenPort int) error {
	// Clean up any existing interface from a previous init
	if exists, _ := linkOps.LinkExists(iface); exists {
		_ = linkOps.DeleteLink(iface)
	}

	// Create WireGuard interface
	if err := wgOps.CreateInterface(iface); err != nil {
		return fmt.Errorf("create %s interface: %w", iface, err)
	}

	// Configure private key and listen port
	if err := wgOps.ConfigureInterface(iface, privateKey, listenPort); err != nil {
		return fmt.Errorf("configure %s: %w", iface, err)
	}

	// Assign tunnel IP with /16 mask
	addr := &net.IPNet{
		IP:   myIP,
		Mask: net.CIDRMask(controlTunnelCIDR, 32),
	}
	if err := linkOps.AddAddress(iface, addr); err != nil {
		return fmt.Errorf("assign IP to %s: %w", iface, err)
	}

	// Bring up the interface
	if err := linkOps.SetLinkUp(iface); err != nil {
		return fmt.Errorf("bring up %s: %w", iface, err)
	}

	return nil
}

// AddControlPeer adds a peer to the specified control tunnel interface.
// Endpoint may be empty (engine learns agent endpoints from incoming packets).
func AddControlPeer(wgOps WireGuardOps, iface, pubKey, endpoint string, tunnelIP net.IP) error {
	allowedIPs := []string{tunnelIP.String() + "/32"}
	if err := wgOps.AddPeer(iface, pubKey, endpoint, allowedIPs, controlKeepalive); err != nil {
		return fmt.Errorf("add control peer %s: %w", pubKey[:8], err)
	}
	return nil
}

// CleanupControlTunnel removes the specified control tunnel interface.
func CleanupControlTunnel(linkOps LinkOperations, iface string) error {
	exists, err := linkOps.LinkExists(iface)
	if err != nil {
		return fmt.Errorf("check %s exists: %w", iface, err)
	}
	if !exists {
		return nil
	}
	if err := linkOps.DeleteLink(iface); err != nil {
		return fmt.Errorf("delete %s: %w", iface, err)
	}
	return nil
}

// SetupControlTunnelExec is a convenience wrapper using exec-based operations.
func SetupControlTunnelExec(iface, privateKey string, myIP net.IP, listenPort int) error {
	return SetupControlTunnel(&ExecWireGuardOps{}, &ExecLinkOps{}, iface, privateKey, myIP, listenPort)
}

// AddControlPeerExec is a convenience wrapper using exec-based operations.
func AddControlPeerExec(iface, pubKey, endpoint string, tunnelIP net.IP) error {
	return AddControlPeer(&ExecWireGuardOps{}, iface, pubKey, endpoint, tunnelIP)
}

// CleanupControlTunnelExec is a convenience wrapper using exec-based operations.
func CleanupControlTunnelExec(iface string) error {
	return CleanupControlTunnel(&ExecLinkOps{}, iface)
}

// ControlTunnelExists checks if the specified control tunnel interface exists.
func ControlTunnelExists(iface string) bool {
	linkOps := &ExecLinkOps{}
	exists, err := linkOps.LinkExists(iface)
	return err == nil && exists
}
