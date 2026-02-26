package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultMaxEvents is the default number of events to retain in the WAL.
const DefaultMaxEvents = 10000

// Event represents a cluster lifecycle event.
type Event struct {
	Timestamp time.Time
	Type      string // "agent.connected", "deployment.started", etc.
	Message   string
	Severity  string // "info", "warning", "error"
}

// EventLog is the interface for recording and querying cluster events.
// EventBuffer (in-memory) and EventStore (WAL-backed) both implement it.
type EventLog interface {
	Add(Event)
	Recent(n int) []Event
}

// walEvent is the on-disk JSON representation of an event.
type walEvent struct {
	Timestamp time.Time `json:"ts"`
	Type      string    `json:"type"`
	Message   string    `json:"msg"`
	Severity  string    `json:"sev"`
}

// EventStore persists cluster events to a WAL (write-ahead log) file.
// Events are stored as newline-delimited JSON for durability.
// An in-memory cache provides fast Recent() reads.
// Auto-compaction triggers at 2x maxEvents to bound disk usage.
type EventStore struct {
	mu        sync.RWMutex
	file      *os.File
	path      string
	events    []Event
	maxEvents int
}

// NewEventStore opens or creates a WAL file at path and loads existing events.
// maxEvents controls the compaction threshold — the store keeps at most maxEvents
// in memory and compacts the WAL when it exceeds 2x that limit.
func NewEventStore(path string, maxEvents int) (*EventStore, error) {
	if maxEvents <= 0 {
		maxEvents = DefaultMaxEvents
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create event WAL directory: %w", err)
	}

	s := &EventStore{
		path:      path,
		maxEvents: maxEvents,
	}

	// Load existing events from file
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("load event WAL: %w", err)
	}

	// Open file for appending new events
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open event WAL: %w", err)
	}
	s.file = f

	return s, nil
}

// load reads existing events from the WAL file into the in-memory cache.
// Corrupt lines are silently skipped.
func (s *EventStore) load() error {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var we walEvent
		if json.Unmarshal(scanner.Bytes(), &we) == nil {
			s.events = append(s.events, Event{
				Timestamp: we.Timestamp,
				Type:      we.Type,
				Message:   we.Message,
				Severity:  we.Severity,
			})
		}
	}

	// Trim to maxEvents if file had more (startup compaction)
	if len(s.events) > s.maxEvents {
		s.events = s.events[len(s.events)-s.maxEvents:]
	}

	return scanner.Err()
}

// Add appends an event to the WAL and in-memory cache.
// Triggers compaction when the cache exceeds 2x maxEvents.
func (s *EventStore) Add(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, e)

	// Write to WAL
	if s.file != nil {
		we := walEvent{
			Timestamp: e.Timestamp,
			Type:      e.Type,
			Message:   e.Message,
			Severity:  e.Severity,
		}
		if data, err := json.Marshal(we); err == nil {
			s.file.Write(append(data, '\n'))
		}
	}

	// Auto-compact when exceeding 2x max
	if len(s.events) > s.maxEvents*2 {
		s.compact()
	}
}

// Recent returns the most recent n events, newest first.
func (s *EventStore) Recent(n int) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.events)
	if n > total {
		n = total
	}
	if n == 0 {
		return nil
	}

	result := make([]Event, n)
	for i := range n {
		result[i] = s.events[total-1-i]
	}
	return result
}

// Close syncs and closes the WAL file.
func (s *EventStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

// compact rewrites the WAL file keeping only the most recent maxEvents.
// Must be called with s.mu held.
func (s *EventStore) compact() {
	if len(s.events) > s.maxEvents {
		s.events = s.events[len(s.events)-s.maxEvents:]
	}

	if s.file == nil {
		return
	}

	// Close current file and rewrite
	s.file.Close()

	f, err := os.Create(s.path)
	if err != nil {
		log.Printf("WARNING: failed to compact event WAL: %v", err)
		// Reopen in append mode to continue writing
		s.file, _ = os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		return
	}

	for i := range s.events {
		we := walEvent{
			Timestamp: s.events[i].Timestamp,
			Type:      s.events[i].Type,
			Message:   s.events[i].Message,
			Severity:  s.events[i].Severity,
		}
		if data, err := json.Marshal(we); err == nil {
			f.Write(append(data, '\n'))
		}
	}

	s.file = f
}

// EventBuffer is a fixed-size in-memory ring buffer of recent events.
// It implements EventLog and is primarily used for testing.
type EventBuffer struct {
	mu     sync.RWMutex
	events []Event
	pos    int  // next write position
	full   bool // true once buffer has wrapped around
	size   int
}

// NewEventBuffer creates an event buffer that holds at most size events.
func NewEventBuffer(size int) *EventBuffer {
	return &EventBuffer{
		events: make([]Event, size),
		size:   size,
	}
}

// Add appends an event to the buffer, overwriting the oldest if full.
func (b *EventBuffer) Add(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.events[b.pos] = e
	b.pos++
	if b.pos >= b.size {
		b.pos = 0
		b.full = true
	}
}

// Recent returns the most recent n events, newest first.
// Returns fewer than n if the buffer contains fewer events.
func (b *EventBuffer) Recent(n int) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	total := b.pos
	if b.full {
		total = b.size
	}
	if n > total {
		n = total
	}
	if n == 0 {
		return nil
	}

	result := make([]Event, n)
	for i := range n {
		// Walk backwards from the most recent entry
		idx := b.pos - 1 - i
		if idx < 0 {
			idx += b.size
		}
		result[i] = b.events[idx]
	}
	return result
}
