package workflow

import (
	"context"
	"time"
)

// Step represents a step of execution(data processor).
type Step interface {
	// Name returns the name of the step.
	Name() string
	// Execute is the step central processing unit.
	// It accepts a context and a request.
	Execute(ctx context.Context, request any) error
	// CanRetry decides if the step can retry its execution.
	CanRetry() bool
}

// Logger is the workflow supported logger.
type Logger interface {
	Info(msg string)
	Error(msg string)
}

// StepConfig provides configuration for a Step of execution.
type StepConfig struct {
	Step                    Step
	ContinueWorkflowOnError bool
}

// RetryConfig is the config for the step retry
type RetryConfig struct {
	MaxRetryAttempts uint
	WaitBeforeRetry  time.Duration
}

type noOpLogger struct{}

// Info is the Info Level log.
func (n noOpLogger) Info(_ string) {}

// Error is the Error Level log.
func (n noOpLogger) Error(_ string) {}
