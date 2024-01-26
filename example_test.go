package workflow

import (
	"context"
	"fmt"
)

func ExampleSequential_Execute() {
	stepsCfg := []StepConfig{
		{
			Step:                    &extractData{},
			ContinueWorkflowOnError: false,
		},
		{
			Step:                    &transformData{},
			ContinueWorkflowOnError: false,
		},
		{
			Step:                    &loadData{},
			ContinueWorkflowOnError: false,
		},
	}
	postHooks := []Step{
		&logRequestData{},
		&sendCompletionNotification{},
	}
	wf := NewSequential("ETL", stepsCfg, postHooks, RetryConfig{2, 0}, nil)
	_ = wf.Execute(context.TODO(), nil)
	// Output:
	// I extracted the data
	// I transformed the data
	// I loaded the data
	// I logged the request data
	// I sent the completion notification
}

// Extract
type extractData struct {
	stepAbstract
}

func (e *extractData) Execute(ctx context.Context, request any) error {
	fmt.Println("I extracted the data")
	return nil
}

// Transform
type transformData struct {
	stepAbstract
}

func (e *transformData) Execute(ctx context.Context, request any) error {
	fmt.Println("I transformed the data")
	return nil
}

// Load
type loadData struct {
	stepAbstract
}

func (e *loadData) Execute(ctx context.Context, request any) error {
	fmt.Println("I loaded the data")
	return nil
}

// Log request data
type logRequestData struct {
	stepAbstract
}

func (l *logRequestData) Execute(ctx context.Context, request any) error {
	fmt.Println("I logged the request data")
	return nil
}

type sendCompletionNotification struct {
	stepAbstract
}

func (s *sendCompletionNotification) Execute(ctx context.Context, request any) error {
	fmt.Println("I sent the completion notification")
	return nil
}

type stepAbstract struct {
	name     string
	canRetry bool
}

func (s *stepAbstract) Name() string {
	return s.name
}

func (s *stepAbstract) Execute(ctx context.Context, request any) error {
	panic("implement me")
}

func (s *stepAbstract) CanRetry() bool {
	return s.canRetry
}
