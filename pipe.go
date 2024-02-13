package workflow

import (
	"context"
	"strconv"
	"time"
)

// PipeStep describes a step of execution.
type PipeStep[T any] interface {
	// Name provides the identity of the step.
	Name() string
	// Execute is the step central processing unit.
	// It accepts a context and a request.
	Execute(ctx context.Context, req T) (T, error)
}

// PipeStepConfig provides configuration for a PipeStep of execution.
type PipeStepConfig[T any] struct {
	Step PipeStep[T]
	// define this only if the Step implements RetryDecider, otherwise it has no effect and no sense!
	RetryConfigProvider func() (maxAttempts uint, attemptDelay time.Duration) // provides the retry configuration
}

// Pipe is a workflow that runs its steps in a predefined sequence(the order of the []PipeStepConfig).
type Pipe[T any] struct {
	name        string
	stepsConfig []PipeStepConfig[T] // the workflow runs the steps following the slice order
	log         Logger              // the internal logger is a no op if nil is provided
}

// NewPipe is the workflow constructor.
func NewPipe[T any](name string, stepsCfg []PipeStepConfig[T], log Logger) *Pipe[T] {
	if log == nil {
		log = noOpLogger{}
	}

	s := Pipe[T]{
		name:        name,
		stepsConfig: stepsCfg,
		log:         log,
	}

	return &s
}

// Name returns the name of the workflow.
func (p *Pipe[T]) Name() string {
	return p.name
}

// Execute loops through all the steps from the s.stepsConfig collection, passes the ctx and the req to the first PipeStepConfig.Step,
// and the following steps receive as request, the output from the previous step - pipe like behaviour.
// The workflow stops at the first failing step and returns the error produced by the step.
func (p *Pipe[T]) Execute(ctx context.Context, req T) (T, error) {
	p.log.Info(concatStr("[START] executing workflow: ", p.name))
	defer func() { p.log.Info(concatStr("[DONE] executing workflow: ", p.name)) }()

	var out T
	var err error
	for i, stepConfig := range p.stepsConfig {
		out, err = p.executeStep(ctx, stepConfig, req)
		if i > 0 {
			req = out
		}
		if err != nil {
			return out, err
		}
	}

	return out, nil
}

// executeStep processes a single PipeStep by passing it the ctx and the req.
// It retries the PipeStep if it implements the RetryDecider interface, and uses the max attempts and the attempt delay provided
// by the PipeStepConfig.RetryConfigProvider() if it's not nil. If the PipeStepConfig.RetryConfigProvider() is nil, there is no retry.
func (p *Pipe[T]) executeStep(ctx context.Context, stepCfg PipeStepConfig[T], req T) (T, error) {
	var out T
	step := stepCfg.Step
	stepName := step.Name()

	var maxAttempts uint
	var attemptDelay time.Duration
	if stepCfg.RetryConfigProvider != nil {
		maxAttempts, attemptDelay = stepCfg.RetryConfigProvider()
	}

	var attempt uint
	var err error
	for attempt = 0; attempt <= maxAttempts; attempt++ {
		// if the attempt is greater than 0, then it's a retry
		if attempt > 0 {
			p.log.Info(
				concatStr("step: ", stepName, " is configured to retry", ", retry attempt count: ", strconv.Itoa(int(attempt))),
			)
			// allow some waiting time before trying again
			p.log.Info(concatStr("waiting for: ", strconv.FormatInt(attemptDelay.Milliseconds(), 10), "ms before retry attempt"))
			time.Sleep(attemptDelay)
		}
		out, err = step.Execute(ctx, req)
		if err == nil {
			p.log.Info(concatStr(succeed, " executing step: ", stepName))

			break
		}
		p.log.Error(concatStr(failed, " executing step: ", stepName, ", err: ", err.Error()))
		// only the ones implementing the RetryDecider, with CanRetry() returning true, can run more than once
		if stepR, ok := step.(RetryDecider); !ok || (ok && !stepR.CanRetry()) {
			break
		}
	}

	return out, err
}
