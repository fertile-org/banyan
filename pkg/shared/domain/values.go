package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Timestamp represents a point in time with timezone awareness.
type Timestamp time.Time

// Now returns the current timestamp.
func Now() Timestamp {
	return Timestamp(time.Now().UTC())
}

// TimestampFromTime creates a Timestamp from a time.Time.
func TimestampFromTime(t time.Time) Timestamp {
	return Timestamp(t.UTC())
}

// Time returns the underlying time.Time.
func (t Timestamp) Time() time.Time {
	return time.Time(t)
}

// IsZero reports whether t represents the zero time instant.
func (t Timestamp) IsZero() bool {
	return time.Time(t).IsZero()
}

// Before reports whether t is before u.
func (t Timestamp) Before(u Timestamp) bool {
	return time.Time(t).Before(time.Time(u))
}

// After reports whether t is after u.
func (t Timestamp) After(u Timestamp) bool {
	return time.Time(t).After(time.Time(u))
}

// Add returns t + d.
func (t Timestamp) Add(d time.Duration) Timestamp {
	return Timestamp(time.Time(t).Add(d))
}

// Sub returns the duration t-u.
func (t Timestamp) Sub(u Timestamp) time.Duration {
	return time.Time(t).Sub(time.Time(u))
}

// String returns RFC3339 formatted string.
func (t Timestamp) String() string {
	return time.Time(t).Format(time.RFC3339)
}

// MarshalJSON implements json.Marshaler.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(t).Format(time.RFC3339))
}

// UnmarshalJSON implements json.Unmarshaler.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}
	*t = Timestamp(parsed)
	return nil
}

// Duration wraps time.Duration for consistent JSON serialization.
type Duration time.Duration

// DurationFromString parses a duration string.
func DurationFromString(s string) (Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return Duration(d), nil
}

// Duration returns the underlying time.Duration.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

func (d Duration) String() string {
	return time.Duration(d).String()
}

// MarshalJSON implements json.Marshaler.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// Try unmarshaling as number (nanoseconds)
		var ns int64
		if numErr := json.Unmarshal(data, &ns); numErr != nil {
			return err
		}
		*d = Duration(time.Duration(ns))
		return nil
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

// Version represents a semantic version.
type Version struct {
	Label string `json:"label,omitempty"`
	Major int    `json:"major"`
	Minor int    `json:"minor"`
	Patch int    `json:"patch"`
}

// NewVersion creates a new version.
func NewVersion(major, minor, patch int) Version {
	return Version{Major: major, Minor: minor, Patch: patch}
}

// String returns the string representation.
func (v Version) String() string {
	if v.Label != "" {
		return fmt.Sprintf("%d.%d.%d-%s", v.Major, v.Minor, v.Patch, v.Label)
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare returns -1 if v < other, 0 if v == other, 1 if v > other.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	return 0
}

// ResourceLimits defines resource constraints.
type ResourceLimits struct {
	CPUMillis   int64 `json:"cpu_millis,omitempty"`   // CPU in millicores (1000 = 1 CPU)
	MemoryBytes int64 `json:"memory_bytes,omitempty"` // Memory in bytes
	DiskBytes   int64 `json:"disk_bytes,omitempty"`   // Disk in bytes
}

// IsEmpty returns true if no limits are set.
func (r ResourceLimits) IsEmpty() bool {
	return r.CPUMillis == 0 && r.MemoryBytes == 0 && r.DiskBytes == 0
}

// Validate checks if the resource limits are valid.
func (r ResourceLimits) Validate() error {
	if r.CPUMillis < 0 {
		return NewValidationError("ResourceLimits").AddFieldError("cpu_millis", "must be non-negative")
	}
	if r.MemoryBytes < 0 {
		return NewValidationError("ResourceLimits").AddFieldError("memory_bytes", "must be non-negative")
	}
	if r.DiskBytes < 0 {
		return NewValidationError("ResourceLimits").AddFieldError("disk_bytes", "must be non-negative")
	}
	return nil
}

// Labels represents key-value metadata.
type Labels map[string]string

// Has checks if a label exists.
func (l Labels) Has(key string) bool {
	_, ok := l[key]
	return ok
}

// Get returns a label value or empty string.
func (l Labels) Get(key string) string {
	return l[key]
}

// Set sets a label value.
func (l Labels) Set(key, value string) {
	l[key] = value
}

// Merge merges another Labels into this one (other takes precedence).
func (l Labels) Merge(other Labels) Labels {
	result := make(Labels)
	for k, v := range l {
		result[k] = v
	}
	for k, v := range other {
		result[k] = v
	}
	return result
}

// Matches returns true if all selector labels match.
func (l Labels) Matches(selector Labels) bool {
	for k, v := range selector {
		if l[k] != v {
			return false
		}
	}
	return true
}

// Annotations represents metadata annotations (typically longer values).
type Annotations map[string]string

// RetryPolicy defines retry behavior for operations.
type RetryPolicy struct {
	RetryableErrors []string `json:"retryable_errors,omitempty"`
	MaxAttempts     int      `json:"max_attempts"`
	InitialDelay    Duration `json:"initial_delay"`
	MaxDelay        Duration `json:"max_delay"`
	BackoffFactor   float64  `json:"backoff_factor"`
}

// DefaultRetryPolicy returns a sensible default retry policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:   3,
		InitialDelay:  Duration(time.Second),
		MaxDelay:      Duration(30 * time.Second),
		BackoffFactor: 2.0,
	}
}

// Validate checks if the retry policy is valid.
func (r RetryPolicy) Validate() error {
	v := NewValidationError("RetryPolicy")
	if r.MaxAttempts < 1 {
		_ = v.AddFieldError("max_attempts", "must be at least 1")
	}
	if r.InitialDelay < 0 {
		_ = v.AddFieldError("initial_delay", "must be non-negative")
	}
	if r.MaxDelay < 0 {
		_ = v.AddFieldError("max_delay", "must be non-negative")
	}
	if r.BackoffFactor < 1.0 {
		_ = v.AddFieldError("backoff_factor", "must be at least 1.0")
	}
	if v.HasErrors() {
		return v
	}
	return nil
}

// TimeWindow represents a time range.
type TimeWindow struct {
	Start Timestamp `json:"start"`
	End   Timestamp `json:"end"`
}

// Contains checks if a timestamp falls within the window.
func (w TimeWindow) Contains(t Timestamp) bool {
	return !t.Before(w.Start) && !t.After(w.End)
}

// Duration returns the duration of the window.
func (w TimeWindow) Duration() time.Duration {
	return w.End.Sub(w.Start)
}

// Overlaps checks if two windows overlap.
func (w TimeWindow) Overlaps(other TimeWindow) bool {
	return w.Start.Before(other.End) && other.Start.Before(w.End)
}
