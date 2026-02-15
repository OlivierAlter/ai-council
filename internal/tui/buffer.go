package tui

import (
	"ai-council/internal/types"
	"strings"
	"sync"
)

const (
	maxRingLines = 500    // Max lines kept in ring buffer per agent
	maxCatchupLines = 200 // Max lines delivered on catchup
)

// PanelBuffer manages output buffering for an agent panel with
// foreground/background awareness. When the panel is backgrounded,
// output accumulates in a catchup buffer. When foregrounded, the
// catchup is flushed and new output streams directly.
//
// Inspired by OpenClaw's foreground/background session model with
// catchup delivery on re-entry.
type PanelBuffer struct {
	mu         sync.Mutex
	agent      types.AgentID
	ring       []string // Ring buffer of recent lines (capped at maxRingLines)
	catchup    []string // Lines accumulated while backgrounded
	foreground bool     // Whether this panel is currently visible
	dirty      bool     // Whether catchup has unflushed content
}

// NewPanelBuffer creates a buffer for the given agent, starting in
// background mode.
func NewPanelBuffer(agent types.AgentID) *PanelBuffer {
	return &PanelBuffer{
		agent: agent,
		ring:  make([]string, 0, maxRingLines),
	}
}

// Append adds a line of output. If backgrounded, it goes to the catchup
// buffer. The ring buffer is always updated.
func (pb *PanelBuffer) Append(line string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	// Always update ring buffer
	pb.ring = append(pb.ring, line)
	if len(pb.ring) > maxRingLines {
		pb.ring = pb.ring[len(pb.ring)-maxRingLines:]
	}

	// If backgrounded, accumulate in catchup
	if !pb.foreground {
		pb.catchup = append(pb.catchup, line)
		if len(pb.catchup) > maxCatchupLines {
			// Keep only the tail, prepend a "skipped N lines" marker
			skipped := len(pb.catchup) - maxCatchupLines
			pb.catchup = pb.catchup[skipped:]
		}
		pb.dirty = true
	}
}

// SetForeground marks this panel as currently visible. Returns the
// catchup text that was accumulated while backgrounded (empty string
// if nothing was missed).
func (pb *PanelBuffer) SetForeground() string {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	pb.foreground = true
	if !pb.dirty || len(pb.catchup) == 0 {
		pb.catchup = nil
		pb.dirty = false
		return ""
	}

	catchupText := strings.Join(pb.catchup, "\n")
	pb.catchup = nil
	pb.dirty = false
	return catchupText
}

// SetBackground marks this panel as not visible. Subsequent Append
// calls will buffer output for catchup.
func (pb *PanelBuffer) SetBackground() {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.foreground = false
}

// Content returns the full ring buffer content for viewport rendering.
func (pb *PanelBuffer) Content() string {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return strings.Join(pb.ring, "\n")
}

// HasCatchup returns true if there's undelivered catchup content.
func (pb *PanelBuffer) HasCatchup() bool {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return pb.dirty
}

// CatchupCount returns the number of lines waiting for catchup delivery.
func (pb *PanelBuffer) CatchupCount() int {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return len(pb.catchup)
}
