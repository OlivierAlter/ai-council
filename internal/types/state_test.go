package types

import (
	"testing"
)

func TestAgentStatusString(t *testing.T) {
	tests := []struct {
		status AgentStatus
		want   string
	}{
		{StatusPending, "pending"},
		{StatusStarting, "starting"},
		{StatusRunning, "running"},
		{StatusWaitingInput, "waiting_input"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusTimedOut, "timeout"},
		{StatusKilled, "killed"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("AgentStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestAgentStatusStringOutOfRange(t *testing.T) {
	s := AgentStatus(99)
	got := s.String()
	if got == "" {
		t.Error("out-of-range status should return non-empty string")
	}
}

func TestIsTerminal(t *testing.T) {
	terminal := []AgentStatus{StatusCompleted, StatusFailed, StatusTimedOut, StatusKilled}
	nonTerminal := []AgentStatus{StatusPending, StatusStarting, StatusRunning, StatusWaitingInput}

	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("%s should not be terminal", s)
		}
	}
}

func TestIsActive(t *testing.T) {
	active := []AgentStatus{StatusStarting, StatusRunning, StatusWaitingInput}
	inactive := []AgentStatus{StatusPending, StatusCompleted, StatusFailed, StatusTimedOut, StatusKilled}

	for _, s := range active {
		if !s.IsActive() {
			t.Errorf("%s should be active", s)
		}
	}
	for _, s := range inactive {
		if s.IsActive() {
			t.Errorf("%s should not be active", s)
		}
	}
}

func TestValidTransitions(t *testing.T) {
	valid := []struct {
		from, to AgentStatus
	}{
		{StatusPending, StatusStarting},
		{StatusStarting, StatusRunning},
		{StatusStarting, StatusFailed},
		{StatusStarting, StatusTimedOut},
		{StatusStarting, StatusKilled},
		{StatusRunning, StatusWaitingInput},
		{StatusRunning, StatusCompleted},
		{StatusRunning, StatusFailed},
		{StatusRunning, StatusTimedOut},
		{StatusRunning, StatusKilled},
		{StatusWaitingInput, StatusRunning},
		{StatusWaitingInput, StatusFailed},
		{StatusWaitingInput, StatusTimedOut},
		{StatusWaitingInput, StatusKilled},
	}

	for _, tt := range valid {
		if !tt.from.CanTransitionTo(tt.to) {
			t.Errorf("%s -> %s should be valid", tt.from, tt.to)
		}
	}
}

func TestInvalidTransitions(t *testing.T) {
	invalid := []struct {
		from, to AgentStatus
	}{
		{StatusPending, StatusRunning},      // Must go through Starting
		{StatusPending, StatusCompleted},     // Can't complete from Pending
		{StatusCompleted, StatusRunning},     // Terminal -> anything is invalid
		{StatusFailed, StatusRunning},        // Terminal -> anything is invalid
		{StatusTimedOut, StatusStarting},     // Terminal -> anything is invalid
		{StatusKilled, StatusPending},        // Terminal -> anything is invalid
		{StatusStarting, StatusWaitingInput}, // Can't wait from Starting
		{StatusStarting, StatusCompleted},    // Can't complete from Starting
		{StatusRunning, StatusStarting},      // Can't go backwards
		{StatusRunning, StatusPending},       // Can't go backwards
	}

	for _, tt := range invalid {
		if tt.from.CanTransitionTo(tt.to) {
			t.Errorf("%s -> %s should be invalid", tt.from, tt.to)
		}
	}
}

func TestAgentStateTransition(t *testing.T) {
	agent := &AgentState{ID: AgentClaude, Status: StatusPending}

	// Valid: Pending -> Starting
	if err := agent.Transition(StatusStarting); err != nil {
		t.Fatalf("Pending -> Starting failed: %v", err)
	}
	if agent.Status != StatusStarting {
		t.Errorf("status = %s, want starting", agent.Status)
	}
	if agent.EndTime != nil {
		t.Error("EndTime should be nil for non-terminal state")
	}

	// Valid: Starting -> Running
	if err := agent.Transition(StatusRunning); err != nil {
		t.Fatalf("Starting -> Running failed: %v", err)
	}

	// Valid: Running -> Completed (terminal — should set EndTime)
	if err := agent.Transition(StatusCompleted); err != nil {
		t.Fatalf("Running -> Completed failed: %v", err)
	}
	if agent.EndTime == nil {
		t.Error("EndTime should be set for terminal state")
	}
}

func TestAgentStateTransitionInvalid(t *testing.T) {
	agent := &AgentState{ID: AgentCodex, Status: StatusPending}

	err := agent.Transition(StatusCompleted)
	if err == nil {
		t.Fatal("Pending -> Completed should fail")
	}
	// Status should not change on invalid transition
	if agent.Status != StatusPending {
		t.Errorf("status changed to %s on invalid transition", agent.Status)
	}
}

func TestSessionStateActiveAgents(t *testing.T) {
	session := &SessionState{
		Agents: map[AgentID]*AgentState{
			AgentClaude: {ID: AgentClaude, Status: StatusRunning},
			AgentCodex:  {ID: AgentCodex, Status: StatusCompleted},
			AgentGemini: {ID: AgentGemini, Status: StatusStarting},
			AgentVibe:   {ID: AgentVibe, Status: StatusFailed},
		},
	}

	active := session.ActiveAgents()
	if len(active) != 2 {
		t.Fatalf("ActiveAgents() returned %d, want 2", len(active))
	}

	ids := map[AgentID]bool{}
	for _, a := range active {
		ids[a.ID] = true
	}
	if !ids[AgentClaude] || !ids[AgentGemini] {
		t.Errorf("expected claude and gemini active, got %v", ids)
	}
}

func TestSessionStateAllTerminal(t *testing.T) {
	session := &SessionState{
		Agents: map[AgentID]*AgentState{
			AgentClaude: {ID: AgentClaude, Status: StatusCompleted},
			AgentCodex:  {ID: AgentCodex, Status: StatusFailed},
		},
	}

	if !session.AllTerminal() {
		t.Error("AllTerminal() should be true when all agents are terminal")
	}

	session.Agents[AgentGemini] = &AgentState{ID: AgentGemini, Status: StatusRunning}
	if session.AllTerminal() {
		t.Error("AllTerminal() should be false when an agent is running")
	}
}
