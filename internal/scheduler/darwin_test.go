package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCronToCalendarIntervalsWeekdays(t *testing.T) {
	intervals, err := CronToCalendarIntervals("0 9 * * 1-5")
	if err != nil {
		t.Fatalf("CronToCalendarIntervals() error: %v", err)
	}
	if len(intervals) != 5 {
		t.Fatalf("len(intervals) = %d, want 5", len(intervals))
	}

	for i, interval := range intervals {
		if interval.Minute == nil || *interval.Minute != 0 {
			t.Fatalf("interval %d minute mismatch", i)
		}
		if interval.Hour == nil || *interval.Hour != 9 {
			t.Fatalf("interval %d hour mismatch", i)
		}
		if interval.Weekday == nil || *interval.Weekday != i+1 {
			t.Fatalf("interval %d weekday = %v, want %d", i, interval.Weekday, i+1)
		}
		if interval.Day != nil {
			t.Fatalf("interval %d should not have day", i)
		}
	}
}

func TestCronToCalendarIntervalsDayAndWeekdayOR(t *testing.T) {
	intervals, err := CronToCalendarIntervals("0 9 1 * 1")
	if err != nil {
		t.Fatalf("CronToCalendarIntervals() error: %v", err)
	}
	if len(intervals) != 2 {
		t.Fatalf("len(intervals) = %d, want 2", len(intervals))
	}

	var dayCount, weekdayCount int
	for _, interval := range intervals {
		if interval.Day != nil {
			dayCount++
		}
		if interval.Weekday != nil {
			weekdayCount++
		}
	}
	if dayCount != 1 || weekdayCount != 1 {
		t.Fatalf("expected 1 day interval and 1 weekday interval, got day=%d weekday=%d", dayCount, weekdayCount)
	}
}

func TestCronToCalendarIntervalsWeekdaySevenMapsToSunday(t *testing.T) {
	intervals, err := CronToCalendarIntervals("0 9 * * 7")
	if err != nil {
		t.Fatalf("CronToCalendarIntervals() error: %v", err)
	}
	if len(intervals) != 1 {
		t.Fatalf("len(intervals) = %d, want 1", len(intervals))
	}
	if intervals[0].Weekday == nil || *intervals[0].Weekday != 0 {
		t.Fatalf("weekday should map to 0 (Sunday), got %+v", intervals[0].Weekday)
	}
}

func TestGenerateLaunchdPlist(t *testing.T) {
	plist, err := GenerateLaunchdPlist("sec-review", "0 9 * * 1-5", "/usr/local/bin/council", "/tmp/logs")
	if err != nil {
		t.Fatalf("GenerateLaunchdPlist() error: %v", err)
	}
	content := string(plist)

	checks := []string{
		"<string>com.council.schedule.sec-review</string>",
		"<string>/usr/local/bin/council</string>",
		"<string>schedule</string>",
		"<string>run</string>",
		"<string>sec-review</string>",
		"<key>StartCalendarInterval</key>",
		"<array>",
		"<key>Weekday</key>",
		"/tmp/logs/council-schedule-sec-review.log",
		"/tmp/logs/council-schedule-sec-review.error.log",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Fatalf("plist missing %q\n%s", want, content)
		}
	}
}

func TestLaunchctlLoadUnload(t *testing.T) {
	var calls []string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil, nil
	}

	if err := LaunchctlLoad(context.Background(), "/tmp/task.plist", runner); err != nil {
		t.Fatalf("LaunchctlLoad() error: %v", err)
	}
	if err := LaunchctlUnload(context.Background(), "/tmp/task.plist", runner); err != nil {
		t.Fatalf("LaunchctlUnload() error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2", len(calls))
	}
	if calls[0] != "launchctl load /tmp/task.plist" {
		t.Fatalf("first call = %q", calls[0])
	}
	if calls[1] != "launchctl unload /tmp/task.plist" {
		t.Fatalf("second call = %q", calls[1])
	}
}

func TestLaunchctlLoadErrorIncludesOutput(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("permission denied"), errors.New("boom")
	}
	err := LaunchctlLoad(context.Background(), "/tmp/task.plist", runner)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("error should include command output, got %v", err)
	}
}
