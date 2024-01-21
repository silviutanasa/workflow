package workflow

import (
	"context"
)

// Step represents a step of execution(data processor).
type Step interface {
	// Name returns the name of the step.
	Name() string
	// Execute is the step central processing unit.
	// It accepts a context and a request.
	Execute(ctx context.Context, request any) error
	// StopWorkflow decides if the step can completely stop the workflow and also the propagation to other steps that should run in a chain
	// after it completes its work.
	StopWorkflow() bool
	// CanRetry decides if the step can retry its execution.
	CanRetry() bool

	// ContinueWorkflowOnError decides if the step can stop the propagation of the request to other steps that ran in a chain
	// in case the step returns an error.
	ContinueWorkflowOnError() bool
}

// Logger is the workflow supported logger.
type Logger interface {
	Info(msg string)
	Error(msg string)
}
