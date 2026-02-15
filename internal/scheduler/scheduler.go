package scheduler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"time"
)

const (
	// DefaultSchedulesVersion is the schema version persisted in schedule config JSON.
	DefaultSchedulesVersion = 1

	// TaskIDPattern restricts task IDs to safe characters for paths and shell usage.
	TaskIDPattern = `^[a-zA-Z0-9-]+$`

	// DefaultWorktreeBranchPrefix is the deterministic prefix for scheduled worktree branches.
	DefaultWorktreeBranchPrefix = "council-scheduled/"

	defaultConfigFileMode = 0o600
	defaultDirMode        = 0o700
)

var (
	taskIDRe = regexp.MustCompile(TaskIDPattern)

	// ErrTaskNotFound indicates the requested scheduled task does not exist.
	ErrTaskNotFound = errors.New("scheduled task not found")

	// ErrTaskExists indicates a task with the same ID already exists.
	ErrTaskExists = errors.New("scheduled task already exists")
)

// ScheduledTask is the persisted schedule configuration for one council task.
type ScheduledTask struct {
	ID                   string    `json:"id"`
	Prompt               string    `json:"prompt"`
	Cron                 string    `json:"cron,omitempty"`
	Agents               []string  `json:"agents,omitempty"`
	Mode                 string    `json:"mode,omitempty"`
	Workspace            string    `json:"workspace,omitempty"`
	Worktree             bool      `json:"worktree"`
	WorktreeRemote       string    `json:"worktreeRemote,omitempty"`
	WorktreeBranchPrefix string    `json:"worktreeBranchPrefix,omitempty"`
	Timeout              int       `json:"timeout,omitempty"`
	Enabled              bool      `json:"enabled"`
	CreatedAt            time.Time `json:"createdAt"`
}

// SchedulesConfig is the root config structure persisted to council-schedules.json.
type SchedulesConfig struct {
	Version int             `json:"version"`
	Tasks   []ScheduledTask `json:"tasks"`
}

// Scheduler provides CRUD operations backed by a persisted JSON config file.
type Scheduler struct {
	mu         sync.Mutex
	configPath string
	config     SchedulesConfig
	now        func() time.Time
}

// NewScheduler loads or creates a scheduler store at configPath.
// If configPath is empty, the global path (~/.claude/council-schedules.json) is used.
func NewScheduler(configPath string) (*Scheduler, error) {
	if configPath == "" {
		var err error
		configPath, err = DefaultConfigPath(false, "")
		if err != nil {
			return nil, err
		}
	}

	cfg, err := loadSchedulesConfig(configPath)
	if err != nil {
		return nil, err
	}

	return &Scheduler{
		configPath: configPath,
		config:     cfg,
		now:        time.Now,
	}, nil
}

// DefaultConfigPath returns the schedule config path for global or project scope.
func DefaultConfigPath(projectScoped bool, workspace string) (string, error) {
	if projectScoped {
		if workspace == "" {
			var err error
			workspace, err = os.Getwd()
			if err != nil {
				return "", fmt.Errorf("get cwd: %w", err)
			}
		}
		return filepath.Join(workspace, ".claude", "council-schedules.json"), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(homeDir, ".claude", "council-schedules.json"), nil
}

// ValidateTaskID enforces the strict security-safe task ID format.
func ValidateTaskID(taskID string) error {
	if !taskIDRe.MatchString(taskID) {
		return fmt.Errorf("invalid task id %q: must match %s", taskID, TaskIDPattern)
	}
	return nil
}

// ConfigPath returns the configured persistence path.
func (s *Scheduler) ConfigPath() string {
	return s.configPath
}

// Add inserts a new scheduled task and persists config.
func (s *Scheduler) Add(task ScheduledTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateAndNormalizeTask(&task, s.now(), true); err != nil {
		return err
	}

	if _, ok := s.taskByID(task.ID); ok {
		return fmt.Errorf("%w: %s", ErrTaskExists, task.ID)
	}

	s.config.Tasks = append(s.config.Tasks, task)
	return s.saveLocked()
}

// Remove deletes a scheduled task by ID and persists config.
func (s *Scheduler) Remove(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ValidateTaskID(taskID); err != nil {
		return err
	}

	idx := s.indexOfTask(taskID)
	if idx < 0 {
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}

	s.config.Tasks = slices.Delete(s.config.Tasks, idx, idx+1)
	return s.saveLocked()
}

// Pause disables a task without removing it.
func (s *Scheduler) Pause(taskID string) error {
	return s.setEnabled(taskID, false)
}

// Resume enables a previously paused task.
func (s *Scheduler) Resume(taskID string) error {
	return s.setEnabled(taskID, true)
}

// List returns a copy of all scheduled tasks.
func (s *Scheduler) List() []ScheduledTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]ScheduledTask, len(s.config.Tasks))
	copy(out, s.config.Tasks)
	for i := range out {
		out[i].Agents = append([]string(nil), out[i].Agents...)
	}
	return out
}

func (s *Scheduler) setEnabled(taskID string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ValidateTaskID(taskID); err != nil {
		return err
	}

	idx := s.indexOfTask(taskID)
	if idx < 0 {
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}

	s.config.Tasks[idx].Enabled = enabled
	return s.saveLocked()
}

func (s *Scheduler) taskByID(taskID string) (ScheduledTask, bool) {
	for _, task := range s.config.Tasks {
		if task.ID == taskID {
			return task, true
		}
	}
	return ScheduledTask{}, false
}

func (s *Scheduler) indexOfTask(taskID string) int {
	for i := range s.config.Tasks {
		if s.config.Tasks[i].ID == taskID {
			return i
		}
	}
	return -1
}

func (s *Scheduler) saveLocked() error {
	if s.config.Version == 0 {
		s.config.Version = DefaultSchedulesVersion
	}
	return saveJSONFile(s.configPath, s.config)
}

func loadSchedulesConfig(configPath string) (SchedulesConfig, error) {
	cfg := SchedulesConfig{
		Version: DefaultSchedulesVersion,
		Tasks:   []ScheduledTask{},
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return SchedulesConfig{}, fmt.Errorf("read schedules config: %w", err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return SchedulesConfig{}, fmt.Errorf("decode schedules config: %w", err)
	}

	if cfg.Version == 0 {
		cfg.Version = DefaultSchedulesVersion
	}
	if cfg.Tasks == nil {
		cfg.Tasks = []ScheduledTask{}
	}

	seen := make(map[string]struct{}, len(cfg.Tasks))
	for i := range cfg.Tasks {
		if err := validateAndNormalizeTask(&cfg.Tasks[i], time.Now(), false); err != nil {
			return SchedulesConfig{}, err
		}
		if _, ok := seen[cfg.Tasks[i].ID]; ok {
			return SchedulesConfig{}, fmt.Errorf("duplicate task id in config: %s", cfg.Tasks[i].ID)
		}
		seen[cfg.Tasks[i].ID] = struct{}{}
	}

	return cfg, nil
}

func validateAndNormalizeTask(task *ScheduledTask, now time.Time, defaultEnabled bool) error {
	if err := ValidateTaskID(task.ID); err != nil {
		return err
	}

	if task.Cron != "" {
		if err := ValidateCronExpression(task.Cron); err != nil {
			return fmt.Errorf("invalid cron for task %s: %w", task.ID, err)
		}
	}

	if task.CreatedAt.IsZero() {
		task.CreatedAt = now.UTC()
	}
	if defaultEnabled && !task.Enabled {
		task.Enabled = true
	}
	if task.Worktree {
		if task.WorktreeRemote == "" {
			task.WorktreeRemote = "origin"
		}
		if task.WorktreeBranchPrefix == "" {
			task.WorktreeBranchPrefix = DefaultWorktreeBranchPrefix
		}
	}
	task.Agents = append([]string(nil), task.Agents...)
	return nil
}

func saveJSONFile(path string, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), defaultDirMode); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	payload = append(payload, '\n')

	if err := writeFileAtomic(path, payload, defaultConfigFileMode); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
