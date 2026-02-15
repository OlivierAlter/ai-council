package scheduler

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type gitCall struct {
	repoPath string
	args     []string
}

type fakeGitRunner struct {
	calls     []gitCall
	responses map[string]fakeGitResponse
}

type fakeGitResponse struct {
	output string
	err    error
}

func (f *fakeGitRunner) Run(ctx context.Context, repoPath string, args ...string) (string, error) {
	f.calls = append(f.calls, gitCall{repoPath: repoPath, args: append([]string(nil), args...)})
	key := repoPath + "|" + strings.Join(args, " ")
	if resp, ok := f.responses[key]; ok {
		return resp.output, resp.err
	}
	return "", nil
}

func TestDeterministicBranchName(t *testing.T) {
	branch, err := DeterministicBranchName("council-scheduled/", "sec-review", time.Date(2026, 2, 15, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DeterministicBranchName() error: %v", err)
	}
	if branch != "council-scheduled/sec-review-2026-02-15" {
		t.Fatalf("branch = %q", branch)
	}

	if _, err := DeterministicBranchName("", "bad/task", time.Now()); err == nil {
		t.Fatalf("expected invalid task id error")
	}
}

func TestWorktreeCreate(t *testing.T) {
	repoPath := "/repo"
	rootDir := t.TempDir()
	now := time.Date(2026, 2, 15, 8, 0, 0, 0, time.UTC)
	expectedPath := filepath.Join(rootDir, "sec-review-1771142400")
	expectedBranch := "council-scheduled/sec-review-2026-02-15"

	runner := &fakeGitRunner{responses: map[string]fakeGitResponse{}}
	runner.responses[repoPath+"|worktree add -b "+expectedBranch+" "+expectedPath] = fakeGitResponse{}

	manager, err := NewWorktreeManager(rootDir, runner)
	if err != nil {
		t.Fatalf("NewWorktreeManager() error: %v", err)
	}
	manager.Now = func() time.Time { return now }

	session, err := manager.Create(context.Background(), repoPath, "sec-review", DefaultWorktreeBranchPrefix)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if session.Path != expectedPath {
		t.Fatalf("session.Path = %q, want %q", session.Path, expectedPath)
	}
	if session.Branch != expectedBranch {
		t.Fatalf("session.Branch = %q, want %q", session.Branch, expectedBranch)
	}
}

func TestCommitPushAndCleanupSuccess(t *testing.T) {
	repoPath := "/repo"
	worktreePath := "/worktrees/sec-review-1"
	branch := "council-scheduled/sec-review-2026-02-15"
	runner := &fakeGitRunner{
		responses: map[string]fakeGitResponse{
			worktreePath + "|status --porcelain": {output: " M file.go\n"},
			worktreePath + "|add -A":             {},
			worktreePath + "|commit -m [council-scheduled] sec-review: Review commits": {},
			worktreePath + "|push origin " + branch:                                    {},
			repoPath + "|worktree remove " + worktreePath + " --force":                 {},
		},
	}

	manager, _ := NewWorktreeManager(t.TempDir(), runner)
	session := &WorktreeSession{
		TaskID:   "sec-review",
		RepoPath: repoPath,
		Path:     worktreePath,
		Branch:   branch,
	}

	changed, err := manager.CommitPushAndCleanup(context.Background(), session, "Review commits", "origin")
	if err != nil {
		t.Fatalf("CommitPushAndCleanup() error: %v", err)
	}
	if !changed {
		t.Fatalf("changed = false, want true")
	}
	if session.Preserved {
		t.Fatalf("session should not be preserved on success")
	}

	lastCall := runner.calls[len(runner.calls)-1]
	if strings.Join(lastCall.args, " ") != "worktree remove "+worktreePath+" --force" {
		t.Fatalf("last git call should cleanup worktree, got %v", lastCall.args)
	}
}

func TestCommitPushAndCleanupNoChanges(t *testing.T) {
	repoPath := "/repo"
	worktreePath := "/worktrees/sec-review-1"
	runner := &fakeGitRunner{
		responses: map[string]fakeGitResponse{
			worktreePath + "|status --porcelain":                       {output: ""},
			repoPath + "|worktree remove " + worktreePath + " --force": {},
		},
	}
	manager, _ := NewWorktreeManager(t.TempDir(), runner)

	session := &WorktreeSession{
		TaskID:   "sec-review",
		RepoPath: repoPath,
		Path:     worktreePath,
		Branch:   "council-scheduled/sec-review-2026-02-15",
	}
	changed, err := manager.CommitPushAndCleanup(context.Background(), session, "Prompt", "origin")
	if err != nil {
		t.Fatalf("CommitPushAndCleanup() error: %v", err)
	}
	if changed {
		t.Fatalf("changed = true, want false")
	}
}

func TestCommitPushAndCleanupPushFailurePreservesWorktree(t *testing.T) {
	repoPath := "/repo"
	worktreePath := "/worktrees/sec-review-1"
	branch := "council-scheduled/sec-review-2026-02-15"
	runner := &fakeGitRunner{
		responses: map[string]fakeGitResponse{
			worktreePath + "|status --porcelain":                               {output: " M file.go\n"},
			worktreePath + "|add -A":                                           {},
			worktreePath + "|commit -m [council-scheduled] sec-review: Prompt": {},
			worktreePath + "|push origin " + branch:                            {err: errors.New("remote rejected")},
		},
	}
	manager, _ := NewWorktreeManager(t.TempDir(), runner)

	session := &WorktreeSession{
		TaskID:   "sec-review",
		RepoPath: repoPath,
		Path:     worktreePath,
		Branch:   branch,
	}
	changed, err := manager.CommitPushAndCleanup(context.Background(), session, "Prompt", "origin")
	if err == nil {
		t.Fatalf("expected push failure")
	}
	if !changed {
		t.Fatalf("changed = false, want true")
	}
	if !session.Preserved {
		t.Fatalf("session should be preserved on push failure")
	}

	for _, call := range runner.calls {
		if strings.Join(call.args, " ") == "worktree remove "+worktreePath+" --force" {
			t.Fatalf("cleanup should not run on push failure")
		}
	}
}
