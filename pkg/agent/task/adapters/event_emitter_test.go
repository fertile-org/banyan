package adapters

import (
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/task/domain"
)

func TestMemoryEventEmitter_SubscribeAndEmit(t *testing.T) {
	emitter := NewMemoryEventEmitterAdapter()

	ch := emitter.Subscribe()

	event := domain.TaskEvent{
		TaskID:    "task-1",
		EventType: domain.TaskEventStarted,
		Timestamp: time.Now(),
	}

	emitter.Emit(event)

	select {
	case received := <-ch:
		if received.TaskID != event.TaskID {
			t.Errorf("expected task ID %s, got %s", event.TaskID, received.TaskID)
		}
		if received.EventType != event.EventType {
			t.Errorf("expected event type %s, got %s", event.EventType, received.EventType)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestMemoryEventEmitter_MultipleSubscribers(t *testing.T) {
	emitter := NewMemoryEventEmitterAdapter()

	ch1 := emitter.Subscribe()
	ch2 := emitter.Subscribe()

	event := domain.TaskEvent{
		TaskID:    "task-1",
		EventType: domain.TaskEventCompleted,
	}

	emitter.Emit(event)

	for i, ch := range []<-chan domain.TaskEvent{ch1, ch2} {
		select {
		case received := <-ch:
			if received.TaskID != event.TaskID {
				t.Errorf("subscriber %d: expected task ID %s, got %s", i+1, event.TaskID, received.TaskID)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("subscriber %d: timed out waiting for event", i+1)
		}
	}
}

func TestMemoryEventEmitter_Unsubscribe(t *testing.T) {
	emitter := NewMemoryEventEmitterAdapter()

	ch := emitter.Subscribe()

	emitter.Unsubscribe(ch)

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after unsubscribe")
	}
}

func TestMemoryEventEmitter_NonBlockingEmit(t *testing.T) {
	emitter := NewMemoryEventEmitterAdapter()

	ch := emitter.Subscribe()

	// Fill the buffer
	for i := 0; i < 150; i++ {
		emitter.Emit(domain.TaskEvent{
			TaskID:    domain.TaskID("task-" + string(rune('0'+i%10))),
			EventType: domain.TaskEventProgress,
		})
	}

	// Should not block even with full buffer
	done := make(chan bool)
	go func() {
		emitter.Emit(domain.TaskEvent{TaskID: "extra", EventType: domain.TaskEventCompleted})
		done <- true
	}()

	select {
	case <-done:
		// Success - emit didn't block
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Emit blocked on full channel")
	}

	// Drain what we can
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			// No more events
			goto done
		}
	}
done:
	// Buffer is 100, so we should have received 100 events
	if count != 100 {
		t.Errorf("expected 100 buffered events, got %d", count)
	}
}
