package workflow

import "context"

type StepStatus string

const (
	FAILED  StepStatus = "FAILED"
	SUCCESS StepStatus = "SUCCESS"
)

// Storage describes the functionality for storing step execution result.
type Storage interface {
	Save(ctx context.Context, stepName, correlationID string, status StepStatus, output *string) error
	Get(ctx context.Context, stepName, correlationID string) (StepStatus, error)
	Clear(ctx context.Context, correlationID string) error
}
