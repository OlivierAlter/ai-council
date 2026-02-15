package scheduler

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const crontabMarkerPrefix = "# council-schedule:"

// CrontabRunner executes crontab commands with optional stdin input.
type CrontabRunner func(ctx context.Context, stdin string, args ...string) (string, error)

// DefaultCrontabRunner runs crontab through os/exec.
func DefaultCrontabRunner(ctx context.Context, stdin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "crontab", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// BuildCrontabEntry builds a managed marker + cron entry pair for a task.
func BuildCrontabEntry(taskID, cronExpr, councilBinaryPath, logDir string) (string, error) {
	if err := ValidateTaskID(taskID); err != nil {
		return "", err
	}
	if err := ValidateCronExpression(cronExpr); err != nil {
		return "", err
	}

	if councilBinaryPath == "" {
		councilBinaryPath = "council"
	}
	if logDir == "" {
		logDir = "~/.claude/logs"
	}

	logPath := filepath.Join(logDir, fmt.Sprintf("council-schedule-%s.log", taskID))
	command := fmt.Sprintf("%s %s schedule run %s >> %s 2>&1", cronExpr, shellQuote(councilBinaryPath), taskID, shellQuote(logPath))
	marker := crontabMarkerPrefix + taskID
	return marker + "\n" + command, nil
}

// UpsertCrontabTask adds or replaces a managed task entry in crontab content.
func UpsertCrontabTask(crontabContent, taskID, cronExpr, councilBinaryPath, logDir string) (string, error) {
	entry, err := BuildCrontabEntry(taskID, cronExpr, councilBinaryPath, logDir)
	if err != nil {
		return "", err
	}

	trimmed, _, err := RemoveCrontabTask(crontabContent, taskID)
	if err != nil {
		return "", err
	}

	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return entry + "\n", nil
	}
	return trimmed + "\n" + entry + "\n", nil
}

// RemoveCrontabTask removes a managed task entry by ID from crontab content.
func RemoveCrontabTask(crontabContent, taskID string) (string, bool, error) {
	if err := ValidateTaskID(taskID); err != nil {
		return "", false, err
	}
	marker := crontabMarkerPrefix + taskID

	lines := strings.Split(strings.ReplaceAll(crontabContent, "\r\n", "\n"), "\n")
	filtered := make([]string, 0, len(lines))
	removed := false

	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if line == marker {
			removed = true
			i++
			if i < len(lines) {
				next := strings.TrimSpace(lines[i])
				if next != "" && !strings.HasPrefix(next, crontabMarkerPrefix) {
					i++
				}
			}
			continue
		}
		filtered = append(filtered, lines[i])
		i++
	}

	result := strings.TrimSpace(strings.Join(filtered, "\n"))
	if result == "" {
		return "", removed, nil
	}
	return result + "\n", removed, nil
}

// ManagedTaskIDs returns all scheduler-managed task IDs present in crontab content.
func ManagedTaskIDs(crontabContent string) []string {
	lines := strings.Split(strings.ReplaceAll(crontabContent, "\r\n", "\n"), "\n")
	ids := make([]string, 0)
	seen := make(map[string]struct{})

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, crontabMarkerPrefix) {
			continue
		}
		id := strings.TrimPrefix(line, crontabMarkerPrefix)
		if id == "" {
			continue
		}
		if err := ValidateTaskID(id); err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

// InstallCrontabTask installs or updates one managed task entry in the system crontab.
func InstallCrontabTask(
	ctx context.Context,
	taskID string,
	cronExpr string,
	councilBinaryPath string,
	logDir string,
	runner CrontabRunner,
) error {
	if runner == nil {
		runner = DefaultCrontabRunner
	}

	current, err := readSystemCrontab(ctx, runner)
	if err != nil {
		return err
	}

	updated, err := UpsertCrontabTask(current, taskID, cronExpr, councilBinaryPath, logDir)
	if err != nil {
		return err
	}

	if _, err := runner(ctx, updated, "-"); err != nil {
		return fmt.Errorf("write crontab failed: %w", err)
	}
	return nil
}

// UninstallCrontabTask removes one managed task entry from the system crontab.
func UninstallCrontabTask(ctx context.Context, taskID string, runner CrontabRunner) error {
	if runner == nil {
		runner = DefaultCrontabRunner
	}

	current, err := readSystemCrontab(ctx, runner)
	if err != nil {
		return err
	}

	updated, _, err := RemoveCrontabTask(current, taskID)
	if err != nil {
		return err
	}

	if _, err := runner(ctx, updated, "-"); err != nil {
		return fmt.Errorf("write crontab failed: %w", err)
	}
	return nil
}

func readSystemCrontab(ctx context.Context, runner CrontabRunner) (string, error) {
	output, err := runner(ctx, "", "-l")
	if err != nil {
		combined := strings.ToLower(strings.TrimSpace(output + " " + err.Error()))
		if strings.Contains(combined, "no crontab for") {
			return "", nil
		}
		return "", fmt.Errorf("read crontab failed: %w (%s)", err, strings.TrimSpace(output))
	}
	return output, nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n'\"\\$`!&|;<>(){}[]*?~") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
