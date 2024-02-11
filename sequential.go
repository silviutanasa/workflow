package workflow

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"sync"
	"time"
	"unsafe"
)

const (
	succeed = "\u2713"
	failed  = "\u2717"
)

// bufPool is used by the internal logging system, to compute the string messages.
// the reason fo this choice is to reduce allocations.
var bufPool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return new(bytes.Buffer)
	},
}

// Sequential is a workflow that runs its steps in a predefined sequence(the order of the []StepConfig).
type Sequential[T any] struct {
	name        string
	stepsConfig []StepConfig[T] // the workflow runs the steps following the slice order
	log         Logger          // the internal logger is a no op if nil is provided
}

// NewSequential is the workflow constructor.
func NewSequential[T any](name string, stepsCfg []StepConfig[T], log Logger) *Sequential[T] {
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

// Execute loops through all the steps from the s.stepsConfig collection and passes the ctx and the req to every StepConfig.Step.
// The errors returned by the failing steps are wrapped in a single error, so any error from any failing StepConfig.Step
// can be checked using errors.Is or errors.As against the returned error.
// In case a StepConfig.Step fails, the workflow checks for the StepConfig.ContinueWorkflowOnError flag, and stops processing
// the remaining steps if the value is true.
func (s *Sequential[T]) Execute(ctx context.Context, req T) error {
	s.log.Info(s.concatStr("[START] executing workflow: ", s.name))
	defer func() { s.log.Info(s.concatStr("[DONE] executing workflow: ", s.name)) }()

	var errs []error
	for _, stepConfig := range s.stepsConfig {
		err := s.executeStep(ctx, stepConfig, req)
		if err != nil {
			errs = append(errs, err)
			if stepConfig.ContinueWorkflowOnError {
				s.log.Info(
					s.concatStr(
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
	if errs != nil {
		return errors.Join(errs...)
	}

	return nil
}

// concatStr produces a 0 allocation string concatenation, by taking the best parts from both bytes.Buffer and strings.Builder.
// The resulting string must be consumed ASAP, otherwise the content is not guaranteed to stay the same.
func (s *Sequential[T]) concatStr(in ...string) string {
	b := bufPool.Get().(*bytes.Buffer)
	b.Reset()
	defer bufPool.Put(b)

	for _, v := range in {
		b.WriteString(v)
	}
	// keep in mind that, as the package name suggests, this approach is not safe, and the string should be
	// "consumed" ASAP after this return, otherwise the content is not guaranteed.
	// it works well in the non-concurrent pre logging string composition, as we send the content to the writer, right
	// after this return.
	return unsafe.String(unsafe.SliceData(b.Bytes()), len(b.Bytes()))
}

// executeStep processes a single Step by passing it the ctx and the req.
// It retries the Step if it implements the RetryDecider interface, and uses the max attempts and the attempt delay provided
// by the StepConfig.RetryConfigProvider() if it's not nil. If the StepConfig.RetryConfigProvider() is nil, there is no retry.
func (s *Sequential[T]) executeStep(ctx context.Context, stepCfg StepConfig[T], req T) error {
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
			s.log.Info(
				s.concatStr("step: ", stepName, " is configured to retry", ", retry attempt count: ", strconv.Itoa(int(attempt))),
			)
			// allow some waiting time before trying again
			s.log.Info(s.concatStr("waiting for: ", strconv.FormatInt(attemptDelay.Milliseconds(), 10), "ms before retry attempt"))
			time.Sleep(attemptDelay)
		}
		err = step.Execute(ctx, req)
		if err == nil {
			s.log.Info(s.concatStr(succeed, " executing step: ", stepName))

			break
		}
		s.log.Error(s.concatStr(failed, " executing step: ", stepName, ", err: ", err.Error()))
		// only the ones implementing the RetryDecider, with CanRetry() returning true, can run more than once
		if stepR, ok := step.(RetryDecider); !ok || (ok && !stepR.CanRetry()) {
			break
		}
	}

	return err
}
