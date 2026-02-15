package orchestrator

import (
	"ai-council/internal/types"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLifecycleBasicCompletion(t *testing.T) {
	var called int
	var lastCompletion AgentCompletion

	sl := NewSessionLifecycle(func(c AgentCompletion) {
		called++
		lastCompletion = c
	})

	sl.TrackAgent(types.AgentClaude)
	sl.Complete(types.AgentClaude, types.StatusCompleted, &types.TokenUsage{InputTokens: 100}, "")

	if called != 1 {
		t.Fatalf("onComplete called %d times, want 1", called)
	}
	if lastCompletion.Agent != types.AgentClaude {
		t.Errorf("agent = %s, want claude", lastCompletion.Agent)
	}
	if lastCompletion.Status != types.StatusCompleted {
		t.Errorf("status = %s, want completed", lastCompletion.Status)
	}
	if lastCompletion.Usage.InputTokens != 100 {
		t.Errorf("input tokens = %d, want 100", lastCompletion.Usage.InputTokens)
	}
}

func TestLifecycleIdempotentComplete(t *testing.T) {
	var called int

	sl := NewSessionLifecycle(func(c AgentCompletion) {
		called++
	})

	sl.TrackAgent(types.AgentClaude)
	sl.Complete(types.AgentClaude, types.StatusCompleted, nil, "")
	sl.Complete(types.AgentClaude, types.StatusFailed, nil, "second call")
	sl.Complete(types.AgentClaude, types.StatusKilled, nil, "third call")

	if called != 1 {
		t.Fatalf("onComplete called %d times, want 1 (first call wins)", called)
	}
}

func TestLifecycleEnsureAllComplete(t *testing.T) {
	completions := make(map[types.AgentID]AgentCompletion)

	sl := NewSessionLifecycle(func(c AgentCompletion) {
		completions[c.Agent] = c
	})

	sl.TrackAgent(types.AgentClaude)
	sl.TrackAgent(types.AgentCodex)
	sl.TrackAgent(types.AgentGemini)

	// Only complete claude explicitly
	sl.Complete(types.AgentClaude, types.StatusCompleted, nil, "")

	// EnsureAllComplete should force-complete codex and gemini
	sl.EnsureAllComplete()

	if len(completions) != 3 {
		t.Fatalf("expected 3 completions, got %d", len(completions))
	}

	if completions[types.AgentClaude].Status != types.StatusCompleted {
		t.Error("claude should be completed")
	}
	if completions[types.AgentCodex].Status != types.StatusFailed {
		t.Errorf("codex should be forced-failed, got %s", completions[types.AgentCodex].Status)
	}
	if completions[types.AgentGemini].Status != types.StatusFailed {
		t.Errorf("gemini should be forced-failed, got %s", completions[types.AgentGemini].Status)
	}
}

func TestLifecycleEnsureAllCompleteIdempotent(t *testing.T) {
	var called int

	sl := NewSessionLifecycle(func(c AgentCompletion) {
		called++
	})

	sl.TrackAgent(types.AgentClaude)

	sl.EnsureAllComplete()
	sl.EnsureAllComplete() // Second call should be a no-op

	if called != 1 {
		t.Fatalf("onComplete called %d times, want 1", called)
	}
}

func TestLifecycleCompletions(t *testing.T) {
	sl := NewSessionLifecycle(nil)

	sl.TrackAgent(types.AgentClaude)
	sl.TrackAgent(types.AgentCodex)

	sl.Complete(types.AgentClaude, types.StatusCompleted, nil, "")
	sl.Complete(types.AgentCodex, types.StatusFailed, nil, "timeout")

	completions := sl.Completions()
	if len(completions) != 2 {
		t.Fatalf("expected 2 completions, got %d", len(completions))
	}
}

func TestLifecycleCompleteUntracked(t *testing.T) {
	var called int

	sl := NewSessionLifecycle(func(c AgentCompletion) {
		called++
	})

	// Complete an agent that was never tracked — should be silently ignored
	sl.Complete(types.AgentVibe, types.StatusCompleted, nil, "")

	if called != 0 {
		t.Fatalf("onComplete should not fire for untracked agent")
	}
}

func TestLifecycleNilCallback(t *testing.T) {
	sl := NewSessionLifecycle(nil)
	sl.TrackAgent(types.AgentClaude)

	// Should not panic with nil callback
	sl.Complete(types.AgentClaude, types.StatusCompleted, nil, "")
	sl.EnsureAllComplete()
}

func TestRunAgentWithLifecycleSuccess(t *testing.T) {
	var completion AgentCompletion

	sl := NewSessionLifecycle(func(c AgentCompletion) {
		completion = c
	})
	sl.TrackAgent(types.AgentClaude)

	RunAgentWithLifecycle(sl, types.AgentClaude, func() (types.AgentStatus, *types.TokenUsage, error) {
		return types.StatusCompleted, &types.TokenUsage{CostUSD: 0.05}, nil
	})

	if completion.Status != types.StatusCompleted {
		t.Errorf("status = %s, want completed", completion.Status)
	}
	if completion.Usage.CostUSD != 0.05 {
		t.Errorf("cost = %f, want 0.05", completion.Usage.CostUSD)
	}
	if completion.Error != "" {
		t.Errorf("error = %q, want empty", completion.Error)
	}
}

func TestRunAgentWithLifecyclePanicRecovery(t *testing.T) {
	var completion AgentCompletion

	sl := NewSessionLifecycle(func(c AgentCompletion) {
		completion = c
	})
	sl.TrackAgent(types.AgentClaude)

	RunAgentWithLifecycle(sl, types.AgentClaude, func() (types.AgentStatus, *types.TokenUsage, error) {
		panic("segfault in agent code")
	})

	if completion.Status != types.StatusFailed {
		t.Errorf("status = %s, want failed after panic", completion.Status)
	}
	if completion.Error != "panic: segfault in agent code" {
		t.Errorf("error = %q, want panic message", completion.Error)
	}
}

func TestLifecycleConcurrentComplete(t *testing.T) {
	var count atomic.Int32

	sl := NewSessionLifecycle(func(c AgentCompletion) {
		count.Add(1)
	})

	agents := []types.AgentID{types.AgentClaude, types.AgentCodex, types.AgentGemini, types.AgentVibe}
	for _, a := range agents {
		sl.TrackAgent(a)
	}

	// Complete all agents concurrently
	var wg sync.WaitGroup
	for _, a := range agents {
		wg.Add(1)
		go func(agent types.AgentID) {
			defer wg.Done()
			sl.Complete(agent, types.StatusCompleted, nil, "")
		}(a)
	}
	wg.Wait()

	if got := count.Load(); got != 4 {
		t.Fatalf("onComplete called %d times, want 4", got)
	}
}

func TestLifecycleConcurrentDoubleComplete(t *testing.T) {
	var count atomic.Int32

	sl := NewSessionLifecycle(func(c AgentCompletion) {
		count.Add(1)
	})

	sl.TrackAgent(types.AgentClaude)

	// 10 goroutines all try to complete the same agent
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sl.Complete(types.AgentClaude, types.StatusCompleted, nil, "")
		}()
	}
	wg.Wait()

	if got := count.Load(); got != 1 {
		t.Fatalf("onComplete called %d times, want 1 (idempotent)", got)
	}
}
