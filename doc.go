// Copyright 2024 Silviu Tanasă. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

/*
Package workflow provides orchestration for a set of steps of execution defined by the user.
It aims to help splitting an application in small pieces(steps) and orchestrate them with a simple API.

1. Sequential workflow:
Simplest usage, in which every step's failure stops the workflow:

	// in real life usage, these must be concrete types implementing the workflow.SequentialStep interface.
	var step1, step2, step3 SequentialStep
	...
	// this is the simplest configuration, in which every step's failure stops the workflow.
	sc := []SequentialStepConfig[any]{
		{Step: step1},
		{Step: step2},
		{Step: step3},
	}
	// the req is optional and can be nil if the scenario doesn't need it
	// in case you need the request param, then its type must be the T from SequentialStepConfig[T]
	var req any
	wf := workflow.NewSequential("example", sc, nil)
	err := wf.Execute(context.Background(), &req)
	...

For granular control over the workflow behaviour, use ContinueWorkflowOnError and RetryConfigProvider options:

	sc := []SequentialStepConfig[any]{
		{
			// step1 must implement the RetryDecider interface, and return true for CanRetry() in order to be retryable
			SequentialStep: step1,
			ContinueWorkflowOnError: true,
			// this has effect only if step1 is retryable
			RetryConfigProvider: func() (uint, time.Duration) {return 2, time.Millisecond}
		},
		{
			SequentialStep: step2,
			ContinueWorkflowOnError: true,
		},
		{
			SequentialStep: step3,
		},
	}

For logging, just provide a logger that implements the workflow.Logger interface

	// in real life usage, this must be a concrete type implementing the workflow.Logger interface.
	var myLogger workflow.Logger
	...
	wf := workflow.NewSequential("example", sc, myLogger)
	...

2. Pipe workflow:

	// in real life usage, these must be concrete types implementing the workflow.PipeStep interface.
	var step1, step2, step3 PipeStep
	...
	// this is the simplest configuration, in which every step's failure stops the workflow.
	sc := []PipeStepConfig[string]{
		{Step: step1},
		{Step: step2},
		{Step: step3},
	}
	req := "some string to be processed"
	wf := workflow.NewPipe("example", sc, nil)
	result, err := wf.Execute(context.Background(), req)
	...

For logging, just provide a logger that implements the workflow.Logger interface

	// in real life usage, this must be a concrete type implementing the workflow.Logger interface.
	var myLogger workflow.Logger
	...
	wf := workflow.NewPipe("example", sc, myLogger)
	...
*/
package workflow
