package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/adhocore/gronx"
)

// CronParser validates and evaluates cron expressions.
type CronParser struct {
	gron *gronx.Gronx
}

// NewCronParser creates a new parser instance.
func NewCronParser() *CronParser {
	return &CronParser{
		gron: gronx.New(),
	}
}

// ValidateCronExpression validates a 5-field cron expression.
func ValidateCronExpression(expr string) error {
	return NewCronParser().Validate(expr)
}

// NextRun returns the next scheduled run after from.
func NextRun(expr string, from time.Time) (time.Time, error) {
	return NewCronParser().NextRun(expr, from)
}

// Validate checks expression syntax and field count.
func (p *CronParser) Validate(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("cron expression cannot be empty")
	}

	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return fmt.Errorf("cron expression must have 5 fields, got %d", len(parts))
	}

	if !p.gron.IsValid(expr) {
		return fmt.Errorf("invalid cron expression: %q", expr)
	}
	return nil
}

// NextRun calculates the next tick strictly after from.
func (p *CronParser) NextRun(expr string, from time.Time) (time.Time, error) {
	if err := p.Validate(expr); err != nil {
		return time.Time{}, err
	}

	if from.IsZero() {
		from = time.Now()
	}

	next, err := gronx.NextTickAfter(expr, from, false)
	if err != nil {
		return time.Time{}, fmt.Errorf("calculate next run for %q: %w", expr, err)
	}
	return next, nil
}
