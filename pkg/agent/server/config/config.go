package config

import "time"

// ServerConfig holds the configuration for the agent server.
type ServerConfig struct {
	Address         string        // Listen address (e.g., ":50051")
	TLSCertFile     string        // Path to TLS certificate file
	TLSKeyFile      string        // Path to TLS key file
	MaxRecvMsgSize  int           // Maximum receive message size in bytes
	MaxSendMsgSize  int           // Maximum send message size in bytes
	KeepAliveTime   time.Duration // Connection keep-alive time
	ShutdownTimeout time.Duration // Graceful shutdown timeout
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Address:         ":50051",
		MaxRecvMsgSize:  4 * 1024 * 1024, // 4MB
		MaxSendMsgSize:  4 * 1024 * 1024, // 4MB
		KeepAliveTime:   30 * time.Second,
		ShutdownTimeout: 30 * time.Second,
	}
}
