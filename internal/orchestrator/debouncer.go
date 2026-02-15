package orchestrator

import (
	"ai-council/internal/types"
	"strings"
	"sync"
	"time"
)

const (
	DefaultDebounceInterval = 500 * time.Millisecond
	MaxLineLength           = 10_000 // 10KB per line — prevents memory blow-up from base64 blobs
	MaxBufferBytes          = 2_000_000 // 2MB total buffer cap
)

// DebouncedLine is a batched output delivery from the debouncer.
type DebouncedLine struct {
	Agent types.AgentID
	Text  string
}

// OutputDebouncer batches rapid agent output into time windows before
// delivering to the TUI. Prevents flooding from high-frequency output.
// Inspired by OpenClaw's NotificationRouter debouncing pattern.
type OutputDebouncer struct {
	mu       sync.Mutex
	buffers  map[types.AgentID]*outputBuffer
	interval time.Duration
	deliver  func(DebouncedLine)
	stopped  bool
}

type outputBuffer struct {
	sb         strings.Builder
	byteCount  int
	timer      *time.Timer
	agent      types.AgentID
}

// NewOutputDebouncer creates a debouncer that batches output for each
// agent and delivers it after the interval elapses with no new input.
func NewOutputDebouncer(interval time.Duration, deliver func(DebouncedLine)) *OutputDebouncer {
	if interval <= 0 {
		interval = DefaultDebounceInterval
	}
	return &OutputDebouncer{
		buffers:  make(map[types.AgentID]*outputBuffer),
		interval: interval,
		deliver:  deliver,
	}
}

// Append adds a line of output for an agent. The line is buffered and
// delivered after the debounce interval. Lines exceeding MaxLineLength
// are truncated.
func (d *OutputDebouncer) Append(agent types.AgentID, line string) {
	// Truncate excessively long lines
	if len(line) > MaxLineLength {
		line = line[:MaxLineLength] + "... [truncated]"
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return
	}

	buf := d.buffers[agent]
	if buf == nil {
		buf = &outputBuffer{agent: agent}
		d.buffers[agent] = buf
	}

	// Enforce total buffer size cap
	if buf.byteCount+len(line) > MaxBufferBytes {
		d.flushLocked(agent)
		buf = d.buffers[agent]
		if buf == nil {
			buf = &outputBuffer{agent: agent}
			d.buffers[agent] = buf
		}
	}

	if buf.sb.Len() > 0 {
		buf.sb.WriteByte('\n')
		buf.byteCount++
	}
	buf.sb.WriteString(line)
	buf.byteCount += len(line)

	// Reset timer on each append
	if buf.timer != nil {
		buf.timer.Stop()
	}
	buf.timer = time.AfterFunc(d.interval, func() {
		d.Flush(agent)
	})
}

// Flush immediately delivers the buffered output for an agent.
func (d *OutputDebouncer) Flush(agent types.AgentID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.flushLocked(agent)
}

// flushLocked delivers buffered output. Caller must hold d.mu.
func (d *OutputDebouncer) flushLocked(agent types.AgentID) {
	buf, ok := d.buffers[agent]
	if !ok || buf.sb.Len() == 0 {
		return
	}

	text := buf.sb.String()
	buf.sb.Reset()
	buf.byteCount = 0
	if buf.timer != nil {
		buf.timer.Stop()
		buf.timer = nil
	}

	if d.deliver != nil {
		d.deliver(DebouncedLine{Agent: agent, Text: text})
	}
}

// FlushAll delivers all buffered output for all agents immediately.
func (d *OutputDebouncer) FlushAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for agent := range d.buffers {
		d.flushLocked(agent)
	}
}

// Stop flushes all remaining output and prevents further buffering.
func (d *OutputDebouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stopped = true
	for agent := range d.buffers {
		d.flushLocked(agent)
	}
}
