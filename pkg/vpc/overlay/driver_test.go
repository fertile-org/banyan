package overlay

import (
	"net"
	"testing"
)

func TestDeterministicMAC(t *testing.T) {
	tests := []struct {
		name     string
		subnet   string
		expected string
	}{
		{
			name:     "10.0.45.0/24",
			subnet:   "10.0.45.0/24",
			expected: "02:42:0a:00:2d:01",
		},
		{
			name:     "10.0.0.0/24",
			subnet:   "10.0.0.0/24",
			expected: "02:42:0a:00:00:01",
		},
		{
			name:     "10.0.255.0/24",
			subnet:   "10.0.255.0/24",
			expected: "02:42:0a:00:ff:01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, subnet, _ := net.ParseCIDR(tt.subnet)
			mac := DeterministicMAC(*subnet)
			if mac.String() != tt.expected {
				t.Errorf("expected MAC %s, got %s", tt.expected, mac.String())
			}
		})
	}
}

func TestVTEPIP(t *testing.T) {
	tests := []struct {
		subnet   string
		expected string
	}{
		{"10.0.45.0/24", "10.0.45.1"},
		{"10.0.0.0/24", "10.0.0.1"},
		{"172.16.1.0/24", "172.16.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.subnet, func(t *testing.T) {
			_, subnet, _ := net.ParseCIDR(tt.subnet)
			vtep := VTEPIP(*subnet)
			if vtep.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, vtep.String())
			}
		})
	}
}
