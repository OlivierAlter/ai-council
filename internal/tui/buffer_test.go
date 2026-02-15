package tui

import (
	"ai-council/internal/types"
	"fmt"
	"strings"
	"testing"
)

func TestPanelBufferStartsBackgrounded(t *testing.T) {
	buf := NewPanelBuffer(types.AgentClaude)

	// Append while backgrounded should accumulate catchup
	buf.Append("line1")
	buf.Append("line2")

	if !buf.HasCatchup() {
		t.Error("should have catchup after appending while backgrounded")
	}
	if buf.CatchupCount() != 2 {
		t.Errorf("CatchupCount() = %d, want 2", buf.CatchupCount())
	}
}

func TestPanelBufferForegroundNoCatchup(t *testing.T) {
	buf := NewPanelBuffer(types.AgentClaude)
	buf.SetForeground()

	buf.Append("line1")
	buf.Append("line2")

	if buf.HasCatchup() {
		t.Error("should not have catchup when foregrounded")
	}
	if buf.CatchupCount() != 0 {
		t.Errorf("CatchupCount() = %d, want 0", buf.CatchupCount())
	}
}

func TestPanelBufferCatchupDelivery(t *testing.T) {
	buf := NewPanelBuffer(types.AgentClaude)

	// Append while backgrounded
	buf.Append("missed1")
	buf.Append("missed2")
	buf.Append("missed3")

	// Foreground — should get catchup
	catchup := buf.SetForeground()

	if catchup != "missed1\nmissed2\nmissed3" {
		t.Errorf("catchup = %q, want joined missed lines", catchup)
	}

	// Catchup should be cleared
	if buf.HasCatchup() {
		t.Error("catchup should be cleared after SetForeground()")
	}
}

func TestPanelBufferCatchupClearedOnForeground(t *testing.T) {
	buf := NewPanelBuffer(types.AgentClaude)

	buf.Append("line1")

	// First foreground — gets catchup
	catchup1 := buf.SetForeground()
	if catchup1 == "" {
		t.Error("first foreground should deliver catchup")
	}

	// Background then foreground again with no new content
	buf.SetBackground()
	catchup2 := buf.SetForeground()
	if catchup2 != "" {
		t.Errorf("second foreground should have empty catchup, got %q", catchup2)
	}
}

func TestPanelBufferRingBufferCap(t *testing.T) {
	buf := NewPanelBuffer(types.AgentClaude)
	buf.SetForeground() // Avoid catchup accumulation

	// Append more than maxRingLines
	for i := 0; i < maxRingLines+100; i++ {
		buf.Append(fmt.Sprintf("line%d", i))
	}

	content := buf.Content()
	lines := strings.Split(content, "\n")

	if len(lines) != maxRingLines {
		t.Errorf("ring buffer has %d lines, want %d (capped)", len(lines), maxRingLines)
	}

	// Should have the last maxRingLines lines
	firstLine := lines[0]
	expectedFirst := fmt.Sprintf("line%d", 100) // 0..599 kept = lines 100..599
	if firstLine != expectedFirst {
		t.Errorf("first line = %q, want %q (oldest kept)", firstLine, expectedFirst)
	}
}

func TestPanelBufferCatchupCap(t *testing.T) {
	buf := NewPanelBuffer(types.AgentClaude)

	// Append more than maxCatchupLines while backgrounded
	for i := 0; i < maxCatchupLines+50; i++ {
		buf.Append(fmt.Sprintf("line%d", i))
	}

	if buf.CatchupCount() != maxCatchupLines {
		t.Errorf("CatchupCount() = %d, want %d (capped)", buf.CatchupCount(), maxCatchupLines)
	}

	// Catchup should contain the tail (most recent lines)
	catchup := buf.SetForeground()
	lines := strings.Split(catchup, "\n")
	lastLine := lines[len(lines)-1]
	expectedLast := fmt.Sprintf("line%d", maxCatchupLines+49)
	if lastLine != expectedLast {
		t.Errorf("last catchup line = %q, want %q", lastLine, expectedLast)
	}
}

func TestPanelBufferContentAlwaysUpdated(t *testing.T) {
	buf := NewPanelBuffer(types.AgentClaude)

	// Ring buffer should update regardless of foreground/background
	buf.Append("bg1")
	buf.Append("bg2")

	content := buf.Content()
	if content != "bg1\nbg2" {
		t.Errorf("Content() while backgrounded = %q, want ring buffer content", content)
	}

	buf.SetForeground()
	buf.Append("fg1")

	content = buf.Content()
	if content != "bg1\nbg2\nfg1" {
		t.Errorf("Content() after foreground append = %q", content)
	}
}

func TestPanelBufferBackgroundToggle(t *testing.T) {
	buf := NewPanelBuffer(types.AgentClaude)

	// Start backgrounded, foreground, background, foreground
	buf.Append("bg1")
	buf.SetForeground() // catchup: "bg1"
	buf.Append("fg1")   // no catchup
	buf.SetBackground()
	buf.Append("bg2")   // catchup: "bg2"
	buf.Append("bg3")   // catchup: "bg2\nbg3"

	catchup := buf.SetForeground()
	if catchup != "bg2\nbg3" {
		t.Errorf("catchup after toggle = %q, want bg2/bg3 only", catchup)
	}
}

func TestPanelBufferEmptyCatchup(t *testing.T) {
	buf := NewPanelBuffer(types.AgentClaude)

	// Foreground with nothing appended
	catchup := buf.SetForeground()
	if catchup != "" {
		t.Errorf("catchup on fresh buffer = %q, want empty", catchup)
	}
}

func TestPanelBufferHasCatchupAfterBackground(t *testing.T) {
	buf := NewPanelBuffer(types.AgentClaude)
	buf.SetForeground()

	if buf.HasCatchup() {
		t.Error("should not have catchup when foregrounded with no background activity")
	}

	buf.SetBackground()
	buf.Append("new content")

	if !buf.HasCatchup() {
		t.Error("should have catchup after appending in background")
	}
}
