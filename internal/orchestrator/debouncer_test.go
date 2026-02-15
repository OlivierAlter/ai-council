package orchestrator

import (
	"ai-council/internal/types"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDebouncerBatchesOutput(t *testing.T) {
	var mu sync.Mutex
	var delivered []DebouncedLine

	d := NewOutputDebouncer(50*time.Millisecond, func(dl DebouncedLine) {
		mu.Lock()
		delivered = append(delivered, dl)
		mu.Unlock()
	})
	defer d.Stop()

	// Rapid fire 3 lines within the debounce window
	d.Append(types.AgentClaude, "line1")
	d.Append(types.AgentClaude, "line2")
	d.Append(types.AgentClaude, "line3")

	// Wait for debounce to fire
	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(delivered) != 1 {
		t.Fatalf("expected 1 batched delivery, got %d", len(delivered))
	}
	if delivered[0].Agent != types.AgentClaude {
		t.Errorf("agent = %s, want claude", delivered[0].Agent)
	}
	if delivered[0].Text != "line1\nline2\nline3" {
		t.Errorf("text = %q, want batched lines", delivered[0].Text)
	}
}

func TestDebouncerPerAgentIndependence(t *testing.T) {
	var mu sync.Mutex
	delivered := make(map[types.AgentID]int)

	d := NewOutputDebouncer(50*time.Millisecond, func(dl DebouncedLine) {
		mu.Lock()
		delivered[dl.Agent]++
		mu.Unlock()
	})
	defer d.Stop()

	d.Append(types.AgentClaude, "claude output")
	d.Append(types.AgentCodex, "codex output")

	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if delivered[types.AgentClaude] != 1 {
		t.Errorf("claude deliveries = %d, want 1", delivered[types.AgentClaude])
	}
	if delivered[types.AgentCodex] != 1 {
		t.Errorf("codex deliveries = %d, want 1", delivered[types.AgentCodex])
	}
}

func TestDebouncerLineTruncation(t *testing.T) {
	var mu sync.Mutex
	var delivered DebouncedLine

	d := NewOutputDebouncer(50*time.Millisecond, func(dl DebouncedLine) {
		mu.Lock()
		delivered = dl
		mu.Unlock()
	})
	defer d.Stop()

	longLine := strings.Repeat("x", MaxLineLength+1000)
	d.Append(types.AgentClaude, longLine)

	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(delivered.Text) > MaxLineLength+50 { // +50 for "... [truncated]"
		t.Errorf("line not truncated: len = %d, max = %d", len(delivered.Text), MaxLineLength)
	}
	if !strings.HasSuffix(delivered.Text, "... [truncated]") {
		t.Error("truncated line should end with truncation marker")
	}
}

func TestDebouncerFlush(t *testing.T) {
	var delivered []DebouncedLine
	d := NewOutputDebouncer(5*time.Second, func(dl DebouncedLine) {
		delivered = append(delivered, dl)
	})
	defer d.Stop()

	d.Append(types.AgentClaude, "should flush immediately")
	d.Flush(types.AgentClaude)

	if len(delivered) != 1 {
		t.Fatalf("Flush should deliver immediately, got %d deliveries", len(delivered))
	}
}

func TestDebouncerFlushAll(t *testing.T) {
	delivered := make(map[types.AgentID]string)
	d := NewOutputDebouncer(5*time.Second, func(dl DebouncedLine) {
		delivered[dl.Agent] = dl.Text
	})
	defer d.Stop()

	d.Append(types.AgentClaude, "claude")
	d.Append(types.AgentCodex, "codex")
	d.Append(types.AgentGemini, "gemini")

	d.FlushAll()

	if len(delivered) != 3 {
		t.Fatalf("FlushAll should deliver all 3, got %d", len(delivered))
	}
}

func TestDebouncerStop(t *testing.T) {
	var mu sync.Mutex
	var delivered int

	d := NewOutputDebouncer(5*time.Second, func(dl DebouncedLine) {
		mu.Lock()
		delivered++
		mu.Unlock()
	})

	d.Append(types.AgentClaude, "buffered")
	d.Stop()

	mu.Lock()
	if delivered != 1 {
		t.Errorf("Stop should flush buffered content, got %d deliveries", delivered)
	}
	mu.Unlock()

	// Append after Stop should be silently ignored
	d.Append(types.AgentClaude, "after stop")
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if delivered != 1 {
		t.Errorf("Append after Stop should be ignored, got %d deliveries", delivered)
	}
	mu.Unlock()
}

func TestDebouncerFlushEmptyBuffer(t *testing.T) {
	var delivered int
	d := NewOutputDebouncer(50*time.Millisecond, func(dl DebouncedLine) {
		delivered++
	})
	defer d.Stop()

	// Flush an agent that has no buffered content — should be a no-op
	d.Flush(types.AgentClaude)

	if delivered != 0 {
		t.Error("flushing empty buffer should not deliver")
	}
}

func TestDebouncerBufferSizeCap(t *testing.T) {
	var mu sync.Mutex
	var deliveries int

	d := NewOutputDebouncer(5*time.Second, func(dl DebouncedLine) {
		mu.Lock()
		deliveries++
		mu.Unlock()
	})
	defer d.Stop()

	// Lines are truncated to MaxLineLength (10KB), so use lines under
	// that limit and send enough to exceed MaxBufferBytes (2MB).
	line := strings.Repeat("x", 5000) // 5KB per line, under truncation limit
	for i := 0; i < 500; i++ {        // 500 * 5KB = 2.5MB > 2MB cap
		d.Append(types.AgentClaude, line)
	}

	mu.Lock()
	defer mu.Unlock()

	// Should have auto-flushed at least once due to buffer cap
	if deliveries < 1 {
		t.Error("expected auto-flush when buffer cap exceeded")
	}
}

func TestDebouncerTimerReset(t *testing.T) {
	var mu sync.Mutex
	var deliveries int

	d := NewOutputDebouncer(80*time.Millisecond, func(dl DebouncedLine) {
		mu.Lock()
		deliveries++
		mu.Unlock()
	})
	defer d.Stop()

	// Send a line, wait 50ms (less than 80ms debounce), send another
	d.Append(types.AgentClaude, "first")
	time.Sleep(50 * time.Millisecond)
	d.Append(types.AgentClaude, "second") // Should reset the timer

	// At 50ms after "second", timer hasn't fired yet
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	earlyDeliveries := deliveries
	mu.Unlock()

	if earlyDeliveries != 0 {
		t.Error("debounce timer should have been reset by second append")
	}

	// Wait for timer to fire
	time.Sleep(60 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if deliveries != 1 {
		t.Errorf("expected 1 delivery after timer fires, got %d", deliveries)
	}
}
