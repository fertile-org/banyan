package types

import (
	"fmt"
	"net"
)

// ResolveAgentEngines builds the engine list from agent config.
// Uses the Engines list if set, otherwise falls back to the single-engine
// fields (EngineHost + EnginePort + EngineWGPublicKey).
// Returns the list and the primary endpoint address.
func ResolveAgentEngines(cfg *AgentConfig) ([]EngineEndpoint, string) {
	if len(cfg.Engines) > 0 {
		return cfg.Engines, cfg.Engines[0].Address
	}
	if cfg.EngineWGPublicKey != "" {
		port := cfg.EnginePort
		if port == "" {
			port = "50051"
		}
		addr := cfg.EngineHost + ":" + port
		return []EngineEndpoint{
			{Address: addr, WGPublicKey: cfg.EngineWGPublicKey},
		}, addr
	}
	return nil, ""
}

// ResolveCLIEngines builds the engine list from CLI config.
// Same pattern as ResolveAgentEngines but for CLIConfig.
func ResolveCLIEngines(cfg *CLIConfig) []EngineEndpoint {
	if len(cfg.Engines) > 0 {
		return cfg.Engines
	}
	if cfg.EngineWGPublicKey != "" {
		port := "50051"
		if cfg.EnginePort != "" {
			port = cfg.EnginePort
		}
		host := cfg.EngineHost
		if host == "" {
			host = "localhost"
		}
		return []EngineEndpoint{
			{Address: host + ":" + port, WGPublicKey: cfg.EngineWGPublicKey},
		}
	}
	return nil
}

// BuildGRPCEndpoints converts engine endpoints to gRPC connection addresses.
// When tunnelActive is true, derives WireGuard tunnel IPs from each engine's
// public key. Otherwise, uses the raw addresses directly.
func BuildGRPCEndpoints(engines []EngineEndpoint, tunnelActive bool) []string {
	var endpoints []string
	for _, eng := range engines {
		if tunnelActive {
			_, port, splitErr := net.SplitHostPort(eng.Address)
			if splitErr != nil {
				continue
			}
			tunnelIP := TunnelIPFromPublicKey(eng.WGPublicKey)
			endpoints = append(endpoints, tunnelIP.String()+":"+port)
		} else {
			endpoints = append(endpoints, eng.Address)
		}
	}
	return endpoints
}

// ValidateEngineStartConfig validates engine configuration before starting.
// Checks whitelisted keys (unless allowInsecure) and multi-engine prerequisites.
func ValidateEngineStartConfig(cfg *EngineConfig, allowInsecure bool) error {
	if !allowInsecure {
		keysDir := cfg.WhitelistedKeysDir
		if keysDir == "" {
			keysDir = DefaultWhitelistedKeysDir
		}
		keys, _ := LoadWhitelistedKeys(keysDir)
		if len(keys) == 0 {
			return fmt.Errorf("no whitelisted client keys found in %s\n"+
				"  Add client keys with: sudo banyan-engine add-client --name <name> --pubkey <key>\n"+
				"  Or use --allow-insecure for development only (NOT for production)", keysDir)
		}
	}

	if cfg.MultiEngine {
		if cfg.ManagedEtcd {
			return fmt.Errorf("multi-engine mode requires external etcd (set managed_etcd: false and provide store_address)")
		}
		if cfg.ManagedRegistry || cfg.ExternalRegistryURL == "" {
			return fmt.Errorf("multi-engine mode requires external registry (set managed_registry: false and provide external_registry_url)")
		}
	}

	return nil
}
