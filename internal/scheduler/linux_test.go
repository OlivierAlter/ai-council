package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBuildCrontabEntry(t *testing.T) {
	entry, err := BuildCrontabEntry("sec-review", "0 9 * * 1-5", "/usr/local/bin/council", "~/.claude/logs")
	if err != nil {
		t.Fatalf("BuildCrontabEntry() error: %v", err)
	}

	if !strings.Contains(entry, "# council-schedule:sec-review") {
		t.Fatalf("entry missing marker: %s", entry)
	}
	if !strings.Contains(entry, "0 9 * * 1-5 /usr/local/bin/council schedule run sec-review") {
		t.Fatalf("entry missing command: %s", entry)
	}
	if !strings.Contains(entry, "council-schedule-sec-review.log") {
		t.Fatalf("entry missing log file: %s", entry)
	}
}

func TestUpsertAndRemoveCrontabTask(t *testing.T) {
	existing := strings.Join([]string{
		"MAILTO=user@example.com",
		"# council-schedule:old-task",
		"0 8 * * * /usr/local/bin/council schedule run old-task >> ~/.claude/logs/council-schedule-old-task.log 2>&1",
		"",
	}, "\n")

	updated, err := UpsertCrontabTask(existing, "sec-review", "0 9 * * 1-5", "/usr/local/bin/council", "~/.claude/logs")
	if err != nil {
		t.Fatalf("UpsertCrontabTask() error: %v", err)
	}
	if !strings.Contains(updated, "# council-schedule:old-task") {
		t.Fatalf("upsert should keep existing tasks")
	}
	if !strings.Contains(updated, "# council-schedule:sec-review") {
		t.Fatalf("upsert should add new task")
	}

	replaced, err := UpsertCrontabTask(updated, "sec-review", "30 9 * * 1-5", "/usr/local/bin/council", "~/.claude/logs")
	if err != nil {
		t.Fatalf("UpsertCrontabTask() replace error: %v", err)
	}
	if strings.Count(replaced, "# council-schedule:sec-review") != 1 {
		t.Fatalf("expected single marker for replaced task\n%s", replaced)
	}
	if !strings.Contains(replaced, "30 9 * * 1-5") {
		t.Fatalf("replaced task should have new schedule")
	}

	removed, ok, err := RemoveCrontabTask(replaced, "sec-review")
	if err != nil {
		t.Fatalf("RemoveCrontabTask() error: %v", err)
	}
	if !ok {
		t.Fatalf("expected task to be removed")
	}
	if strings.Contains(removed, "sec-review") {
		t.Fatalf("removed content still references task: %s", removed)
	}
}

func TestManagedTaskIDs(t *testing.T) {
	content := strings.Join([]string{
		"# council-schedule:sec-review",
		"0 9 * * 1-5 ...",
		"# council-schedule:dep-audit",
		"0 10 * * * ...",
		"# council-schedule:sec-review",
		"",
	}, "\n")

	ids := ManagedTaskIDs(content)
	if len(ids) != 2 {
		t.Fatalf("len(ids) = %d, want 2", len(ids))
	}
	if ids[0] != "sec-review" || ids[1] != "dep-audit" {
		t.Fatalf("ids = %v", ids)
	}
}

func TestInstallAndUninstallCrontabTask(t *testing.T) {
	var writes []string
	var call int
	runner := func(ctx context.Context, stdin string, args ...string) (string, error) {
		call++
		switch call {
		case 1:
			if len(args) != 1 || args[0] != "-l" {
				t.Fatalf("first call args = %v, want -l", args)
			}
			return "", errors.New("no crontab for root")
		case 2:
			if len(args) != 1 || args[0] != "-" {
				t.Fatalf("second call args = %v, want -", args)
			}
			writes = append(writes, stdin)
			return "", nil
		case 3:
			return writes[0], nil
		case 4:
			writes = append(writes, stdin)
			return "", nil
		default:
			return "", nil
		}
	}

	if err := InstallCrontabTask(context.Background(), "sec-review", "0 9 * * 1-5", "/usr/local/bin/council", "~/.claude/logs", runner); err != nil {
		t.Fatalf("InstallCrontabTask() error: %v", err)
	}
	if len(writes) != 1 || !strings.Contains(writes[0], "# council-schedule:sec-review") {
		t.Fatalf("install write content unexpected: %v", writes)
	}

	if err := UninstallCrontabTask(context.Background(), "sec-review", runner); err != nil {
		t.Fatalf("UninstallCrontabTask() error: %v", err)
	}
	if len(writes) != 2 {
		t.Fatalf("expected second write for uninstall")
	}
	if strings.Contains(writes[1], "sec-review") {
		t.Fatalf("uninstall should remove task, got %q", writes[1])
	}
}
