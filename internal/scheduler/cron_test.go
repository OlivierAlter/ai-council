package scheduler

import (
	"testing"
	"time"
)

func TestValidateCronExpression(t *testing.T) {
	valid := []string{
		"0 9 * * 1-5",
		"*/15 * * * *",
		"0 0 1 * *",
	}
	for _, expr := range valid {
		if err := ValidateCronExpression(expr); err != nil {
			t.Fatalf("expected %q valid, got %v", expr, err)
		}
	}

	invalid := []string{
		"",
		"* * * *",        // wrong field count
		"* * * * * *",    // wrong field count for this scheduler
		"61 * * * *",     // invalid minute
		"* * * * monday", // invalid token
	}
	for _, expr := range invalid {
		if err := ValidateCronExpression(expr); err == nil {
			t.Fatalf("expected %q invalid", expr)
		}
	}
}

func TestNextRun(t *testing.T) {
	ref := time.Date(2026, 2, 15, 8, 30, 0, 0, time.UTC) // Sunday
	next, err := NextRun("0 9 * * 1-5", ref)
	if err != nil {
		t.Fatalf("NextRun() error: %v", err)
	}
	want := time.Date(2026, 2, 16, 9, 0, 0, 0, time.UTC) // Monday
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}

	ref2 := time.Date(2026, 2, 15, 10, 0, 30, 0, time.UTC)
	next2, err := NextRun("* * * * *", ref2)
	if err != nil {
		t.Fatalf("NextRun() error: %v", err)
	}
	want2 := time.Date(2026, 2, 15, 10, 1, 0, 0, time.UTC)
	if !next2.Equal(want2) {
		t.Fatalf("next2 = %s, want %s", next2, want2)
	}
}
