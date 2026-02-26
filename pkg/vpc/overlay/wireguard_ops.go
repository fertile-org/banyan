package overlay

// WireGuardOps abstracts WireGuard configuration operations for testability.
type WireGuardOps interface {
	// CreateInterface creates a WireGuard network interface.
	CreateInterface(name string) error
	// ConfigureInterface sets the private key and listen port on the interface.
	ConfigureInterface(name, privateKeyB64 string, port int) error
	// AddPeer adds or updates a WireGuard peer with endpoint and allowed IPs.
	AddPeer(iface, pubKey, endpoint string, allowedIPs []string, keepalive int) error
	// RemovePeer removes a WireGuard peer by public key.
	RemovePeer(iface, pubKey string) error
}
