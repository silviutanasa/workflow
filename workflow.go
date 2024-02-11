package workflow

import (
	"context"
	"time"
)

// Step describes a step of execution.
type Step[T any] interface {
	// Name provides the identity of the step.
	Name() string
	// Execute is the step central processing unit.
	// It accepts a context and a request.
	Execute(ctx context.Context, req T) error
}

// StepConfig provides configuration for a Step of execution.
type StepConfig[T any] struct {
	Step                    Step[T]
	ContinueWorkflowOnError bool // decides if the workflow stops on Step errors
	// define this only if the Step implements RetryDecider, otherwise it has no effect and no sense!
	RetryConfigProvider func() (maxAttempts uint, attemptDelay time.Duration) // provides the retry configuration
}

// RetryDecider signals if an operation is retryable.
type RetryDecider interface {
	CanRetry() bool
}

// Logger is the workflow supported logger.
type Logger interface {
	Info(msg string)
	Error(msg string)
}

// noOpLogger is the internal, default logger, and is a no op.
// It exists only to allow the user to disable logging, by providing a nil logger to the Sequential constructor.
type noOpLogger struct{}

// Info is the Info level log.
func (n noOpLogger) Info(_ string) {}

// Error is the Error level log.
func (n noOpLogger) Error(_ string) {}
