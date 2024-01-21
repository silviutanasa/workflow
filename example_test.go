package workflow

import (
	"context"
	"fmt"
	"time"
)

// ETL (Extract Transform Load) workflow
func ETL() error {
	steps := []Step{
		&extractData{
			stepAbstract{
				stopWorkflow:            false,
				canRetry:                false,
				continueWorkflowOnError: false,
			},
		},
		&transformData{
			stepAbstract{
				stopWorkflow:            false,
				canRetry:                false,
				continueWorkflowOnError: false,
			},
		},
		&loadData{
			stepAbstract{
				stopWorkflow:            false,
				canRetry:                false,
				continueWorkflowOnError: false,
			},
		},
	}
	wf := NewSequential("ETL", steps, nil, RetryConfig{2, 0})

	return wf.Execute(context.TODO(), nil)
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

type stepAbstract struct {
	name                    string
	stopWorkflow            bool
	canRetry                bool
	continueWorkflowOnError bool
}

func (s *stepAbstract) Name() string {
	return s.name
}

func (s *stepAbstract) Execute(ctx context.Context, request any) error {
	//TODO implement me
	panic("implement me")
}

func (s *stepAbstract) StopWorkflow() bool {
	return s.stopWorkflow
}

func (s *stepAbstract) CanRetry() bool {
	return s.canRetry
}

func (s *stepAbstract) RetryConfig() (attempts uint, delay time.Duration) {
	return 1, time.Microsecond
}

func (s *stepAbstract) ContinueWorkflowOnError() bool {
	return s.continueWorkflowOnError
}
