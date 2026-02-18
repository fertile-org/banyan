package cmd

import (
	"testing"
)

func TestDetermineEngineIP(t *testing.T) {
	original := engineEtcdClientURLs
	t.Cleanup(func() { engineEtcdClientURLs = original })

	t.Run("explicit IP returns that IP", func(t *testing.T) {
		engineEtcdClientURLs = "http://192.168.1.10:2379"
		ip, err := determineEngineIP()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip != "192.168.1.10" {
			t.Errorf("expected 192.168.1.10, got %s", ip)
		}
	})

	t.Run("localhost returns localhost", func(t *testing.T) {
		engineEtcdClientURLs = "http://localhost:2379"
		ip, err := determineEngineIP()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip != "localhost" {
			t.Errorf("expected localhost, got %s", ip)
		}
	})

	t.Run("0.0.0.0 auto-detects non-loopback IP", func(t *testing.T) {
		engineEtcdClientURLs = "http://0.0.0.0:2379"
		ip, err := determineEngineIP()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip == "" {
			t.Error("expected non-empty IP")
		}
		if ip == "127.0.0.1" || ip == "0.0.0.0" {
			t.Errorf("expected non-loopback IP, got %s", ip)
		}
	})

	t.Run("invalid URL returns error", func(t *testing.T) {
		engineEtcdClientURLs = "://invalid"
		_, err := determineEngineIP()
		if err == nil {
			t.Error("expected error for invalid URL")
		}
	})
}
