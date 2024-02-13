package workflow_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/silviutanasa/workflow"
)

func ExampleSequential_Execute() {
	// because it also implements the SequentialStep interface, the workflow can also be a step from another workflow.
	stepsCfgEmb := []workflow.SequentialStepConfig[any]{
		{Step: &sequentialStepAbstract{name: "get-raw-data-from-db"}},
		{Step: &sequentialStepAbstract{name: "transform-raw-data-into-models"}},
	}
	extractDataWorkflow := workflow.NewSequential("extract-data", stepsCfgEmb, nil)

	stepsCfg := []workflow.SequentialStepConfig[any]{
		{Step: extractDataWorkflow},
		{Step: &sequentialStepAbstract{name: "transform-data"}},
		{Step: &sequentialStepAbstract{name: "load-data"}},
	}

	wf := workflow.NewSequential("ETL", stepsCfg, nil)
	wf.Execute(context.TODO(), nil)
	// Output:
	//running: get-raw-data-from-db
	//running: transform-raw-data-into-models
	//running: transform-data
	//running: load-data
}

func ExamplePipe_Execute() {
	stepsCfg := []workflow.PipeStepConfig[string]{
		{Step: &trimSpaces{name: "trim-spaces"}},
		{Step: &removeCommas{name: "remove-commas"}},
		{Step: &removeDots{name: "remove-dots"}},
		{Step: &transformToUppercase{name: "transform-to-upper"}},
	}

	wf := workflow.NewPipe("ETL", stepsCfg, nil)
	output, _ := wf.Execute(context.TODO(), "    I. am. the, string. to be transformed,   ,     ")
	fmt.Print(output)
	// Output:
	//I AM THE STRING TO BE TRANSFORMED

}

type sequentialStepAbstract struct {
	name string
}

func (s *sequentialStepAbstract) Name() string {
	return s.name
}

func (s *sequentialStepAbstract) Execute(ctx context.Context, request any) error {
	fmt.Printf("\nrunning: %v", s.name)
	return nil
}

type trimSpaces struct {
	name string
}

func (s *trimSpaces) Name() string {
	return s.name
}

func (s *trimSpaces) Execute(ctx context.Context, req string) (string, error) {
	return strings.TrimSpace(req), nil
}

type transformToUppercase struct {
	name string
}

func (s *transformToUppercase) Name() string {
	return s.name
}

func (s *transformToUppercase) Execute(ctx context.Context, req string) (string, error) {
	return strings.ToUpper(req), nil
}

type removeCommas struct {
	name string
}

func (s *removeCommas) Name() string {
	return s.name
}

func (s *removeCommas) Execute(ctx context.Context, req string) (string, error) {
	return strings.ReplaceAll(req, ",", ""), nil
}

type removeDots struct {
	name string
}

func (s *removeDots) Name() string {
	return s.name
}

func (s *removeDots) Execute(ctx context.Context, req string) (string, error) {
	return strings.ReplaceAll(req, ".", ""), nil
}
