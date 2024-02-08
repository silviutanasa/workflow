// Copyright 2024 Silviu Tanasă. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

/*
Package workflow provides orchestration for a set of steps of execution defined by the user.
It aims to help splitting an application in small pieces(steps) and orchestrate them with a simple API.

Simplest usage, in which every step's failure stops the workflow:

	// in real life usage, these must be concrete types implementing the workflow.Step interface.
	var step1, step2, step3 Step
	...
	// this is the simplest configuration, in which every step's failure stops the workflow.
	sc := []StepConfig{
		StepConfig{
			Step: step1,
		},
		StepConfig{
			Step: step2,
		},
		StepConfig{
			Step: step3,
		},
	}
	// the req is optional and can be nil if the scenario doesn't need it
	var req any
	wf := workflow.NewSequential("example", sc, nil)
	err := wf.Execute(context.Background(), &req)
	...

For granular control over the workflow behaviour, use ContinueWorkflowOnError and RetryConfigProvider options:

	sc := []StepConfig{
		StepConfig{
			// step1 must implement the RetryDecider interface, and return true for CanRetry() in order to be retryable
			Step: step1,
			ContinueWorkflowOnError: true,
			// this has effect only if step1 is retryable
			RetryConfigProvider: func() (uint, time.Duration) {return 2, time.Millisecond}
		},
		StepConfig{
			Step: step2,
			ContinueWorkflowOnError: true,
		},
		StepConfig{
			Step: step3,
		},
	}

For logging, just provide a logger that implements the workflow.Logger interface

	// in real life usage, this must be a concrete type implementing the workflow.Logger interface.
	val myLogger workflow.Logger
	...
	wf := workflow.NewSequential("example", sc, myLogger)
	...
*/
package workflow
