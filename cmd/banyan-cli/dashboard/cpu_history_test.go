package dashboard

import (
	"testing"
	"time"
)

func TestUpdateCPUHistory_AccumulatesValues(t *testing.T) {
	m := New(nil, 5*time.Second)

	// First refresh
	m.data = &DashboardData{
		Containers: []ContainerData{
			{Name: "web-0", CPUPercent: 25.0},
			{Name: "db-0", CPUPercent: 10.0},
		},
	}
	m.updateCPUHistory()

	if len(m.cpuHistory["web-0"]) != 1 || m.cpuHistory["web-0"][0] != 25.0 {
		t.Errorf("web-0 history after 1st refresh = %v, want [25.0]", m.cpuHistory["web-0"])
	}

	// Second refresh
	m.data.Containers[0].CPUPercent = 30.0
	m.data.Containers[1].CPUPercent = 15.0
	m.updateCPUHistory()

	if len(m.cpuHistory["web-0"]) != 2 {
		t.Errorf("web-0 history length = %d, want 2", len(m.cpuHistory["web-0"]))
	}
	if m.cpuHistory["web-0"][1] != 30.0 {
		t.Errorf("web-0 history[1] = %f, want 30.0", m.cpuHistory["web-0"][1])
	}
}

func TestUpdateCPUHistory_CapsAtMax(t *testing.T) {
	m := New(nil, 5*time.Second)

	// Fill beyond maxCPUHistory
	for i := 0; i < maxCPUHistory+5; i++ {
		m.data = &DashboardData{
			Containers: []ContainerData{
				{Name: "web-0", CPUPercent: float64(i)},
			},
		}
		m.updateCPUHistory()
	}

	if len(m.cpuHistory["web-0"]) != maxCPUHistory {
		t.Errorf("history length = %d, want %d", len(m.cpuHistory["web-0"]), maxCPUHistory)
	}
	// Should keep the latest values (5, 6, ... 24)
	first := m.cpuHistory["web-0"][0]
	if first != 5.0 {
		t.Errorf("oldest value = %f, want 5.0", first)
	}
}

func TestUpdateCPUHistory_PrunesRemovedContainers(t *testing.T) {
	m := New(nil, 5*time.Second)

	// First refresh with two containers
	m.data = &DashboardData{
		Containers: []ContainerData{
			{Name: "web-0", CPUPercent: 25.0},
			{Name: "db-0", CPUPercent: 10.0},
		},
	}
	m.updateCPUHistory()

	if len(m.cpuHistory) != 2 {
		t.Errorf("history should have 2 entries, got %d", len(m.cpuHistory))
	}

	// Second refresh with only one container
	m.data = &DashboardData{
		Containers: []ContainerData{
			{Name: "web-0", CPUPercent: 30.0},
		},
	}
	m.updateCPUHistory()

	if _, exists := m.cpuHistory["db-0"]; exists {
		t.Error("db-0 should be pruned from history")
	}
	if len(m.cpuHistory) != 1 {
		t.Errorf("history should have 1 entry, got %d", len(m.cpuHistory))
	}
}

func TestUpdateCPUHistory_NilData(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.updateCPUHistory() // should not panic
	if m.cpuHistory != nil {
		t.Error("nil data should not create history map")
	}
}

func TestGetCPUHistory(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.cpuHistory = map[string][]float64{
		"web-0": {10, 20, 30},
	}

	got := m.getCPUHistory("web-0")
	if len(got) != 3 {
		t.Errorf("getCPUHistory length = %d, want 3", len(got))
	}

	got = m.getCPUHistory("nonexistent")
	if got != nil {
		t.Error("getCPUHistory for missing key should return nil")
	}
}
