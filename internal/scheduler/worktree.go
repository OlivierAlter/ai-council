package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultWorktreeDirName = ".council-worktrees"
)

// GitRunner executes git commands.
type GitRunner interface {
	Run(ctx context.Context, repoPath string, args ...string) (string, error)
}

// ExecGitRunner executes git commands via os/exec.
type ExecGitRunner struct{}

// Run executes a git command and returns stdout/stderr output.
func (ExecGitRunner) Run(ctx context.Context, repoPath string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", commandArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

// WorktreeSession tracks the lifecycle of one scheduled run worktree.
type WorktreeSession struct {
	TaskID    string
	RepoPath  string
	Path      string
	Branch    string
	CreatedAt time.Time
	Preserved bool
}

// WorktreeManager creates and finalizes isolated git worktrees.
type WorktreeManager struct {
	RootDir string
	Runner  GitRunner
	Now     func() time.Time
}

// NewWorktreeManager creates a manager with sane defaults.
func NewWorktreeManager(rootDir string, runner GitRunner) (*WorktreeManager, error) {
	if rootDir == "" {
		var err error
		rootDir, err = DefaultWorktreeRootDir()
		if err != nil {
			return nil, err
		}
	}
	if runner == nil {
		runner = ExecGitRunner{}
	}
	return &WorktreeManager{
		RootDir: rootDir,
		Runner:  runner,
		Now:     time.Now,
	}, nil
}

// DefaultWorktreeRootDir resolves ~/.council-worktrees.
func DefaultWorktreeRootDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(homeDir, defaultWorktreeDirName), nil
}

// DeterministicBranchName returns council-scheduled/<task-id>-<YYYY-MM-DD>.
func DeterministicBranchName(prefix, taskID string, date time.Time) (string, error) {
	if err := ValidateTaskID(taskID); err != nil {
		return "", err
	}
	if prefix == "" {
		prefix = DefaultWorktreeBranchPrefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return fmt.Sprintf("%s%s-%s", prefix, taskID, date.UTC().Format("2006-01-02")), nil
}

// Create creates an isolated worktree and branch for a scheduled task.
func (m *WorktreeManager) Create(
	ctx context.Context,
	repoPath string,
	taskID string,
	branchPrefix string,
) (*WorktreeSession, error) {
	if err := ValidateTaskID(taskID); err != nil {
		return nil, err
	}
	if m.Runner == nil {
		m.Runner = ExecGitRunner{}
	}
	if m.Now == nil {
		m.Now = time.Now
	}

	now := m.Now().UTC()
	branch, err := DeterministicBranchName(branchPrefix, taskID, now)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(m.RootDir, 0o700); err != nil {
		return nil, fmt.Errorf("create worktree root dir: %w", err)
	}

	worktreePath := filepath.Join(m.RootDir, fmt.Sprintf("%s-%d", taskID, now.Unix()))
	if _, err := m.Runner.Run(ctx, repoPath, "worktree", "add", "-b", branch, worktreePath); err != nil {
		return nil, err
	}

	return &WorktreeSession{
		TaskID:    taskID,
		RepoPath:  repoPath,
		Path:      worktreePath,
		Branch:    branch,
		CreatedAt: now,
	}, nil
}

// HasChanges reports whether the worktree contains uncommitted changes.
func (m *WorktreeManager) HasChanges(ctx context.Context, session *WorktreeSession) (bool, error) {
	if session == nil {
		return false, fmt.Errorf("session is nil")
	}
	output, err := m.Runner.Run(ctx, session.Path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

// CommitPushAndCleanup commits worktree changes, pushes branch, and cleans up on success.
// If push fails, the worktree is preserved for manual inspection.
func (m *WorktreeManager) CommitPushAndCleanup(
	ctx context.Context,
	session *WorktreeSession,
	prompt string,
	remote string,
) (bool, error) {
	if session == nil {
		return false, fmt.Errorf("session is nil")
	}
	if remote == "" {
		remote = "origin"
	}

	changed, err := m.HasChanges(ctx, session)
	if err != nil {
		session.Preserved = true
		return false, err
	}
	if !changed {
		return false, m.Cleanup(ctx, session)
	}

	if _, err := m.Runner.Run(ctx, session.Path, "add", "-A"); err != nil {
		session.Preserved = true
		return true, err
	}

	commitMessage := BuildScheduledCommitMessage(session.TaskID, prompt)
	if _, err := m.Runner.Run(ctx, session.Path, "commit", "-m", commitMessage); err != nil {
		session.Preserved = true
		return true, err
	}

	if _, err := m.Runner.Run(ctx, session.Path, "push", remote, session.Branch); err != nil {
		session.Preserved = true
		return true, fmt.Errorf("push failed, worktree preserved at %s: %w", session.Path, err)
	}

	if err := m.Cleanup(ctx, session); err != nil {
		session.Preserved = true
		return true, err
	}
	return true, nil
}

// Cleanup removes a worktree from its parent repository.
func (m *WorktreeManager) Cleanup(ctx context.Context, session *WorktreeSession) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	if _, err := m.Runner.Run(ctx, session.RepoPath, "worktree", "remove", session.Path, "--force"); err != nil {
		return err
	}
	return nil
}

// BuildScheduledCommitMessage builds the deterministic scheduled commit message.
func BuildScheduledCommitMessage(taskID, prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "Scheduled council run"
	}
	prompt = strings.Join(strings.Fields(prompt), " ")
	if len(prompt) > 120 {
		prompt = prompt[:120] + "..."
	}
	return fmt.Sprintf("[council-scheduled] %s: %s", taskID, prompt)
}
