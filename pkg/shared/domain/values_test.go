package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTimestamp(t *testing.T) {
	t.Run("Now", func(t *testing.T) {
		ts := Now()
		if ts.IsZero() {
			t.Error("Now() should not return zero timestamp")
		}
	})

	t.Run("Before and After", func(t *testing.T) {
		ts1 := Now()
		time.Sleep(time.Millisecond)
		ts2 := Now()

		if !ts1.Before(ts2) {
			t.Error("ts1 should be before ts2")
		}

		if !ts2.After(ts1) {
			t.Error("ts2 should be after ts1")
		}
	})

	t.Run("Add and Sub", func(t *testing.T) {
		ts := Now()
		ts2 := ts.Add(time.Hour)

		diff := ts2.Sub(ts)
		if diff != time.Hour {
			t.Errorf("expected 1 hour difference, got %v", diff)
		}
	})

	t.Run("JSON marshaling", func(t *testing.T) {
		ts := TimestampFromTime(time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC))

		data, err := json.Marshal(ts)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		var ts2 Timestamp
		if err := json.Unmarshal(data, &ts2); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if !ts.Time().Equal(ts2.Time()) {
			t.Errorf("timestamps don't match: %v != %v", ts, ts2)
		}
	})
}

func TestDuration(t *testing.T) {
	t.Run("from string", func(t *testing.T) {
		d, err := DurationFromString("5m30s")
		if err != nil {
			t.Fatalf("failed to parse: %v", err)
		}

		expected := 5*time.Minute + 30*time.Second
		if d.Duration() != expected {
			t.Errorf("expected %v, got %v", expected, d.Duration())
		}
	})

	t.Run("JSON marshaling string", func(t *testing.T) {
		d := Duration(5 * time.Minute)

		data, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		var d2 Duration
		if err := json.Unmarshal(data, &d2); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if d != d2 {
			t.Errorf("durations don't match: %v != %v", d, d2)
		}
	})

	t.Run("JSON unmarshaling number", func(t *testing.T) {
		data := []byte("300000000000") // 5 minutes in nanoseconds

		var d Duration
		if err := json.Unmarshal(data, &d); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		expected := Duration(5 * time.Minute)
		if d != expected {
			t.Errorf("expected %v, got %v", expected, d)
		}
	})
}

func TestVersion(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		v := NewVersion(1, 2, 3)
		if v.String() != "1.2.3" {
			t.Errorf("expected 1.2.3, got %s", v.String())
		}

		v.Label = "beta"
		if v.String() != "1.2.3-beta" {
			t.Errorf("expected 1.2.3-beta, got %s", v.String())
		}
	})

	t.Run("Compare", func(t *testing.T) {
		tests := []struct {
			v1, v2   Version
			expected int
		}{
			{NewVersion(1, 0, 0), NewVersion(1, 0, 0), 0},
			{NewVersion(1, 0, 0), NewVersion(2, 0, 0), -1},
			{NewVersion(2, 0, 0), NewVersion(1, 0, 0), 1},
			{NewVersion(1, 1, 0), NewVersion(1, 2, 0), -1},
			{NewVersion(1, 1, 1), NewVersion(1, 1, 2), -1},
		}

		for _, tt := range tests {
			if got := tt.v1.Compare(tt.v2); got != tt.expected {
				t.Errorf("%s.Compare(%s) = %d, want %d", tt.v1, tt.v2, got, tt.expected)
			}
		}
	})
}

func TestResourceLimits(t *testing.T) {
	t.Run("IsEmpty", func(t *testing.T) {
		empty := ResourceLimits{}
		if !empty.IsEmpty() {
			t.Error("expected empty limits")
		}

		nonEmpty := ResourceLimits{CPUMillis: 1000}
		if nonEmpty.IsEmpty() {
			t.Error("expected non-empty limits")
		}
	})

	t.Run("Validate", func(t *testing.T) {
		valid := ResourceLimits{CPUMillis: 1000, MemoryBytes: 1024 * 1024}
		if err := valid.Validate(); err != nil {
			t.Errorf("valid limits should not error: %v", err)
		}

		invalid := ResourceLimits{CPUMillis: -1}
		if err := invalid.Validate(); err == nil {
			t.Error("negative CPU should be invalid")
		}
	})
}

func TestLabels(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		labels := Labels{"app": "test", "env": "dev"}

		if !labels.Has("app") {
			t.Error("expected to have 'app' label")
		}

		if labels.Has("nonexistent") {
			t.Error("expected not to have 'nonexistent' label")
		}

		if labels.Get("app") != "test" {
			t.Error("expected 'app' to be 'test'")
		}

		if labels.Get("nonexistent") != "" {
			t.Error("expected empty string for missing label")
		}
	})

	t.Run("Set", func(t *testing.T) {
		labels := Labels{}
		labels.Set("key", "value")
		if labels.Get("key") != "value" {
			t.Error("Set should set the value")
		}
	})

	t.Run("Merge", func(t *testing.T) {
		l1 := Labels{"a": "1", "b": "2"}
		l2 := Labels{"b": "3", "c": "4"}

		merged := l1.Merge(l2)

		if merged.Get("a") != "1" {
			t.Error("expected 'a' from l1")
		}
		if merged.Get("b") != "3" {
			t.Error("expected 'b' to be overwritten by l2")
		}
		if merged.Get("c") != "4" {
			t.Error("expected 'c' from l2")
		}
	})

	t.Run("Matches", func(t *testing.T) {
		labels := Labels{"app": "test", "env": "dev", "version": "1.0"}

		if !labels.Matches(Labels{"app": "test"}) {
			t.Error("should match single selector")
		}

		if !labels.Matches(Labels{"app": "test", "env": "dev"}) {
			t.Error("should match multiple selectors")
		}

		if labels.Matches(Labels{"app": "other"}) {
			t.Error("should not match different value")
		}

		if labels.Matches(Labels{"nonexistent": "value"}) {
			t.Error("should not match missing label")
		}
	})
}

func TestRetryPolicy(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		p := DefaultRetryPolicy()

		if p.MaxAttempts != 3 {
			t.Errorf("expected MaxAttempts=3, got %d", p.MaxAttempts)
		}
		if p.BackoffFactor != 2.0 {
			t.Errorf("expected BackoffFactor=2.0, got %f", p.BackoffFactor)
		}
	})

	t.Run("Validate", func(t *testing.T) {
		valid := DefaultRetryPolicy()
		if err := valid.Validate(); err != nil {
			t.Errorf("valid policy should not error: %v", err)
		}

		invalid := RetryPolicy{MaxAttempts: 0}
		if err := invalid.Validate(); err == nil {
			t.Error("MaxAttempts=0 should be invalid")
		}

		invalid2 := RetryPolicy{MaxAttempts: 1, BackoffFactor: 0.5}
		if err := invalid2.Validate(); err == nil {
			t.Error("BackoffFactor<1 should be invalid")
		}
	})
}

func TestTimeWindow(t *testing.T) {
	start := TimestampFromTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	end := TimestampFromTime(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	window := TimeWindow{Start: start, End: end}

	t.Run("Contains", func(t *testing.T) {
		inside := TimestampFromTime(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
		if !window.Contains(inside) {
			t.Error("window should contain timestamp inside")
		}

		outside := TimestampFromTime(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC))
		if window.Contains(outside) {
			t.Error("window should not contain timestamp outside")
		}
	})

	t.Run("Duration", func(t *testing.T) {
		expected := 24 * time.Hour
		if window.Duration() != expected {
			t.Errorf("expected duration %v, got %v", expected, window.Duration())
		}
	})

	t.Run("Overlaps", func(t *testing.T) {
		// Overlapping window
		w2Start := TimestampFromTime(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
		w2End := TimestampFromTime(time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC))
		w2 := TimeWindow{Start: w2Start, End: w2End}

		if !window.Overlaps(w2) {
			t.Error("windows should overlap")
		}

		// Non-overlapping window
		w3Start := TimestampFromTime(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC))
		w3End := TimestampFromTime(time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC))
		w3 := TimeWindow{Start: w3Start, End: w3End}

		if window.Overlaps(w3) {
			t.Error("windows should not overlap")
		}
	})
}
