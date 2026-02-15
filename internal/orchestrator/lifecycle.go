package orchestrator

import (
	"ai-council/internal/types"
	"fmt"
	"sync"
	"time"
)

// AgentCompletion captures the final state of an agent run.
type AgentCompletion struct {
	Agent     types.AgentID
	Status    types.AgentStatus
	Duration  time.Duration
	Usage     *types.TokenUsage
	Error     string
}

// OnCompleteFunc is called when an agent reaches a terminal state.
type OnCompleteFunc func(AgentCompletion)

// SessionLifecycle ensures that onComplete callbacks fire on ALL exit paths
// (success, failure, timeout, kill) for every agent in a session.
// Inspired by OpenClaw's bug where onComplete only fired on the result path.
type SessionLifecycle struct {
	mu          sync.Mutex
	agents      map[types.AgentID]*agentTracker
	onComplete  OnCompleteFunc
	completions []AgentCompletion
}

type agentTracker struct {
	id        types.AgentID
	startTime time.Time
	completed bool
}

func NewSessionLifecycle(onComplete OnCompleteFunc) *SessionLifecycle {
	return &SessionLifecycle{
		agents:     make(map[types.AgentID]*agentTracker),
		onComplete: onComplete,
	}
}

// TrackAgent registers an agent for lifecycle tracking. Must be called
// before the agent starts work.
func (sl *SessionLifecycle) TrackAgent(id types.AgentID) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.agents[id] = &agentTracker{
		id:        id,
		startTime: time.Now(),
	}
}

// Complete marks an agent as finished. Safe to call multiple times —
// only the first call per agent triggers onComplete.
func (sl *SessionLifecycle) Complete(id types.AgentID, status types.AgentStatus, usage *types.TokenUsage, err string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	tracker, ok := sl.agents[id]
	if !ok || tracker.completed {
		return
	}
	tracker.completed = true

	completion := AgentCompletion{
		Agent:    id,
		Status:   status,
		Duration: time.Since(tracker.startTime),
		Usage:    usage,
		Error:    err,
	}
	sl.completions = append(sl.completions, completion)

	if sl.onComplete != nil {
		sl.onComplete(completion)
	}
}

// EnsureAllComplete forces completion for any agent that hasn't been
// explicitly completed. Use this in a defer to guarantee no agent is
// silently dropped. Agents without explicit completion are marked as Failed.
func (sl *SessionLifecycle) EnsureAllComplete() {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	for id, tracker := range sl.agents {
		if tracker.completed {
			continue
		}
		tracker.completed = true

		completion := AgentCompletion{
			Agent:    id,
			Status:   types.StatusFailed,
			Duration: time.Since(tracker.startTime),
			Error:    "agent did not complete normally (forced by lifecycle cleanup)",
		}
		sl.completions = append(sl.completions, completion)

		if sl.onComplete != nil {
			sl.onComplete(completion)
		}
	}
}

// Completions returns all agent completions recorded so far.
func (sl *SessionLifecycle) Completions() []AgentCompletion {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	out := make([]AgentCompletion, len(sl.completions))
	copy(out, sl.completions)
	return out
}

// RunAgentWithLifecycle wraps an agent execution function with lifecycle
// guarantees. The provided fn runs the agent work; regardless of how it
// exits (return, panic, etc.), completion is recorded.
//
// Usage:
//
//	lifecycle.TrackAgent(agentID)
//	go RunAgentWithLifecycle(lifecycle, agentID, func() (types.AgentStatus, *types.TokenUsage, error) {
//	    // ... do work ...
//	    return types.StatusCompleted, usage, nil
//	})
func RunAgentWithLifecycle(
	sl *SessionLifecycle,
	id types.AgentID,
	fn func() (types.AgentStatus, *types.TokenUsage, error),
) {
	var (
		status types.AgentStatus = types.StatusFailed
		usage  *types.TokenUsage
		errMsg string
	)

	defer func() {
		if r := recover(); r != nil {
			status = types.StatusFailed
			errMsg = fmt.Sprintf("panic: %v", r)
		}
		sl.Complete(id, status, usage, errMsg)
	}()

	status, usage, err := fn()
	if err != nil {
		errMsg = err.Error()
	}
}
