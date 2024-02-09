package workflow_test

import (
	"context"
	"fmt"

	"github.com/silviutanasa/workflow"
)

func ExampleSequential_Execute() {
	// because it also implements the Step interface, the workflow can also be a step from another workflow.
	stepsCfgEmb := []workflow.StepConfig{
		{Step: &stepAbstract{name: "get-raw-data-from-db"}},
		{Step: &stepAbstract{name: "transform-raw-data-into-models"}},
	}
	extractDataWorkflow := workflow.NewSequential("extract-data", stepsCfgEmb, nil)

	stepsCfg := []workflow.StepConfig{
		{Step: extractDataWorkflow},
		{Step: &stepAbstract{name: "transform-data"}},
		{Step: &stepAbstract{name: "load-data"}},
	}

	wf := workflow.NewSequential("ETL", stepsCfg, nil)
	wf.Execute(context.TODO(), nil)
	// Output:
	//running: get-raw-data-from-db
	//running: transform-raw-data-into-models
	//running: transform-data
	//running: load-data
}

type stepAbstract struct {
	name string
}

func (s *stepAbstract) Name() string {
	return s.name
}

func (s *stepAbstract) Execute(ctx context.Context, request any) error {
	fmt.Printf("\nrunning: %v", s.name)
	return nil
}
