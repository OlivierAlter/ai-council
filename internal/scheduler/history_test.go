package scheduler

import (
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryAddListAndRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store, err := NewHistoryStore(path)
	if err != nil {
		t.Fatalf("NewHistoryStore() error: %v", err)
	}

	base := time.Date(2026, 2, 15, 8, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return base }
	store.retention = 3

	for i := 0; i < 5; i++ {
		err := store.Add(ExecutionRecord{
			TaskID:         "sec-review",
			Timestamp:      base.Add(time.Duration(i) * time.Minute),
			Status:         ExecutionStatusSuccess,
			DurationMillis: int64(1000 + i),
			OutputPath:     "/tmp/output",
		})
		if err != nil {
			t.Fatalf("Add() error: %v", err)
		}
	}

	records, err := store.List("sec-review", 0)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("retained records = %d, want 3", len(records))
	}
	if !records[0].Timestamp.Equal(base.Add(4 * time.Minute)) {
		t.Fatalf("newest timestamp mismatch: %s", records[0].Timestamp)
	}
	if !records[2].Timestamp.Equal(base.Add(2 * time.Minute)) {
		t.Fatalf("oldest retained timestamp mismatch: %s", records[2].Timestamp)
	}
}

func TestHistoryPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")

	store, err := NewHistoryStore(path)
	if err != nil {
		t.Fatalf("NewHistoryStore() error: %v", err)
	}
	if err := store.Add(ExecutionRecord{
		TaskID:         "dep-audit",
		Timestamp:      time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC),
		Status:         ExecutionStatusFailed,
		DurationMillis: 2500,
		OutputPath:     "/tmp/output/dep-audit",
	}); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	reloaded, err := NewHistoryStore(path)
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	records, err := reloaded.List("dep-audit", 0)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].Status != ExecutionStatusFailed {
		t.Fatalf("status = %q, want failed", records[0].Status)
	}
}

func TestHistoryListAllTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store, err := NewHistoryStore(path)
	if err != nil {
		t.Fatalf("NewHistoryStore() error: %v", err)
	}

	records := []ExecutionRecord{
		{
			TaskID:         "task-a",
			Timestamp:      time.Date(2026, 2, 15, 8, 0, 0, 0, time.UTC),
			Status:         ExecutionStatusSuccess,
			DurationMillis: 1000,
		},
		{
			TaskID:         "task-b",
			Timestamp:      time.Date(2026, 2, 15, 9, 0, 0, 0, time.UTC),
			Status:         ExecutionStatusSuccess,
			DurationMillis: 1000,
		},
		{
			TaskID:         "task-a",
			Timestamp:      time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC),
			Status:         ExecutionStatusTimeout,
			DurationMillis: 1000,
		},
	}
	for _, record := range records {
		if err := store.Add(record); err != nil {
			t.Fatalf("Add() error: %v", err)
		}
	}

	all, err := store.List("", 2)
	if err != nil {
		t.Fatalf("List(all) error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len(all) = %d, want 2", len(all))
	}
	if all[0].TaskID != "task-a" || all[0].Status != ExecutionStatusTimeout {
		t.Fatalf("unexpected first record: %+v", all[0])
	}
	if all[1].TaskID != "task-b" {
		t.Fatalf("unexpected second record: %+v", all[1])
	}
}
