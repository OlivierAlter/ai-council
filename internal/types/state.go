package types

import (
	"fmt"
	"time"
)

type AgentID string

const (
	AgentClaude AgentID = "claude"
	AgentCodex  AgentID = "codex"
	AgentGemini AgentID = "gemini"
	AgentVibe   AgentID = "vibe"
)

var AgentOrder = []AgentID{AgentClaude, AgentCodex, AgentGemini, AgentVibe}

// AgentStatus represents the lifecycle state of an agent session.
// Valid transitions:
//
//	Pending  -> Starting     (container launching)
//	Starting -> Running      (first output received)
//	Running  -> WaitingInput (multi-turn: waiting for next prompt, Phase 3)
//	WaitingInput -> Running  (follow-up prompt sent)
//	Running  -> Completed    (finished successfully)
//	Running  -> Failed       (error/crash)
//	Starting -> Failed       (launch failure)
//	Running  -> TimedOut     (watchdog killed)
//	Starting -> TimedOut     (launch timeout)
//	*        -> Killed       (user-initiated kill, from any active state)
type AgentStatus int

const (
	StatusPending      AgentStatus = iota
	StatusStarting                         // Container launching, not yet producing output
	StatusRunning                          // Actively producing output
	StatusWaitingInput                     // Multi-turn: waiting for next prompt (Phase 3)
	StatusCompleted                        // Finished successfully
	StatusFailed                           // Crashed or error
	StatusTimedOut                         // Watchdog killed
	StatusKilled                           // User-initiated kill
)

var statusNames = [...]string{
	"pending", "starting", "running", "waiting_input",
	"completed", "failed", "timeout", "killed",
}

func (s AgentStatus) String() string {
	if int(s) < len(statusNames) {
		return statusNames[s]
	}
	return fmt.Sprintf("unknown(%d)", s)
}

// IsTerminal returns true if the agent is in a final state.
func (s AgentStatus) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusTimedOut, StatusKilled:
		return true
	}
	return false
}

// IsActive returns true if the agent is doing work or about to.
func (s AgentStatus) IsActive() bool {
	switch s {
	case StatusStarting, StatusRunning, StatusWaitingInput:
		return true
	}
	return false
}

// validTransitions defines the allowed state machine transitions.
var validTransitions = map[AgentStatus][]AgentStatus{
	StatusPending:      {StatusStarting},
	StatusStarting:     {StatusRunning, StatusFailed, StatusTimedOut, StatusKilled},
	StatusRunning:      {StatusWaitingInput, StatusCompleted, StatusFailed, StatusTimedOut, StatusKilled},
	StatusWaitingInput: {StatusRunning, StatusFailed, StatusTimedOut, StatusKilled},
}

// CanTransitionTo checks if transitioning from the current status to next is valid.
func (s AgentStatus) CanTransitionTo(next AgentStatus) bool {
	allowed, ok := validTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == next {
			return true
		}
	}
	return false
}

type TokenUsage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

type OutputLine struct {
	Text      string
	Stream    string // "stdout" or "stderr"
	Timestamp time.Time
}

type AgentState struct {
	ID           AgentID
	Status       AgentStatus
	StartTime    time.Time
	EndTime      *time.Time
	LastOutputAt *time.Time    // Tracks last output for idle detection
	Output       []OutputLine  // Full log
	Stream       []string      // Ring buffer for UI (last N lines)
	FinalJSON    string        // Parsed final result
	Error        string
	Usage        *TokenUsage
}

// Transition attempts to move the agent to a new status.
// Returns an error if the transition is not valid.
func (a *AgentState) Transition(next AgentStatus) error {
	if !a.Status.CanTransitionTo(next) {
		return fmt.Errorf("invalid transition: %s -> %s for agent %s", a.Status, next, a.ID)
	}
	a.Status = next
	if next.IsTerminal() {
		now := time.Now()
		a.EndTime = &now
	}
	return nil
}

type SynthesisResult struct {
	Summary     string            `json:"summary"`
	Approach    string            `json:"approach"`
	Confidence  string            `json:"confidence"`
	Consensus   []string          `json:"consensus_areas"`
	Divergences []string          `json:"divergence_areas"`
	Warnings    []string          `json:"warnings"`
	AgentEvals  map[string]string `json:"agents"`
}

type SessionState struct {
	ID              string
	Prompt          string
	StartTime       time.Time
	EndTime         *time.Time
	Timeout         time.Duration
	Agents          map[AgentID]*AgentState
	SynthesisResult *SynthesisResult
	SynthesisActive bool
}

// ActiveAgents returns agents that are currently doing work.
func (s *SessionState) ActiveAgents() []*AgentState {
	var active []*AgentState
	for _, a := range s.Agents {
		if a.Status.IsActive() {
			active = append(active, a)
		}
	}
	return active
}

// AllTerminal returns true if every agent has reached a terminal state.
func (s *SessionState) AllTerminal() bool {
	for _, a := range s.Agents {
		if !a.Status.IsTerminal() {
			return false
		}
	}
	return true
}
