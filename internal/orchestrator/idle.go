package orchestrator

import (
	"ai-council/internal/types"
	"sync"
	"time"
)

const (
	DefaultIdleThreshold    = 60 * time.Second  // Warn after 60s of no output
	DefaultStalledThreshold = 180 * time.Second // Stalled after 3x idle
)

// IdleCallback is called when an agent's idle state changes.
type IdleCallback func(agent types.AgentID, event IdleEvent)

// IdleEvent describes what happened with agent idle detection.
type IdleEvent int

const (
	IdleDetected  IdleEvent = iota // No output for IdleThreshold
	StalledDetected                // No output for StalledThreshold
	IdleCleared                    // Agent produced output, no longer idle
)

func (e IdleEvent) String() string {
	return [...]string{"idle", "stalled", "cleared"}[e]
}

// IdleMonitor tracks per-agent output timing and fires callbacks
// when agents go idle (no output for a configurable threshold) or
// stalled (no output for a longer threshold).
//
// Inspired by OpenClaw's safety-net idle timer that detects when
// agents hang without producing output.
type IdleMonitor struct {
	mu               sync.Mutex
	trackers         map[types.AgentID]*idleTracker
	idleThreshold    time.Duration
	stalledThreshold time.Duration
	callback         IdleCallback
	stopped          bool
}

type idleTracker struct {
	lastOutput  time.Time
	idleTimer   *time.Timer
	stallTimer  *time.Timer
	isIdle      bool
	isStalled   bool
}

// NewIdleMonitor creates a monitor with the given thresholds and callback.
func NewIdleMonitor(idleThreshold, stalledThreshold time.Duration, callback IdleCallback) *IdleMonitor {
	if idleThreshold <= 0 {
		idleThreshold = DefaultIdleThreshold
	}
	if stalledThreshold <= 0 {
		stalledThreshold = DefaultStalledThreshold
	}
	return &IdleMonitor{
		trackers:         make(map[types.AgentID]*idleTracker),
		idleThreshold:    idleThreshold,
		stalledThreshold: stalledThreshold,
		callback:         callback,
	}
}

// StartTracking begins idle monitoring for an agent. Call this when
// the agent starts running.
func (im *IdleMonitor) StartTracking(agent types.AgentID) {
	im.mu.Lock()
	defer im.mu.Unlock()

	if im.stopped {
		return
	}

	now := time.Now()
	tracker := &idleTracker{
		lastOutput: now,
	}

	tracker.idleTimer = time.AfterFunc(im.idleThreshold, func() {
		im.onIdle(agent)
	})
	tracker.stallTimer = time.AfterFunc(im.stalledThreshold, func() {
		im.onStalled(agent)
	})

	im.trackers[agent] = tracker
}

// RecordOutput resets the idle timers for an agent. Call this on every
// line of output received.
func (im *IdleMonitor) RecordOutput(agent types.AgentID) {
	im.mu.Lock()
	defer im.mu.Unlock()

	if im.stopped {
		return
	}

	tracker, ok := im.trackers[agent]
	if !ok {
		return
	}

	tracker.lastOutput = time.Now()

	// Clear idle/stalled state if previously set
	wasIdle := tracker.isIdle || tracker.isStalled
	tracker.isIdle = false
	tracker.isStalled = false

	// Reset timers
	if tracker.idleTimer != nil {
		tracker.idleTimer.Stop()
	}
	tracker.idleTimer = time.AfterFunc(im.idleThreshold, func() {
		im.onIdle(agent)
	})

	if tracker.stallTimer != nil {
		tracker.stallTimer.Stop()
	}
	tracker.stallTimer = time.AfterFunc(im.stalledThreshold, func() {
		im.onStalled(agent)
	})

	if wasIdle && im.callback != nil {
		im.callback(agent, IdleCleared)
	}
}

// StopTracking stops monitoring an agent. Call this when the agent
// completes (any terminal state).
func (im *IdleMonitor) StopTracking(agent types.AgentID) {
	im.mu.Lock()
	defer im.mu.Unlock()

	tracker, ok := im.trackers[agent]
	if !ok {
		return
	}

	if tracker.idleTimer != nil {
		tracker.idleTimer.Stop()
	}
	if tracker.stallTimer != nil {
		tracker.stallTimer.Stop()
	}
	delete(im.trackers, agent)
}

// IsIdle returns whether an agent is currently idle.
func (im *IdleMonitor) IsIdle(agent types.AgentID) bool {
	im.mu.Lock()
	defer im.mu.Unlock()
	if tracker, ok := im.trackers[agent]; ok {
		return tracker.isIdle
	}
	return false
}

// IsStalled returns whether an agent is currently stalled.
func (im *IdleMonitor) IsStalled(agent types.AgentID) bool {
	im.mu.Lock()
	defer im.mu.Unlock()
	if tracker, ok := im.trackers[agent]; ok {
		return tracker.isStalled
	}
	return false
}

// IdleDuration returns how long the agent has been without output.
func (im *IdleMonitor) IdleDuration(agent types.AgentID) time.Duration {
	im.mu.Lock()
	defer im.mu.Unlock()
	if tracker, ok := im.trackers[agent]; ok {
		return time.Since(tracker.lastOutput)
	}
	return 0
}

// Stop cancels all timers and prevents further callbacks.
func (im *IdleMonitor) Stop() {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.stopped = true
	for _, tracker := range im.trackers {
		if tracker.idleTimer != nil {
			tracker.idleTimer.Stop()
		}
		if tracker.stallTimer != nil {
			tracker.stallTimer.Stop()
		}
	}
	im.trackers = make(map[types.AgentID]*idleTracker)
}

func (im *IdleMonitor) onIdle(agent types.AgentID) {
	im.mu.Lock()
	if im.stopped {
		im.mu.Unlock()
		return
	}
	tracker, ok := im.trackers[agent]
	if !ok {
		im.mu.Unlock()
		return
	}
	tracker.isIdle = true
	im.mu.Unlock()

	if im.callback != nil {
		im.callback(agent, IdleDetected)
	}
}

func (im *IdleMonitor) onStalled(agent types.AgentID) {
	im.mu.Lock()
	if im.stopped {
		im.mu.Unlock()
		return
	}
	tracker, ok := im.trackers[agent]
	if !ok {
		im.mu.Unlock()
		return
	}
	tracker.isStalled = true
	im.mu.Unlock()

	if im.callback != nil {
		im.callback(agent, StalledDetected)
	}
}
