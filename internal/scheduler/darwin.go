package scheduler

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const launchdLabelPrefix = "com.council.schedule."

// CalendarInterval mirrors launchd's StartCalendarInterval dictionary fields.
type CalendarInterval struct {
	Minute  *int
	Hour    *int
	Day     *int
	Month   *int
	Weekday *int
}

// LaunchctlRunner executes launchctl commands.
type LaunchctlRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// DefaultLaunchctlRunner runs launchctl through os/exec.
func DefaultLaunchctlRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// LaunchdLabel returns the launchd job label for a task.
func LaunchdLabel(taskID string) (string, error) {
	if err := ValidateTaskID(taskID); err != nil {
		return "", err
	}
	return launchdLabelPrefix + taskID, nil
}

// LaunchAgentPlistPath returns ~/Library/LaunchAgents plist path for a task.
func LaunchAgentPlistPath(homeDir, taskID string) (string, error) {
	if err := ValidateTaskID(taskID); err != nil {
		return "", err
	}
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
	}
	label, _ := LaunchdLabel(taskID)
	return filepath.Join(homeDir, "Library", "LaunchAgents", label+".plist"), nil
}

// CronToCalendarIntervals converts 5-field cron into launchd calendar intervals.
func CronToCalendarIntervals(expr string) ([]CalendarInterval, error) {
	if err := ValidateCronExpression(expr); err != nil {
		return nil, err
	}

	fields := strings.Fields(expr)

	minuteVals, minuteStar, err := parseCronField(fields[0], 0, 59, false)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field: %w", err)
	}
	hourVals, hourStar, err := parseCronField(fields[1], 0, 23, false)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field: %w", err)
	}
	dayVals, dayStar, err := parseCronField(fields[2], 1, 31, false)
	if err != nil {
		return nil, fmt.Errorf("invalid day field: %w", err)
	}
	monthVals, monthStar, err := parseCronField(fields[3], 1, 12, false)
	if err != nil {
		return nil, fmt.Errorf("invalid month field: %w", err)
	}
	weekdayVals, weekdayStar, err := parseCronField(fields[4], 0, 7, true)
	if err != nil {
		return nil, fmt.Errorf("invalid weekday field: %w", err)
	}

	intervals := []CalendarInterval{{}}
	if !minuteStar {
		intervals = expandIntervals(intervals, minuteVals, func(ci *CalendarInterval, v int) {
			val := v
			ci.Minute = &val
		})
	}
	if !hourStar {
		intervals = expandIntervals(intervals, hourVals, func(ci *CalendarInterval, v int) {
			val := v
			ci.Hour = &val
		})
	}
	if !monthStar {
		intervals = expandIntervals(intervals, monthVals, func(ci *CalendarInterval, v int) {
			val := v
			ci.Month = &val
		})
	}

	switch {
	case dayStar && weekdayStar:
		// No additional key required.
	case !dayStar && weekdayStar:
		intervals = expandIntervals(intervals, dayVals, func(ci *CalendarInterval, v int) {
			val := v
			ci.Day = &val
		})
	case dayStar && !weekdayStar:
		intervals = expandIntervals(intervals, weekdayVals, func(ci *CalendarInterval, v int) {
			val := v
			ci.Weekday = &val
		})
	default:
		// Cron day-of-month and day-of-week use OR semantics.
		dayIntervals := expandIntervals(intervals, dayVals, func(ci *CalendarInterval, v int) {
			val := v
			ci.Day = &val
		})
		weekdayIntervals := expandIntervals(intervals, weekdayVals, func(ci *CalendarInterval, v int) {
			val := v
			ci.Weekday = &val
		})
		intervals = append(dayIntervals, weekdayIntervals...)
	}

	return intervals, nil
}

// GenerateLaunchdPlist builds plist XML for a scheduled task.
func GenerateLaunchdPlist(taskID, cronExpr, councilBinaryPath, logDir string) ([]byte, error) {
	label, err := LaunchdLabel(taskID)
	if err != nil {
		return nil, err
	}

	if councilBinaryPath == "" {
		councilBinaryPath = "council"
	}
	if logDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		logDir = filepath.Join(homeDir, ".claude", "logs")
	}

	intervals, err := CronToCalendarIntervals(cronExpr)
	if err != nil {
		return nil, err
	}

	stdoutPath := filepath.Join(logDir, fmt.Sprintf("council-schedule-%s.log", taskID))
	stderrPath := filepath.Join(logDir, fmt.Sprintf("council-schedule-%s.error.log", taskID))

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n")
	b.WriteString(`<dict>` + "\n")
	b.WriteString("  <key>Label</key>\n")
	b.WriteString("  <string>" + xmlEscape(label) + "</string>\n")
	b.WriteString("  <key>ProgramArguments</key>\n")
	b.WriteString("  <array>\n")
	b.WriteString("    <string>" + xmlEscape(councilBinaryPath) + "</string>\n")
	b.WriteString("    <string>schedule</string>\n")
	b.WriteString("    <string>run</string>\n")
	b.WriteString("    <string>" + xmlEscape(taskID) + "</string>\n")
	b.WriteString("  </array>\n")
	b.WriteString("  <key>StartCalendarInterval</key>\n")
	b.WriteString(renderIntervalsXML(intervals))
	b.WriteString("  <key>StandardOutPath</key>\n")
	b.WriteString("  <string>" + xmlEscape(stdoutPath) + "</string>\n")
	b.WriteString("  <key>StandardErrorPath</key>\n")
	b.WriteString("  <string>" + xmlEscape(stderrPath) + "</string>\n")
	b.WriteString("</dict>\n")
	b.WriteString("</plist>\n")

	return []byte(b.String()), nil
}

// WriteLaunchdPlist writes plist to disk.
func WriteLaunchdPlist(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create launch agent dir: %w", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}
	return nil
}

// LaunchctlLoad loads a launchd plist.
func LaunchctlLoad(ctx context.Context, plistPath string, runner LaunchctlRunner) error {
	return runLaunchctl(ctx, runner, "load", plistPath)
}

// LaunchctlUnload unloads a launchd plist.
func LaunchctlUnload(ctx context.Context, plistPath string, runner LaunchctlRunner) error {
	return runLaunchctl(ctx, runner, "unload", plistPath)
}

func runLaunchctl(ctx context.Context, runner LaunchctlRunner, subcommand, plistPath string) error {
	if runner == nil {
		runner = DefaultLaunchctlRunner
	}

	output, err := runner(ctx, "launchctl", subcommand, plistPath)
	if err != nil {
		return fmt.Errorf("launchctl %s %s failed: %w (%s)", subcommand, plistPath, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func parseCronField(field string, min, max int, weekday bool) ([]int, bool, error) {
	field = strings.TrimSpace(field)
	if field == "*" {
		return nil, true, nil
	}

	values := make(map[int]struct{})
	parts := strings.Split(field, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false, fmt.Errorf("empty segment")
		}

		rangePart := part
		step := 1
		if strings.Contains(part, "/") {
			stepParts := strings.Split(part, "/")
			if len(stepParts) != 2 {
				return nil, false, fmt.Errorf("invalid step syntax %q", part)
			}
			rangePart = stepParts[0]
			parsedStep, err := strconv.Atoi(stepParts[1])
			if err != nil || parsedStep <= 0 {
				return nil, false, fmt.Errorf("invalid step %q", stepParts[1])
			}
			step = parsedStep
		}

		var start, end int
		switch {
		case rangePart == "*":
			start, end = min, max
		case strings.Contains(rangePart, "-"):
			rangeTokens := strings.Split(rangePart, "-")
			if len(rangeTokens) != 2 {
				return nil, false, fmt.Errorf("invalid range syntax %q", rangePart)
			}
			parsedStart, err := strconv.Atoi(rangeTokens[0])
			if err != nil {
				return nil, false, err
			}
			parsedEnd, err := strconv.Atoi(rangeTokens[1])
			if err != nil {
				return nil, false, err
			}
			start, end = parsedStart, parsedEnd
		default:
			single, err := strconv.Atoi(rangePart)
			if err != nil {
				return nil, false, err
			}
			start, end = single, single
		}

		if start < min || end > max || start > end {
			return nil, false, fmt.Errorf("value out of range: %d-%d (allowed %d-%d)", start, end, min, max)
		}

		for value := start; value <= end; value += step {
			normalized := value
			if weekday && normalized == 7 {
				normalized = 0
			}
			values[normalized] = struct{}{}
		}
	}

	out := make([]int, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Ints(out)
	return out, false, nil
}

func expandIntervals(
	base []CalendarInterval,
	values []int,
	setter func(*CalendarInterval, int),
) []CalendarInterval {
	out := make([]CalendarInterval, 0, len(base)*len(values))
	for _, interval := range base {
		for _, value := range values {
			next := cloneInterval(interval)
			setter(&next, value)
			out = append(out, next)
		}
	}
	return out
}

func cloneInterval(interval CalendarInterval) CalendarInterval {
	out := CalendarInterval{}
	if interval.Minute != nil {
		v := *interval.Minute
		out.Minute = &v
	}
	if interval.Hour != nil {
		v := *interval.Hour
		out.Hour = &v
	}
	if interval.Day != nil {
		v := *interval.Day
		out.Day = &v
	}
	if interval.Month != nil {
		v := *interval.Month
		out.Month = &v
	}
	if interval.Weekday != nil {
		v := *interval.Weekday
		out.Weekday = &v
	}
	return out
}

func renderIntervalsXML(intervals []CalendarInterval) string {
	var b strings.Builder
	if len(intervals) == 1 {
		b.WriteString(renderIntervalDictXML(intervals[0], 2))
		return b.String()
	}

	b.WriteString("  <array>\n")
	for _, interval := range intervals {
		b.WriteString(renderIntervalDictXML(interval, 4))
	}
	b.WriteString("  </array>\n")
	return b.String()
}

func renderIntervalDictXML(interval CalendarInterval, indent int) string {
	indentStr := strings.Repeat(" ", indent)
	var b strings.Builder
	b.WriteString(indentStr + "<dict>\n")

	writeKeyInt := func(key string, value *int) {
		if value == nil {
			return
		}
		b.WriteString(indentStr + "  <key>" + key + "</key>\n")
		b.WriteString(indentStr + "  <integer>" + strconv.Itoa(*value) + "</integer>\n")
	}

	writeKeyInt("Minute", interval.Minute)
	writeKeyInt("Hour", interval.Hour)
	writeKeyInt("Day", interval.Day)
	writeKeyInt("Month", interval.Month)
	writeKeyInt("Weekday", interval.Weekday)

	b.WriteString(indentStr + "</dict>\n")
	return b.String()
}

func xmlEscape(value string) string {
	var buf bytes.Buffer
	_ = xmlEscapeText(&buf, []byte(value))
	return buf.String()
}

func xmlEscapeText(dst *bytes.Buffer, text []byte) error {
	for _, b := range text {
		switch b {
		case '&':
			dst.WriteString("&amp;")
		case '<':
			dst.WriteString("&lt;")
		case '>':
			dst.WriteString("&gt;")
		case '\'':
			dst.WriteString("&apos;")
		case '"':
			dst.WriteString("&quot;")
		default:
			dst.WriteByte(b)
		}
	}
	return nil
}
