// Package domain defines the core entities for the Compose Parser.
package domain

// ParsedBanyan represents a parsed banyan.yaml extension file.
type ParsedBanyan struct {
	Version   string                      `yaml:"version"`
	VPC       VPCExtension                `yaml:"vpc"`
	Services  map[string]ServiceExtension `yaml:"services"`
	Placement PlacementConfig             `yaml:"placement"`
	Scaling   ScalingConfig               `yaml:"scaling"`
	Plugins   []PluginConfig              `yaml:"plugins"`
}

// VPCExtension contains VPC-specific configurations.
type VPCExtension struct {
	Name    string          `yaml:"name"`
	CIDR    string          `yaml:"cidr"`
	Subnets []SubnetConfig  `yaml:"subnets"`
	DNS     DNSConfig       `yaml:"dns"`
	Peering []PeeringConfig `yaml:"peering"`
	NAT     *NATConfig      `yaml:"nat"`
}

// SubnetConfig defines a subnet within the VPC.
type SubnetConfig struct {
	Name       string `yaml:"name"`
	CIDR       string `yaml:"cidr"`
	Zone       string `yaml:"zone"`
	Public     bool   `yaml:"public"`
	NATGateway bool   `yaml:"nat_gateway"`
}

// DNSConfig defines DNS configuration.
type DNSConfig struct {
	Enabled bool     `yaml:"enabled"`
	Servers []string `yaml:"servers"`
	Domain  string   `yaml:"domain"`
}

// PeeringConfig defines VPC peering configuration.
type PeeringConfig struct {
	Name   string `yaml:"name"`
	VPCID  string `yaml:"vpc_id"`
	Region string `yaml:"region"`
}

// NATConfig defines NAT gateway configuration.
type NATConfig struct {
	Enabled bool   `yaml:"enabled"`
	Subnet  string `yaml:"subnet"`
}

// ServiceExtension contains Banyan-specific service configurations.
type ServiceExtension struct {
	Placement     ServicePlacement `yaml:"placement"`
	Scaling       ServiceScaling   `yaml:"scaling"`
	SecurityGroup string           `yaml:"security_group"`
	Subnet        string           `yaml:"subnet"`
	GPU           *GPUConfig       `yaml:"gpu"`
	Storage       *StorageConfig   `yaml:"storage"`
}

// ServicePlacement defines placement constraints.
type ServicePlacement struct {
	Constraints []string              `yaml:"constraints"`
	Preferences []PlacementPreference `yaml:"preferences"`
	NodeLabels  map[string]string     `yaml:"node_labels"`
}

// PlacementPreference defines placement preference.
type PlacementPreference struct {
	Spread string `yaml:"spread"`
}

// ServiceScaling defines auto-scaling configuration.
type ServiceScaling struct {
	Min       int    `yaml:"min"`
	Max       int    `yaml:"max"`
	TargetCPU int    `yaml:"target_cpu"`
	TargetMem int    `yaml:"target_mem"`
	Cooldown  string `yaml:"cooldown"`
}

// GPUConfig defines GPU configuration.
type GPUConfig struct {
	Count int    `yaml:"count"`
	Type  string `yaml:"type"`
}

// StorageConfig defines storage configuration.
type StorageConfig struct {
	Type string `yaml:"type"`
	Size string `yaml:"size"`
	IOPS int    `yaml:"iops"`
}

// PlacementConfig defines global placement configuration.
type PlacementConfig struct {
	DefaultZone   string            `yaml:"default_zone"`
	SpreadPolicy  string            `yaml:"spread_policy"`
	NodeSelectors map[string]string `yaml:"node_selectors"`
}

// ScalingConfig defines global scaling configuration.
type ScalingConfig struct {
	Enabled      bool   `yaml:"enabled"`
	MinInstances int    `yaml:"min_instances"`
	MaxInstances int    `yaml:"max_instances"`
	MetricServer string `yaml:"metric_server"`
}

// PluginConfig defines a plugin to be applied.
type PluginConfig struct {
	Name    string                 `yaml:"name"`
	Version string                 `yaml:"version"`
	Config  map[string]interface{} `yaml:"config"`
	When    string                 `yaml:"when"`
}
