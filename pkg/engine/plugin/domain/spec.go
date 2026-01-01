// Package domain contains the core domain entities for the Plugin Manager.
package domain

// PluginSpec contains type-specific configuration.
type PluginSpec struct {
	// Webhook spec
	WebhookURL     string            `json:"webhook_url,omitempty" yaml:"webhook_url,omitempty"`
	WebhookHeaders map[string]string `json:"webhook_headers,omitempty" yaml:"webhook_headers,omitempty"`
	WebhookMethod  string            `json:"webhook_method,omitempty" yaml:"webhook_method,omitempty"` // Default: POST

	// gRPC spec
	GRPCAddress string `json:"grpc_address,omitempty" yaml:"grpc_address,omitempty"`
	GRPCMethod  string `json:"grpc_method,omitempty" yaml:"grpc_method,omitempty"`

	// Builtin spec
	BuiltinName string `json:"builtin_name,omitempty" yaml:"builtin_name,omitempty"`
}

// Validate validates the spec based on plugin type.
func (s *PluginSpec) Validate(pluginType PluginType) []string {
	var errors []string

	switch pluginType {
	case PluginTypeWebhook:
		if s.WebhookURL == "" {
			errors = append(errors, "webhook_url is required for webhook type")
		}
	case PluginTypeGRPC:
		if s.GRPCAddress == "" {
			errors = append(errors, "grpc_address is required for grpc type")
		}
		if s.GRPCMethod == "" {
			errors = append(errors, "grpc_method is required for grpc type")
		}
	case PluginTypeBuiltin:
		if s.BuiltinName == "" {
			errors = append(errors, "builtin_name is required for builtin type")
		}
	}

	return errors
}

// Clone creates a deep copy of the spec.
func (s *PluginSpec) Clone() PluginSpec {
	clone := *s

	if s.WebhookHeaders != nil {
		clone.WebhookHeaders = make(map[string]string, len(s.WebhookHeaders))
		for k, v := range s.WebhookHeaders {
			clone.WebhookHeaders[k] = v
		}
	}

	return clone
}
