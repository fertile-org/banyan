package overlay

import "net"

const (
	defaultBridge = "banyan0"
	cniConfigDir  = "/etc/cni/net.d"
	cniConfigFile = "10-banyan.conf"
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

// cniConfigData is the CNI configuration for the banyan bridge network.
type cniConfigData struct {
	CNIVersion  string      `json:"cniVersion"`
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Bridge      string      `json:"bridge"`
	IsGateway   bool        `json:"isGateway"`
	IPMasq      bool        `json:"ipMasq"`
	HairpinMode bool        `json:"hairpinMode"`
	IPAM        cniIPAMData `json:"ipam"`
}

type cniIPAMData struct {
	Type   string         `json:"type"`
	Subnet string         `json:"subnet"`
	Routes []cniRouteData `json:"routes"`
}

type cniRouteData struct {
	Dst string `json:"dst"`
}
