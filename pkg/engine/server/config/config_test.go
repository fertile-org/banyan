package config

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Address != ":50052" {
		t.Errorf("Expected address ':50052', got '%s'", cfg.Address)
	}
	if cfg.MaxRecvMsgSize != 16*1024*1024 {
		t.Errorf("Expected MaxRecvMsgSize 16MB, got %d", cfg.MaxRecvMsgSize)
	}
	if cfg.MaxSendMsgSize != 16*1024*1024 {
		t.Errorf("Expected MaxSendMsgSize 16MB, got %d", cfg.MaxSendMsgSize)
	}
	if cfg.KeepAliveTime != 30*time.Second {
		t.Errorf("Expected KeepAliveTime 30s, got %v", cfg.KeepAliveTime)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		config *ServerConfig
		want   *ServerConfig
	}{
		{
			name:   "empty address",
			config: &ServerConfig{Address: ""},
			want:   &ServerConfig{Address: ":50052", MaxRecvMsgSize: 16 * 1024 * 1024, MaxSendMsgSize: 16 * 1024 * 1024, KeepAliveTime: 30 * time.Second},
		},
		{
			name:   "zero msg sizes",
			config: &ServerConfig{Address: ":8080", MaxRecvMsgSize: 0, MaxSendMsgSize: 0},
			want:   &ServerConfig{Address: ":8080", MaxRecvMsgSize: 16 * 1024 * 1024, MaxSendMsgSize: 16 * 1024 * 1024, KeepAliveTime: 30 * time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			if tt.config.Address != tt.want.Address {
				t.Errorf("Address = %s, want %s", tt.config.Address, tt.want.Address)
			}
			if tt.config.MaxRecvMsgSize != tt.want.MaxRecvMsgSize {
				t.Errorf("MaxRecvMsgSize = %d, want %d", tt.config.MaxRecvMsgSize, tt.want.MaxRecvMsgSize)
			}
		})
	}
}

func TestWithMethods(t *testing.T) {
	cfg := DefaultConfig()

	cfg.WithAddress(":9090").
		WithTLS("cert.pem", "key.pem", "ca.pem").
		WithReflection(true)

	if cfg.Address != ":9090" {
		t.Errorf("Expected address ':9090', got '%s'", cfg.Address)
	}
	if cfg.TLSCertFile != "cert.pem" {
		t.Errorf("Expected TLSCertFile 'cert.pem', got '%s'", cfg.TLSCertFile)
	}
	if cfg.TLSKeyFile != "key.pem" {
		t.Errorf("Expected TLSKeyFile 'key.pem', got '%s'", cfg.TLSKeyFile)
	}
	if cfg.TLSCAFile != "ca.pem" {
		t.Errorf("Expected TLSCAFile 'ca.pem', got '%s'", cfg.TLSCAFile)
	}
	if !cfg.EnableReflection {
		t.Error("Expected EnableReflection to be true")
	}
}
