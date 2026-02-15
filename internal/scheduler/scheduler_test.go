package scheduler

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateTaskID(t *testing.T) {
	valid := []string{"sec-review", "abc123", "A1-B2-C3", "task"}
	for _, id := range valid {
		if err := ValidateTaskID(id); err != nil {
			t.Fatalf("expected %q valid, got error: %v", id, err)
		}
	}

	invalid := []string{"", "task id", "../task", "task_", "task$", "a/b"}
	for _, id := range invalid {
		if err := ValidateTaskID(id); err == nil {
			t.Fatalf("expected %q invalid", id)
		}
	}
}

func TestSchedulerAddListPauseResumeRemove(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "council-schedules.json")
	s, err := NewScheduler(configPath)
	if err != nil {
		t.Fatalf("NewScheduler() error: %v", err)
	}
	s.now = func() time.Time { return time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC) }

	task := ScheduledTask{
		ID:        "sec-review",
		Prompt:    "Review commits",
		Cron:      "0 9 * * 1-5",
		Agents:    []string{"claude", "codex"},
		Mode:      "standard",
		Workspace: "/repo",
		Worktree:  true,
	}
	if err := s.Add(task); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	tasks := s.List()
	if len(tasks) != 1 {
		t.Fatalf("List() got %d tasks, want 1", len(tasks))
	}

	got := tasks[0]
	if !got.Enabled {
		t.Fatalf("task should be enabled by default")
	}
	if got.WorktreeRemote != "origin" {
		t.Fatalf("WorktreeRemote = %q, want origin", got.WorktreeRemote)
	}
	if got.WorktreeBranchPrefix != DefaultWorktreeBranchPrefix {
		t.Fatalf("WorktreeBranchPrefix = %q, want %q", got.WorktreeBranchPrefix, DefaultWorktreeBranchPrefix)
	}
	if got.CreatedAt.IsZero() {
		t.Fatalf("CreatedAt should be set")
	}

	if err := s.Pause("sec-review"); err != nil {
		t.Fatalf("Pause() error: %v", err)
	}
	if s.List()[0].Enabled {
		t.Fatalf("task should be paused")
	}

	if err := s.Resume("sec-review"); err != nil {
		t.Fatalf("Resume() error: %v", err)
	}
	if !s.List()[0].Enabled {
		t.Fatalf("task should be resumed")
	}

	if err := s.Remove("sec-review"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
	if len(s.List()) != 0 {
		t.Fatalf("expected no tasks after remove")
	}
}

func TestSchedulerPersistenceAndDuplicate(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "council-schedules.json")

	s, err := NewScheduler(configPath)
	if err != nil {
		t.Fatalf("NewScheduler() error: %v", err)
	}
	if err := s.Add(ScheduledTask{
		ID:     "daily-review",
		Prompt: "Run review",
		Cron:   "0 9 * * 1-5",
	}); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	if err := s.Add(ScheduledTask{
		ID:     "daily-review",
		Prompt: "Duplicate",
		Cron:   "0 10 * * *",
	}); !errors.Is(err, ErrTaskExists) {
		t.Fatalf("expected ErrTaskExists, got %v", err)
	}

	s2, err := NewScheduler(configPath)
	if err != nil {
		t.Fatalf("NewScheduler() reload error: %v", err)
	}
	tasks := s2.List()
	if len(tasks) != 1 {
		t.Fatalf("reloaded task count = %d, want 1", len(tasks))
	}
	if tasks[0].ID != "daily-review" {
		t.Fatalf("reloaded task id = %q, want daily-review", tasks[0].ID)
	}
}

func TestSchedulerPausePersistsDisabledState(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "council-schedules.json")

	s, err := NewScheduler(configPath)
	if err != nil {
		t.Fatalf("NewScheduler() error: %v", err)
	}
	if err := s.Add(ScheduledTask{
		ID:     "nightly",
		Prompt: "Nightly run",
		Cron:   "0 0 * * *",
	}); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if err := s.Pause("nightly"); err != nil {
		t.Fatalf("Pause() error: %v", err)
	}

	reloaded, err := NewScheduler(configPath)
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	if reloaded.List()[0].Enabled {
		t.Fatalf("paused task should stay disabled after reload")
	}
}
