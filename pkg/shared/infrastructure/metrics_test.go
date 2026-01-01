package infrastructure

import (
	"testing"
	"time"
)

func TestNewInMemoryMetrics(t *testing.T) {
	metrics := NewInMemoryMetrics()
	if metrics == nil {
		t.Error("expected non-nil metrics")
	}
}

func TestCounter(t *testing.T) {
	metrics := NewInMemoryMetrics()

	t.Run("increment counter", func(t *testing.T) {
		metrics.Counter("test_counter", 1, L("label1", "value1"))
		metrics.Counter("test_counter", 2, L("label1", "value1"))

		value := metrics.GetCounter("test_counter", L("label1", "value1"))
		if value != 3 {
			t.Errorf("expected counter value 3, got %d", value)
		}
	})

	t.Run("different labels are separate", func(t *testing.T) {
		metrics.Counter("label_counter", 5, L("env", "prod"))
		metrics.Counter("label_counter", 3, L("env", "dev"))

		prodValue := metrics.GetCounter("label_counter", L("env", "prod"))
		devValue := metrics.GetCounter("label_counter", L("env", "dev"))

		if prodValue != 5 {
			t.Errorf("expected prod counter 5, got %d", prodValue)
		}
		if devValue != 3 {
			t.Errorf("expected dev counter 3, got %d", devValue)
		}
	})

	t.Run("non-existent counter returns 0", func(t *testing.T) {
		value := metrics.GetCounter("non_existent")
		if value != 0 {
			t.Errorf("expected 0 for non-existent counter, got %d", value)
		}
	})
}

func TestGauge(t *testing.T) {
	metrics := NewInMemoryMetrics()

	t.Run("set gauge", func(t *testing.T) {
		metrics.Gauge("test_gauge", 42.5, L("type", "memory"))

		value := metrics.GetGauge("test_gauge", L("type", "memory"))
		if value != 42.5 {
			t.Errorf("expected gauge value 42.5, got %f", value)
		}
	})

	t.Run("overwrite gauge", func(t *testing.T) {
		metrics.Gauge("overwrite_gauge", 10)
		metrics.Gauge("overwrite_gauge", 20)

		value := metrics.GetGauge("overwrite_gauge")
		if value != 20 {
			t.Errorf("expected gauge value 20, got %f", value)
		}
	})

	t.Run("non-existent gauge returns 0", func(t *testing.T) {
		value := metrics.GetGauge("non_existent_gauge")
		if value != 0 {
			t.Errorf("expected 0 for non-existent gauge, got %f", value)
		}
	})
}

func TestHistogram(t *testing.T) {
	metrics := NewInMemoryMetrics()

	t.Run("record histogram values", func(t *testing.T) {
		metrics.Histogram("test_histogram", 1.0)
		metrics.Histogram("test_histogram", 2.0)
		metrics.Histogram("test_histogram", 3.0)

		values := metrics.GetHistogram("test_histogram")
		if len(values) != 3 {
			t.Errorf("expected 3 values, got %d", len(values))
		}

		// Check values are recorded
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		if sum != 6.0 {
			t.Errorf("expected sum 6.0, got %f", sum)
		}
	})

	t.Run("histogram with labels", func(t *testing.T) {
		metrics.Histogram("labeled_histogram", 5.0, L("method", "GET"))
		metrics.Histogram("labeled_histogram", 10.0, L("method", "POST"))

		getValues := metrics.GetHistogram("labeled_histogram", L("method", "GET"))
		postValues := metrics.GetHistogram("labeled_histogram", L("method", "POST"))

		if len(getValues) != 1 || getValues[0] != 5.0 {
			t.Errorf("unexpected GET histogram values: %v", getValues)
		}
		if len(postValues) != 1 || postValues[0] != 10.0 {
			t.Errorf("unexpected POST histogram values: %v", postValues)
		}
	})

	t.Run("non-existent histogram returns empty slice", func(t *testing.T) {
		values := metrics.GetHistogram("non_existent_histogram")
		if len(values) != 0 {
			t.Errorf("expected empty slice for non-existent histogram, got %v", values)
		}
	})
}

func TestTimer(t *testing.T) {
	metrics := NewInMemoryMetrics()

	t.Run("record timer", func(t *testing.T) {
		stop := metrics.Timer("test_timer")
		time.Sleep(10 * time.Millisecond)
		stop()

		values := metrics.GetHistogram("test_timer")
		if len(values) != 1 {
			t.Errorf("expected 1 timer value, got %d", len(values))
		}

		// Timer should record duration in seconds, should be at least 10ms
		if values[0] < 0.01 {
			t.Errorf("expected at least 0.01 seconds, got %f", values[0])
		}
	})
}

func TestMetricKeyGeneration(t *testing.T) {
	metrics := NewInMemoryMetrics()

	// Test that different label combinations create different keys
	metrics.Counter("same_name", 1, L("a", "1"), L("b", "2"))
	metrics.Counter("same_name", 10, L("a", "1"), L("b", "3"))

	value1 := metrics.GetCounter("same_name", L("a", "1"), L("b", "2"))
	value2 := metrics.GetCounter("same_name", L("a", "1"), L("b", "3"))

	if value1 != 1 {
		t.Errorf("expected counter value 1, got %d", value1)
	}
	if value2 != 10 {
		t.Errorf("expected counter value 10, got %d", value2)
	}
}

func TestLabel(t *testing.T) {
	label := L("test_key", "test_value")

	if label.Key != "test_key" {
		t.Errorf("expected key 'test_key', got '%s'", label.Key)
	}

	if label.Value != "test_value" {
		t.Errorf("expected value 'test_value', got '%s'", label.Value)
	}
}

func TestLabelConstructors(t *testing.T) {
	tests := []struct {
		name  string
		label Label
		key   string
		value string
	}{
		{"LComponent", LComponent("engine"), "component", "engine"},
		{"LOperation", LOperation("create"), "operation", "create"},
		{"LStatus", LStatus("success"), "status", "success"},
		{"LAgentID", LAgentID("agent-123"), "agent_id", "agent-123"},
		{"LDeploymentID", LDeploymentID("dep-123"), "deployment_id", "dep-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.label.Key != tt.key {
				t.Errorf("expected key %s, got %s", tt.key, tt.label.Key)
			}
			if tt.label.Value != tt.value {
				t.Errorf("expected value %s, got %s", tt.value, tt.label.Value)
			}
		})
	}
}

func TestReset(t *testing.T) {
	metrics := NewInMemoryMetrics()

	metrics.Counter("counter", 5)
	metrics.Gauge("gauge", 10.0)
	metrics.Histogram("histogram", 1.0)

	metrics.Reset()

	if metrics.GetCounter("counter") != 0 {
		t.Error("expected counter to be reset")
	}
	if metrics.GetGauge("gauge") != 0 {
		t.Error("expected gauge to be reset")
	}
	if len(metrics.GetHistogram("histogram")) != 0 {
		t.Error("expected histogram to be reset")
	}
}

func TestMetricConstants(t *testing.T) {
	// Just verify constants are defined
	constants := []string{
		MetricDeploymentsTotal,
		MetricDeploymentsActive,
		MetricTasksTotal,
		MetricAgentsTotal,
		MetricAgentsHealthy,
		MetricContainersTotal,
		MetricContainersRunning,
		MetricHealthChecks,
	}

	for _, c := range constants {
		if c == "" {
			t.Error("metric constant should not be empty")
		}
	}
}

func TestNopMetrics(t *testing.T) {
	metrics := NopMetrics{}

	// All methods should work without panicking
	metrics.Counter("test", 1)
	metrics.Gauge("test", 1.0)
	metrics.Histogram("test", 1.0)

	stop := metrics.Timer("test")
	stop() // Should not panic
}
