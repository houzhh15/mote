package copilot

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mote/internal/provider"
)

// TestSessionEventIsolation tests that multiple concurrent sessions
// have isolated event channels.
func TestSessionEventIsolation(t *testing.T) {
	p := &ACPProvider{}

	// Create three concurrent sessions
	convIDs := []string{"conv-1", "conv-2", "conv-3"}
	eventCounts := make(map[string]*atomic.Int32)
	var wg sync.WaitGroup

	for _, convID := range convIDs {
		eventCounts[convID] = &atomic.Int32{}
	}

	// Start all sessions concurrently
	for _, convID := range convIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			events := make(chan provider.ChatEvent, 10)
			entry := &sessionEventEntry{
				events:    events,
				ctx:       ctx,
				createdAt: time.Now(),
			}

			// Register session
			p.sessionEvents.Store(id, entry)
			defer p.sessionEvents.Delete(id)

			// Send events to this session
			for i := 0; i < 5; i++ {
				p.safeSendEvent(id, provider.ChatEvent{
					Type:  provider.EventTypeContent,
					Delta: "test",
				})
			}

			// Count received events
			close(events)
			for range events {
				eventCounts[id].Add(1)
			}
		}(convID)
	}

	wg.Wait()

	// Verify each session received exactly 5 events
	for _, convID := range convIDs {
		count := eventCounts[convID].Load()
		if count != 5 {
			t.Errorf("Session %s: expected 5 events, got %d", convID, count)
		}
	}
}

// TestSessionEventIsolationCrossSession tests that events sent to one session
// don't appear in another session.
func TestSessionEventIsolationCrossSession(t *testing.T) {
	p := &ACPProvider{}

	ctx := context.Background()

	// Create two sessions
	events1 := make(chan provider.ChatEvent, 10)
	events2 := make(chan provider.ChatEvent, 10)

	entry1 := &sessionEventEntry{events: events1, ctx: ctx, createdAt: time.Now()}
	entry2 := &sessionEventEntry{events: events2, ctx: ctx, createdAt: time.Now()}

	p.sessionEvents.Store("session-1", entry1)
	p.sessionEvents.Store("session-2", entry2)
	defer p.sessionEvents.Delete("session-1")
	defer p.sessionEvents.Delete("session-2")

	// Send events to session-1
	p.safeSendEvent("session-1", provider.ChatEvent{
		Type:  provider.EventTypeContent,
		Delta: "hello from session 1",
	})

	// Send events to session-2
	p.safeSendEvent("session-2", provider.ChatEvent{
		Type:  provider.EventTypeContent,
		Delta: "hello from session 2",
	})

	// Verify session-1 received only its event
	select {
	case event := <-events1:
		if event.Delta != "hello from session 1" {
			t.Errorf("Session-1 received wrong event: %s", event.Delta)
		}
	default:
		t.Error("Session-1 did not receive any event")
	}

	// Verify session-2 received only its event
	select {
	case event := <-events2:
		if event.Delta != "hello from session 2" {
			t.Errorf("Session-2 received wrong event: %s", event.Delta)
		}
	default:
		t.Error("Session-2 did not receive any event")
	}

	// Verify no cross-contamination
	select {
	case event := <-events1:
		t.Errorf("Session-1 received extra event: %s", event.Delta)
	default:
		// Good, no extra events
	}

	select {
	case event := <-events2:
		t.Errorf("Session-2 received extra event: %s", event.Delta)
	default:
		// Good, no extra events
	}
}

// TestSessionEventCleanup tests that session events are properly cleaned up
// when the session ends.
func TestSessionEventCleanup(t *testing.T) {
	p := &ACPProvider{}

	ctx := context.Background()
	events := make(chan provider.ChatEvent, 10)
	entry := &sessionEventEntry{events: events, ctx: ctx, createdAt: time.Now()}

	// Register session
	p.sessionEvents.Store("cleanup-test", entry)

	// Verify it exists
	if _, ok := p.sessionEvents.Load("cleanup-test"); !ok {
		t.Error("Session should exist before cleanup")
	}

	// Clean up
	p.sessionEvents.Delete("cleanup-test")

	// Verify it's gone
	if _, ok := p.sessionEvents.Load("cleanup-test"); ok {
		t.Error("Session should not exist after cleanup")
	}

	// Verify safeSendEvent returns false for non-existent session
	result := p.safeSendEvent("cleanup-test", provider.ChatEvent{
		Type:  provider.EventTypeContent,
		Delta: "should not be sent",
	})
	if result {
		t.Error("safeSendEvent should return false for non-existent session")
	}
}

// TestSessionEventContextCancelled tests that events are not sent
// when the session context is cancelled.
func TestSessionEventContextCancelled(t *testing.T) {
	p := &ACPProvider{}

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan provider.ChatEvent, 10)
	entry := &sessionEventEntry{events: events, ctx: ctx, createdAt: time.Now()}

	p.sessionEvents.Store("cancel-test", entry)
	defer p.sessionEvents.Delete("cancel-test")

	// Cancel the context
	cancel()

	// Try to send event
	result := p.safeSendEvent("cancel-test", provider.ChatEvent{
		Type:  provider.EventTypeContent,
		Delta: "should not be sent",
	})

	if result {
		t.Error("safeSendEvent should return false when context is cancelled")
	}
}

// TestReverseSessionMapConsistency tests that the reverse session map
// is correctly maintained.
func TestReverseSessionMapConsistency(t *testing.T) {
	p := &ACPProvider{}

	// Simulate session creation
	convID := "conv-123"
	acpSessionID := "acp-session-456"

	p.sessionMap.Store(convID, acpSessionID)
	p.reverseSessionMap.Store(acpSessionID, convID)

	// Verify forward mapping
	if val, ok := p.sessionMap.Load(convID); !ok || val != acpSessionID {
		t.Errorf("Forward mapping failed: expected %s, got %v", acpSessionID, val)
	}

	// Verify reverse mapping
	if val, ok := p.reverseSessionMap.Load(acpSessionID); !ok || val != convID {
		t.Errorf("Reverse mapping failed: expected %s, got %v", convID, val)
	}
}

// TestConcurrentSessionRegistration tests that concurrent session registration
// doesn't cause data races.
func TestConcurrentSessionRegistration(t *testing.T) {
	p := &ACPProvider{}

	var wg sync.WaitGroup
	numSessions := 100

	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx := context.Background()
			convID := string(rune('a'+id%26)) + string(rune('0'+id/26))
			events := make(chan provider.ChatEvent, 10)
			entry := &sessionEventEntry{events: events, ctx: ctx, createdAt: time.Now()}

			// Register
			p.sessionEvents.Store(convID, entry)

			// Send some events
			for j := 0; j < 10; j++ {
				p.safeSendEvent(convID, provider.ChatEvent{
					Type:  provider.EventTypeContent,
					Delta: "test",
				})
			}

			// Clean up
			p.sessionEvents.Delete(convID)
			close(events)
		}(i)
	}

	wg.Wait()
	// If we get here without race detector complaints, the test passes
}
