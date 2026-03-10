package types

import (
	"testing"
)

func TestParseMemoryBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint64
		wantErr bool
	}{
		{"megabytes", "512m", 512 * 1024 * 1024, false},
		{"gigabytes", "1g", 1024 * 1024 * 1024, false},
		{"kilobytes", "256k", 256 * 1024, false},
		{"plain bytes", "1048576", 1048576, false},
		{"fractional gigabytes", "0.5g", 512 * 1024 * 1024, false},
		{"fractional megabytes", "1.5m", 1572864, false},
		{"uppercase", "512M", 512 * 1024 * 1024, false},
		{"with b suffix", "1024b", 1024, false},
		{"zero", "0", 0, false},
		{"empty", "", 0, true},
		{"invalid", "abc", 0, true},
		{"negative", "-512m", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMemoryBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMemoryBytes(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseMemoryBytes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseCPU(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{"integer", "2", 2.0, false},
		{"fractional", "0.5", 0.5, false},
		{"one", "1.0", 1.0, false},
		{"quarter", "0.25", 0.25, false},
		{"zero", "0", 0, false},
		{"empty", "", 0, true},
		{"invalid", "abc", 0, true},
		{"negative", "-1", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCPU(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCPU(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseCPU(%q) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

func TestServiceResourceRequest(t *testing.T) {
	tests := []struct {
		name       string
		svc        ServiceRecord
		wantMemory uint64
		wantCPU    float64
	}{
		{
			name:       "defaults when nothing specified",
			svc:        ServiceRecord{},
			wantMemory: DefaultMemoryRequest,
			wantCPU:    DefaultCPURequest,
		},
		{
			name:       "uses memory limit",
			svc:        ServiceRecord{MemoryLimit: "256m"},
			wantMemory: 256 * 1024 * 1024,
			wantCPU:    DefaultCPURequest,
		},
		{
			name:       "uses cpu limit",
			svc:        ServiceRecord{CPULimit: "0.5"},
			wantMemory: DefaultMemoryRequest,
			wantCPU:    0.5,
		},
		{
			name:       "reservation overrides limit",
			svc:        ServiceRecord{MemoryLimit: "1g", MemoryReservation: "256m"},
			wantMemory: 256 * 1024 * 1024,
			wantCPU:    DefaultCPURequest,
		},
		{
			name:       "all specified",
			svc:        ServiceRecord{MemoryLimit: "1g", CPULimit: "2", MemoryReservation: "512m"},
			wantMemory: 512 * 1024 * 1024,
			wantCPU:    2.0,
		},
		{
			name:       "invalid memory falls back to default",
			svc:        ServiceRecord{MemoryLimit: "invalid"},
			wantMemory: DefaultMemoryRequest,
			wantCPU:    DefaultCPURequest,
		},
		{
			name:       "invalid cpu falls back to default",
			svc:        ServiceRecord{CPULimit: "invalid"},
			wantMemory: DefaultMemoryRequest,
			wantCPU:    DefaultCPURequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ServiceResourceRequest(tt.svc)
			if req.MemoryBytes != tt.wantMemory {
				t.Errorf("MemoryBytes = %d, want %d", req.MemoryBytes, tt.wantMemory)
			}
			if req.CPUCores != tt.wantCPU {
				t.Errorf("CPUCores = %f, want %f", req.CPUCores, tt.wantCPU)
			}
		})
	}
}
