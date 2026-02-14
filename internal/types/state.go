package types

import "time"

type AgentID string

const (
    AgentClaude AgentID = "claude"
    AgentCodex  AgentID = "codex"
    AgentGemini AgentID = "gemini"
    AgentVibe   AgentID = "vibe"
)

var AgentOrder = []AgentID{AgentClaude, AgentCodex, AgentGemini, AgentVibe}

type AgentStatus int

const (
    StatusPending AgentStatus = iota
    StatusRunning
    StatusCompleted
    StatusFailed
    StatusTimeout
    StatusCanceled
)

func (s AgentStatus) String() string {
    return [...]string{"pending", "running", "completed", "failed", "timeout", "canceled"}[s]
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
    ID        AgentID
    Status    AgentStatus
    StartTime time.Time
    EndTime   *time.Time
    Output    []OutputLine  // Full log
    Stream    []string      // Ring buffer for UI (last N lines)
    FinalJSON string        // Parsed final result
    Error     string
    Usage     *TokenUsage
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
