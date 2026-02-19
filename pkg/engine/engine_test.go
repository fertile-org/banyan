package engine

import (
	"testing"
)

func TestDetermineEngineIP(t *testing.T) {
	t.Run("auto-detects non-loopback IP", func(t *testing.T) {
		ip, err := DetermineEngineIP()
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
}
