package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- EventBuffer tests (in-memory ring buffer) ---

func TestEventBuffer_AddAndRecent(t *testing.T) {
	buf := NewEventBuffer(5)

	// Empty buffer
	if got := buf.Recent(10); len(got) != 0 {
		t.Errorf("empty buffer: got %d events, want 0", len(got))
	}

	// Add 3 events
	buf.Add(Event{Timestamp: time.Unix(1, 0), Type: "a", Message: "event-1"})
	buf.Add(Event{Timestamp: time.Unix(2, 0), Type: "b", Message: "event-2"})
	buf.Add(Event{Timestamp: time.Unix(3, 0), Type: "c", Message: "event-3"})

	events := buf.Recent(10)
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	// Newest first
	if events[0].Message != "event-3" {
		t.Errorf("events[0] = %q, want event-3", events[0].Message)
	}
	if events[2].Message != "event-1" {
		t.Errorf("events[2] = %q, want event-1", events[2].Message)
	}
}

func TestEventBuffer_LimitN(t *testing.T) {
	buf := NewEventBuffer(10)

	for i := range 5 {
		buf.Add(Event{Message: string(rune('a' + i))})
	}

	// Request fewer than available
	events := buf.Recent(2)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Message != "e" {
		t.Errorf("events[0] = %q, want e", events[0].Message)
	}
	if events[1].Message != "d" {
		t.Errorf("events[1] = %q, want d", events[1].Message)
	}
}

func TestEventBuffer_Wraparound(t *testing.T) {
	buf := NewEventBuffer(3)

	// Add 5 events into a buffer of size 3 — first 2 are overwritten
	buf.Add(Event{Message: "old-1"})
	buf.Add(Event{Message: "old-2"})
	buf.Add(Event{Message: "keep-3"})
	buf.Add(Event{Message: "keep-4"})
	buf.Add(Event{Message: "keep-5"})

	events := buf.Recent(10)
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	if events[0].Message != "keep-5" {
		t.Errorf("events[0] = %q, want keep-5", events[0].Message)
	}
	if events[1].Message != "keep-4" {
		t.Errorf("events[1] = %q, want keep-4", events[1].Message)
	}
	if events[2].Message != "keep-3" {
		t.Errorf("events[2] = %q, want keep-3", events[2].Message)
	}
}

func TestEventBuffer_ZeroRecent(t *testing.T) {
	buf := NewEventBuffer(5)
	buf.Add(Event{Message: "x"})
	if got := buf.Recent(0); len(got) != 0 {
		t.Errorf("Recent(0) returned %d events", len(got))
	}
}

// --- EventStore tests (WAL-backed) ---

func TestEventStore_AddAndRecent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.wal")

	store, err := NewEventStore(path, 100)
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	defer store.Close()

	// Empty store
	if got := store.Recent(10); len(got) != 0 {
		t.Errorf("empty store: got %d events, want 0", len(got))
	}

	// Add events
	store.Add(Event{Timestamp: time.Unix(1, 0), Type: "a", Message: "event-1", Severity: "info"})
	store.Add(Event{Timestamp: time.Unix(2, 0), Type: "b", Message: "event-2", Severity: "info"})
	store.Add(Event{Timestamp: time.Unix(3, 0), Type: "c", Message: "event-3", Severity: "warning"})

	events := store.Recent(10)
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	// Newest first
	if events[0].Message != "event-3" {
		t.Errorf("events[0] = %q, want event-3", events[0].Message)
	}
	if events[0].Severity != "warning" {
		t.Errorf("events[0].Severity = %q, want warning", events[0].Severity)
	}
	if events[2].Message != "event-1" {
		t.Errorf("events[2] = %q, want event-1", events[2].Message)
	}

	// Verify WAL file was written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Error("WAL file is empty")
	}
}

func TestEventStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.wal")

	// Write events
	store1, err := NewEventStore(path, 100)
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	store1.Add(Event{Timestamp: time.Unix(1, 0), Type: "deploy", Message: "deployed app", Severity: "info"})
	store1.Add(Event{Timestamp: time.Unix(2, 0), Type: "agent", Message: "agent connected", Severity: "info"})
	store1.Close()

	// Reopen and verify events survived
	store2, err := NewEventStore(path, 100)
	if err != nil {
		t.Fatalf("NewEventStore reopen: %v", err)
	}
	defer store2.Close()

	events := store2.Recent(10)
	if len(events) != 2 {
		t.Fatalf("after reopen: got %d events, want 2", len(events))
	}
	if events[0].Message != "agent connected" {
		t.Errorf("events[0] = %q, want 'agent connected'", events[0].Message)
	}
	if events[1].Message != "deployed app" {
		t.Errorf("events[1] = %q, want 'deployed app'", events[1].Message)
	}
}

func TestEventStore_Compaction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.wal")

	// maxEvents=5 → compaction triggers at 10 events
	store, err := NewEventStore(path, 5)
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	defer store.Close()

	// Add 11 events to trigger compaction (exceeds 2*5=10)
	for i := range 11 {
		store.Add(Event{
			Timestamp: time.Unix(int64(i), 0),
			Type:      "test",
			Message:   string(rune('a' + i)),
		})
	}

	// After compaction, should have exactly maxEvents (5)
	events := store.Recent(100)
	if len(events) != 5 {
		t.Fatalf("after compaction: got %d events, want 5", len(events))
	}
	// Most recent event should be 'k' (index 10 = 'a'+10)
	if events[0].Message != string(rune('a'+10)) {
		t.Errorf("events[0] = %q, want %q", events[0].Message, string(rune('a'+10)))
	}

	// Verify WAL file was rewritten (reopen and check)
	store.Close()
	store2, err := NewEventStore(path, 5)
	if err != nil {
		t.Fatalf("NewEventStore reopen: %v", err)
	}
	defer store2.Close()

	events2 := store2.Recent(100)
	if len(events2) != 5 {
		t.Fatalf("after reopen: got %d events, want 5", len(events2))
	}
}

func TestEventStore_StartupTrim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.wal")

	// Write 20 events with maxEvents=100 (no compaction)
	store1, err := NewEventStore(path, 100)
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	for i := range 20 {
		store1.Add(Event{
			Timestamp: time.Unix(int64(i), 0),
			Type:      "test",
			Message:   string(rune('a' + i)),
		})
	}
	store1.Close()

	// Reopen with maxEvents=5 — should trim to 5 most recent on load
	store2, err := NewEventStore(path, 5)
	if err != nil {
		t.Fatalf("NewEventStore reopen: %v", err)
	}
	defer store2.Close()

	events := store2.Recent(100)
	if len(events) != 5 {
		t.Fatalf("after startup trim: got %d events, want 5", len(events))
	}
}

func TestEventStore_LimitRecent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.wal")

	store, err := NewEventStore(path, 100)
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	defer store.Close()

	for i := range 10 {
		store.Add(Event{Message: string(rune('a' + i))})
	}

	events := store.Recent(3)
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	if events[0].Message != "j" {
		t.Errorf("events[0] = %q, want j", events[0].Message)
	}
}

func TestEventStore_ZeroRecent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.wal")

	store, err := NewEventStore(path, 100)
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	defer store.Close()

	store.Add(Event{Message: "x"})
	if got := store.Recent(0); len(got) != 0 {
		t.Errorf("Recent(0) returned %d events", len(got))
	}
}

func TestEventStore_DefaultMaxEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.wal")

	// maxEvents=0 should use default
	store, err := NewEventStore(path, 0)
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	defer store.Close()

	if store.maxEvents != DefaultMaxEvents {
		t.Errorf("maxEvents = %d, want %d", store.maxEvents, DefaultMaxEvents)
	}
}

func TestEventStore_CorruptLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.wal")

	// Write a mix of valid and corrupt lines
	data := `{"ts":"2026-01-01T00:00:00Z","type":"good","msg":"valid event","sev":"info"}
not json at all
{"ts":"2026-01-02T00:00:00Z","type":"good","msg":"second valid","sev":"warning"}
`
	os.WriteFile(path, []byte(data), 0o600)

	store, err := NewEventStore(path, 100)
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	defer store.Close()

	events := store.Recent(10)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2 (corrupt line skipped)", len(events))
	}
	if events[0].Message != "second valid" {
		t.Errorf("events[0] = %q, want 'second valid'", events[0].Message)
	}
}

func TestEventStore_NonexistentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "events.wal")

	store, err := NewEventStore(path, 100)
	if err != nil {
		t.Fatalf("NewEventStore should create directories: %v", err)
	}
	defer store.Close()

	store.Add(Event{Message: "test"})
	events := store.Recent(1)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
}

// --- EventLog interface compliance ---

func TestEventLogInterface(t *testing.T) {
	// Verify both types implement EventLog
	var _ EventLog = (*EventBuffer)(nil)
	var _ EventLog = (*EventStore)(nil)
}
