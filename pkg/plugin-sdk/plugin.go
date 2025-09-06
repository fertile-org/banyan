package sdk

import (
	"github.com/fertile-org/banyan/pkg/interfaces"
)

// PluginSDK provides utilities for building Banyan plugins
type PluginSDK struct {
	name    string
	version string
}

// NewPlugin creates a new plugin SDK instance
func NewPlugin(name, version string) *PluginSDK {
	return &PluginSDK{
		name:    name,
		version: version,
	}
}

// Name returns the plugin name
func (p *PluginSDK) Name() string {
	return p.name
}

// Version returns the plugin version
func (p *PluginSDK) Version() string {
	return p.version
}

// ValidateConfig provides common validation for deployment configs
func ValidateConfig(config interfaces.DeploymentConfig) error {
	if config.ID == "" {
		return interfaces.ErrMissingDeploymentID
	}
	if config.Name == "" {
		return interfaces.ErrMissingDeploymentName
	}
	return nil
}
