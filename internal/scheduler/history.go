package scheduler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	// DefaultHistoryVersion is the JSON schema version for execution history.
	DefaultHistoryVersion = 1

	// DefaultHistoryRetention keeps last N records per task.
	DefaultHistoryRetention = 100
)

// ExecutionStatus is the outcome status of one scheduled task execution.
type ExecutionStatus string

const (
	ExecutionStatusSuccess ExecutionStatus = "success"
	ExecutionStatusFailed  ExecutionStatus = "failed"
	ExecutionStatusTimeout ExecutionStatus = "timeout"
)

// ExecutionRecord represents one scheduled task run.
type ExecutionRecord struct {
	TaskID         string          `json:"taskID"`
	Timestamp      time.Time       `json:"timestamp"`
	Status         ExecutionStatus `json:"status"`
	DurationMillis int64           `json:"durationMillis"`
	OutputPath     string          `json:"outputPath,omitempty"`
}

// HistoryConfig is the persisted execution history file structure.
type HistoryConfig struct {
	Version int                          `json:"version"`
	Records map[string][]ExecutionRecord `json:"records"`
}

// HistoryStore persists and queries per-task execution records.
type HistoryStore struct {
	mu        sync.Mutex
	path      string
	data      HistoryConfig
	now       func() time.Time
	retention int
}

// NewHistoryStore loads or creates a history store.
// If path is empty, ~/.claude/council-schedule-history.json is used.
func NewHistoryStore(path string) (*HistoryStore, error) {
	if path == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		path = filepath.Join(homeDir, ".claude", "council-schedule-history.json")
	}

	data, err := loadHistory(path)
	if err != nil {
		return nil, err
	}

	return &HistoryStore{
		path:      path,
		data:      data,
		now:       time.Now,
		retention: DefaultHistoryRetention,
	}, nil
}

// Add appends one execution record and applies retention.
func (h *HistoryStore) Add(record ExecutionRecord) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := ValidateTaskID(record.TaskID); err != nil {
		return err
	}
	if record.Timestamp.IsZero() {
		record.Timestamp = h.now().UTC()
	}

	records := append(h.data.Records[record.TaskID], record)
	if h.retention <= 0 {
		h.retention = DefaultHistoryRetention
	}
	if len(records) > h.retention {
		records = records[len(records)-h.retention:]
	}
	h.data.Records[record.TaskID] = records
	return h.saveLocked()
}

// List returns records for a specific task or all tasks when taskID is empty.
// Results are sorted newest-first. Set last > 0 to cap results.
func (h *HistoryStore) List(taskID string, last int) ([]ExecutionRecord, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var out []ExecutionRecord
	if taskID != "" {
		if err := ValidateTaskID(taskID); err != nil {
			return nil, err
		}
		out = append(out, h.data.Records[taskID]...)
	} else {
		for _, records := range h.data.Records {
			out = append(out, records...)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})

	if last > 0 && len(out) > last {
		out = out[:last]
	}
	return out, nil
}

func (h *HistoryStore) saveLocked() error {
	if h.data.Version == 0 {
		h.data.Version = DefaultHistoryVersion
	}
	if h.data.Records == nil {
		h.data.Records = map[string][]ExecutionRecord{}
	}
	return saveJSONFile(h.path, h.data)
}

func loadHistory(path string) (HistoryConfig, error) {
	data := HistoryConfig{
		Version: DefaultHistoryVersion,
		Records: map[string][]ExecutionRecord{},
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return data, nil
		}
		return HistoryConfig{}, fmt.Errorf("read history file: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return data, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil {
		return HistoryConfig{}, fmt.Errorf("decode history file: %w", err)
	}

	if data.Version == 0 {
		data.Version = DefaultHistoryVersion
	}
	if data.Records == nil {
		data.Records = map[string][]ExecutionRecord{}
	}

	for taskID, records := range data.Records {
		if err := ValidateTaskID(taskID); err != nil {
			return HistoryConfig{}, err
		}
		for i := range records {
			if records[i].TaskID == "" {
				records[i].TaskID = taskID
			}
			if records[i].TaskID != taskID {
				return HistoryConfig{}, fmt.Errorf("taskID mismatch in history record for %s", taskID)
			}
		}
		data.Records[taskID] = records
	}

	return data, nil
}
