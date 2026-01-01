package config

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if cfg.Address != ":50051" {
		t.Errorf("expected Address :50051, got %s", cfg.Address)
	}

	expectedMaxSize := 4 * 1024 * 1024
	if cfg.MaxRecvMsgSize != expectedMaxSize {
		t.Errorf("expected MaxRecvMsgSize %d, got %d", expectedMaxSize, cfg.MaxRecvMsgSize)
	}

	if cfg.MaxSendMsgSize != expectedMaxSize {
		t.Errorf("expected MaxSendMsgSize %d, got %d", expectedMaxSize, cfg.MaxSendMsgSize)
	}

	expectedKeepAlive := 30 * time.Second
	if cfg.KeepAliveTime != expectedKeepAlive {
		t.Errorf("expected KeepAliveTime %v, got %v", expectedKeepAlive, cfg.KeepAliveTime)
	}

	expectedShutdownTimeout := 30 * time.Second
	if cfg.ShutdownTimeout != expectedShutdownTimeout {
		t.Errorf("expected ShutdownTimeout %v, got %v", expectedShutdownTimeout, cfg.ShutdownTimeout)
	}

	// TLS should be disabled by default
	if cfg.TLSCertFile != "" {
		t.Errorf("expected TLSCertFile empty, got %s", cfg.TLSCertFile)
	}

	if cfg.TLSKeyFile != "" {
		t.Errorf("expected TLSKeyFile empty, got %s", cfg.TLSKeyFile)
	}
}

func TestServerConfig_CustomValues(t *testing.T) {
	cfg := &ServerConfig{
		Address:         ":8080",
		TLSCertFile:     "/path/to/cert.pem",
		TLSKeyFile:      "/path/to/key.pem",
		MaxRecvMsgSize:  10 * 1024 * 1024,
		MaxSendMsgSize:  10 * 1024 * 1024,
		KeepAliveTime:   60 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}

	if cfg.Address != ":8080" {
		t.Errorf("expected Address :8080, got %s", cfg.Address)
	}

	if cfg.TLSCertFile != "/path/to/cert.pem" {
		t.Errorf("expected TLSCertFile /path/to/cert.pem, got %s", cfg.TLSCertFile)
	}

	if cfg.TLSKeyFile != "/path/to/key.pem" {
		t.Errorf("expected TLSKeyFile /path/to/key.pem, got %s", cfg.TLSKeyFile)
	}
}
