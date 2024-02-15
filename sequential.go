package workflow

import (
	"context"
	"errors"
	"strconv"
	"time"
)

// SequentialStep describes a step of execution.
type SequentialStep[T any] interface {
	// Name provides the identity of the step.
	Name() string
	// Execute is the step central processing unit.
	// It accepts a context and a request.
	Execute(ctx context.Context, req T) error
}

// SequentialStepConfig provides configuration for a SequentialStep of execution.
type SequentialStepConfig[T any] struct {
	Step                    SequentialStep[T]
	ContinueWorkflowOnError bool // decides if the workflow stops on Step errors
	// define this only if the Step implements RetryDecider, otherwise it has no effect and no sense!
	RetryConfigProvider func() (maxAttempts uint, attemptDelay time.Duration) // provides the retry configuration
}

// Sequential is a workflow that runs its steps in a predefined sequence(the order of the []SequentialStepConfig).
type Sequential[T any] struct {
	name        string
	stepsConfig []SequentialStepConfig[T] // the workflow runs the steps following the slice order
	log         Logger                    // the internal logger is a no op if nil is provided
}

// NewSequential is the workflow constructor.
func NewSequential[T any](name string, stepsCfg []SequentialStepConfig[T], log Logger) *Sequential[T] {
	if log == nil {
		log = noOpLogger{}
	}

	s := Sequential[T]{
		name:        name,
		stepsConfig: stepsCfg,
		log:         log,
	}

	return &s
}

// Name returns the name of the workflow.
func (s *Sequential[T]) Name() string {
	return s.name
}

// Execute loops through all the steps from the s.stepsConfig collection and passes the ctx and the req to every SequentialStepConfig.Step.
// The errors returned by the failing steps are wrapped in a single error, so any error from any failing SequentialStepConfig.Step
// can be checked using errors.Is or errors.As against the returned error.
// In case a SequentialStepConfig.Step fails, the workflow checks for the SequentialStepConfig.ContinueWorkflowOnError flag, and stops processing
// the remaining steps if the value is true.
func (s *Sequential[T]) Execute(ctx context.Context, req T) error {
	s.log.Info(concatStr("[START] executing workflow: ", s.name))
	defer func() { s.log.Info(concatStr("[DONE] executing workflow: ", s.name)) }()

	var errs []error
	var err error
	for _, stepConfig := range s.stepsConfig {
		err = s.executeStep(ctx, stepConfig, req)
		if err != nil {
			// this prevents extra allocations, by creating the slice only once, and with enough capacity.
			if errs == nil {
				errs = make([]error, 0, len(s.stepsConfig))
			}
			errs = append(errs, err)

			if stepConfig.ContinueWorkflowOnError {
				s.log.Info(
					concatStr(
						"the step name: ",
						stepConfig.Step.Name(),
						", is configured not to stop the workflow on error, so the following stepsConfig(if any) will still run",
					),
				)

				continue
			}

			break
		}
	}

	switch {
	// prevents unnecessary allocations caused by errors.Join, if the collection holds only 1 error.
	case len(errs) == 1:
		return errs[0]
	case len(errs) > 1:
		return errors.Join(errs...)
	}

	return nil
}

// executeStep processes a single SequentialStep by passing it the ctx and the req.
// It retries the SequentialStep if it implements the RetryDecider interface, and uses the max attempts and the attempt delay provided
// by the SequentialStepConfig.RetryConfigProvider() if it's not nil. If the SequentialStepConfig.RetryConfigProvider() is nil, there is no retry.
func (s *Sequential[T]) executeStep(ctx context.Context, stepCfg SequentialStepConfig[T], req T) error {
	step := stepCfg.Step
	stepName := step.Name()

	var maxAttempts uint
	var attemptDelay time.Duration
	if stepCfg.RetryConfigProvider != nil {
		maxAttempts, attemptDelay = stepCfg.RetryConfigProvider()
	}

	var attempt int
	var err error
	for attempt = 0; attempt <= int(maxAttempts); attempt++ {
		// if the attempt is greater than 0, then it's a retry
		if attempt > 0 {
			s.log.Info(
				concatStr("step: ", stepName, " is configured to retry", ", retry attempt count: ", strconv.Itoa(attempt)),
			)
			// allow some waiting time before trying again
			s.log.Info(concatStr("waiting for: ", strconv.FormatInt(attemptDelay.Milliseconds(), 10), "ms before retry attempt"))
			time.Sleep(attemptDelay)
		}
		err = step.Execute(ctx, req)
		if err == nil {
			s.log.Info(concatStr(succeed, " executing step: ", stepName))

			break
		}
		s.log.Error(concatStr(failed, " executing step: ", stepName, ", err: ", err.Error()))
		// only the ones implementing the RetryDecider, with CanRetry() returning true, can run more than once
		if stepR, ok := step.(RetryDecider); !ok || (ok && !stepR.CanRetry()) {
			break
		}
	}

	return err
}
