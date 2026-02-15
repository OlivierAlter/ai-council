package msgs

import (
    "time"
    "ai-council/internal/types"
)

// Agent lifecycle messages
type AgentStartingMsg struct {
    Agent types.AgentID
    Time  time.Time
}

type AgentStartedMsg struct {
    Agent types.AgentID
    Time  time.Time
}

type AgentOutputMsg struct {
    Agent  types.AgentID
    Line   string
    Stream string // stdout/stderr
}

type AgentCompletedMsg struct {
    Agent    types.AgentID
    Status   types.AgentStatus
    Duration time.Duration
    Usage    *types.TokenUsage
}

// Synthesis messages
type SynthesisStartedMsg struct{}
type SynthesisChunkMsg struct{ Text string }
type SynthesisCompleteMsg struct{ Result *types.SynthesisResult }

// Idle detection messages
type AgentIdleMsg struct {
    Agent    types.AgentID
    Duration time.Duration
}

type AgentStalledMsg struct {
    Agent    types.AgentID
    Duration time.Duration
}

type AgentIdleClearedMsg struct {
    Agent types.AgentID
}

// System messages
type TickMsg time.Time
type ErrorMsg struct{ Err error }

// Session messages
type SessionStartedMsg struct {
    ID     string
    Prompt string
    Agents []types.AgentID
}
