package overlay

import "net"

const (
	defaultBridge = "banyan0"
	cniConfigDir  = "/etc/cni/net.d"
	cniConfigFile = "10-banyan.conflist"
)

// LinkOperations abstracts netlink/exec operations for testability.
type LinkOperations interface {
	CreateBridge(name string) error
	SetLinkUp(name string) error
	SetLinkAddress(name string, mac net.HardwareAddr) error
	AddAddress(name string, addr *net.IPNet) error
	AddRoute(dst net.IPNet, gw net.IP, dev string) error
	DeleteLink(name string) error
	LinkExists(name string) (bool, error)
}

// cniConflistData is the CNI conflist configuration for the banyan bridge network.
type cniConflistData struct {
	CNIVersion string            `json:"cniVersion"`
	Name       string            `json:"name"`
	Plugins    []cniPluginConfig `json:"plugins"`
}

type cniPluginConfig struct {
	Type        string      `json:"type"`
	Bridge      string      `json:"bridge,omitempty"`
	IsGateway   bool        `json:"isGateway,omitempty"`
	IPMasq      bool        `json:"ipMasq"`
	HairpinMode bool        `json:"hairpinMode,omitempty"`
	IPAM        *cniIPAMData `json:"ipam,omitempty"`
}

type cniIPAMData struct {
	Type   string         `json:"type"`
	Subnet string         `json:"subnet"`
	Routes []cniRouteData `json:"routes"`
}

type cniRouteData struct {
	Dst string `json:"dst"`
}
