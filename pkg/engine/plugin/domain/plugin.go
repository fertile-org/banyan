// Package domain contains the core domain entities for the Plugin Manager.
package domain

import "time"

// Plugin represents a registered lifecycle plugin.
type Plugin struct {
	Name        string
	Description string
	Type        PluginType
	Hooks       []HookPoint
	Spec        PluginSpec
	Enabled     bool
	Priority    int // Lower = higher priority
	Timeout     time.Duration
	Config      map[string]string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PluginType defines how the plugin is invoked.
type PluginType string

const (
	PluginTypeWebhook PluginType = "webhook" // HTTP POST to URL
	PluginTypeGRPC    PluginType = "grpc"    // gRPC call
	PluginTypeBuiltin PluginType = "builtin" // Built-in Go function
)

// IsValid checks if the plugin type is valid.
func (pt PluginType) IsValid() bool {
	switch pt {
	case PluginTypeWebhook, PluginTypeGRPC, PluginTypeBuiltin:
		return true
	default:
		return false
	}
}

// HookPoint represents a lifecycle hook.
type HookPoint string

const (
	HookPreValidate  HookPoint = "pre-validate"
	HookPostValidate HookPoint = "post-validate"
	HookPreDeploy    HookPoint = "pre-deploy"
	HookPostDeploy   HookPoint = "post-deploy"
	HookPreStop      HookPoint = "pre-stop"
	HookPostStop     HookPoint = "post-stop"
	HookOnFailure    HookPoint = "on-failure"
)

// IsValid checks if the hook point is valid.
func (hp HookPoint) IsValid() bool {
	switch hp {
	case HookPreValidate, HookPostValidate, HookPreDeploy, HookPostDeploy, HookPreStop, HookPostStop, HookOnFailure:
		return true
	default:
		return false
	}
}

// IsValidationHook returns true if this hook is related to validation.
func (hp HookPoint) IsValidationHook() bool {
	return hp == HookPreValidate || hp == HookPostValidate
}

// AllHookPoints returns all valid hook points.
func AllHookPoints() []HookPoint {
	return []HookPoint{
		HookPreValidate,
		HookPostValidate,
		HookPreDeploy,
		HookPostDeploy,
		HookPreStop,
		HookPostStop,
		HookOnFailure,
	}
}

// Validate validates the plugin configuration.
func (p *Plugin) Validate() []string {
	var errors []string

	if p.Name == "" {
		errors = append(errors, "name is required")
	}

	if !p.Type.IsValid() {
		errors = append(errors, "invalid plugin type")
	}

	if len(p.Hooks) == 0 {
		errors = append(errors, "at least one hook is required")
	}

	for _, hook := range p.Hooks {
		if !hook.IsValid() {
			errors = append(errors, "invalid hook: "+string(hook))
		}
	}

	// Validate spec based on type
	specErrors := p.Spec.Validate(p.Type)
	errors = append(errors, specErrors...)

	return errors
}

// HasHook checks if the plugin is registered for a specific hook.
func (p *Plugin) HasHook(hook HookPoint) bool {
	for _, h := range p.Hooks {
		if h == hook {
			return true
		}
	}
	return false
}

// Clone creates a deep copy of the plugin.
func (p *Plugin) Clone() *Plugin {
	clone := *p

	// Deep copy slices
	clone.Hooks = make([]HookPoint, len(p.Hooks))
	copy(clone.Hooks, p.Hooks)

	// Deep copy maps
	if p.Config != nil {
		clone.Config = make(map[string]string, len(p.Config))
		for k, v := range p.Config {
			clone.Config[k] = v
		}
	}

	// Deep copy spec
	clone.Spec = p.Spec.Clone()

	return &clone
}
