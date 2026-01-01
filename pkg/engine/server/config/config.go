package config

import (
	"time"
)

// ServerConfig holds engine gRPC server configuration.
type ServerConfig struct {
	Address          string
	TLSCertFile      string
	TLSKeyFile       string
	TLSCAFile        string
	MaxRecvMsgSize   int
	MaxSendMsgSize   int
	KeepAliveTime    time.Duration
	EnableReflection bool
}

// DefaultConfig returns the default server configuration.
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Address:          ":50052",         // Different from agent's :50051
		MaxRecvMsgSize:   16 * 1024 * 1024, // 16MB
		MaxSendMsgSize:   16 * 1024 * 1024, // 16MB
		KeepAliveTime:    30 * time.Second,
		EnableReflection: false,
	}
}

// Validate validates the configuration.
func (c *ServerConfig) Validate() error {
	if c.Address == "" {
		c.Address = ":50052"
	}
	if c.MaxRecvMsgSize <= 0 {
		c.MaxRecvMsgSize = 16 * 1024 * 1024
	}
	if c.MaxSendMsgSize <= 0 {
		c.MaxSendMsgSize = 16 * 1024 * 1024
	}
	if c.KeepAliveTime <= 0 {
		c.KeepAliveTime = 30 * time.Second
	}
	return nil
}

// WithAddress sets the listen address.
func (c *ServerConfig) WithAddress(addr string) *ServerConfig {
	c.Address = addr
	return c
}

// WithTLS sets TLS configuration.
func (c *ServerConfig) WithTLS(certFile, keyFile, caFile string) *ServerConfig {
	c.TLSCertFile = certFile
	c.TLSKeyFile = keyFile
	c.TLSCAFile = caFile
	return c
}

// WithReflection enables/disables gRPC reflection.
func (c *ServerConfig) WithReflection(enabled bool) *ServerConfig {
	c.EnableReflection = enabled
	return c
}
