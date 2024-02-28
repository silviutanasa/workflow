package workflow

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// ErrMissingCorrelationID is thrown when the storage option is provided but there is no step correlation id.
var ErrMissingCorrelationID = errors.New("missing correlation ID")

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
	// this ID is only used for storage and usually is the eventID from request object.
	// in this way when the event is replayed the `workflow` will be able to skip previous successful steps.
	correlationID *string

	name        string
	stepsConfig []SequentialStepConfig[T] // the workflow runs the steps following the slice order
	log         Logger                    // the internal logger is a no op if nil is provided
	store       Storage
}

// SequentialOption is an alias function used to apply various configurations.
type SequentialOption func(s *Sequential[any])

// NewSequential is the workflow constructor.
func NewSequential[T any](name string, stepsCfg []SequentialStepConfig[T], opts ...SequentialOption) *Sequential[T] {
	s := &Sequential[T]{
		name:        name,
		stepsConfig: stepsCfg,
	}

	for _, opt := range opts {
		opt((interface{})(s).(*Sequential[any]))
	}

	return s
}

func WithLogger(log Logger) SequentialOption {
	return func(s *Sequential[any]) {
		s.log = log
	}
}

func WithStorage(st Storage) SequentialOption {
	return func(s *Sequential[any]) {
		s.store = st
	}
}

func WithCorrelationID(id string) SequentialOption {
	return func(s *Sequential[any]) {
		s.correlationID = &id
	}
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
	s.print(concatStr("[START] executing workflow: ", s.name), infoLevel)
	defer func() { s.print(concatStr("[DONE] executing workflow: ", s.name), infoLevel) }()

	var (
		errs []error
		err  error
	)

	for _, stepConfig := range s.stepsConfig {
		if s.skipStep(ctx, stepConfig.Step.Name()) {
			s.print(concatStr("step: ", stepConfig.Step.Name(), " is already processed, skipping"), infoLevel)
			continue
		}

		err = s.executeStep(ctx, stepConfig, req)
		if err != nil {
			// this prevents extra allocations, by creating the slice only once, and with enough capacity.
			if errs == nil {
				errs = make([]error, 0, len(s.stepsConfig))
			}
			errs = append(errs, err)

			if stepConfig.ContinueWorkflowOnError {
				s.print(concatStr(
					"the step name: ",
					stepConfig.Step.Name(),
					", is configured not to stop the workflow on error, so the following stepsConfig(if any) will still run",
				), infoLevel)

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

	// if everything was ok, then cleanup storage
	return s.clearDB(ctx, s.correlationID)
}

// executeStep processes a single SequentialStep by passing it the ctx and the req.
// It retries the SequentialStep if it implements the RetryDecider interface, and uses the max attempts and the attempt delay provided
// by the SequentialStepConfig.RetryConfigProvider() if it's not nil. If the SequentialStepConfig.RetryConfigProvider() is nil, there is no retry.
func (s *Sequential[T]) executeStep(ctx context.Context, stepCfg SequentialStepConfig[T], req T) error {
	var (
		maxAttempts  uint
		attemptDelay time.Duration
		attempt      int
		err          error
	)

	step := stepCfg.Step
	stepName := step.Name()

	if stepCfg.RetryConfigProvider != nil {
		maxAttempts, attemptDelay = stepCfg.RetryConfigProvider()
	}

	for attempt = 0; attempt <= int(maxAttempts); attempt++ {
		// if the attempt is greater than 0, then it's a retry
		if attempt > 0 {
			s.print(
				concatStr("step: ", stepName, " is configured to retry", ", retry attempt count: ", strconv.Itoa(attempt)),
				infoLevel,
			)
			// allow some waiting time before trying again
			s.print(concatStr("waiting for: ", strconv.FormatInt(attemptDelay.Milliseconds(), 10), "ms before retry attempt"), infoLevel)
			time.Sleep(attemptDelay)
		}

		err = step.Execute(ctx, req)
		if err == nil {
			s.print(concatStr(succeed, " executing step: ", stepName), infoLevel)

			break
		}

		s.print(concatStr(failed, " executing step: ", stepName, ", err: ", err.Error()), errorLevel)
		// only the ones implementing the RetryDecider, with CanRetry() returning true, can run more than once
		if stepR, ok := step.(RetryDecider); !ok || (ok && !stepR.CanRetry()) {
			break
		}
	}

	dbErr := s.storeStepResult(ctx, stepName, err)
	if dbErr != nil {
		err = fmt.Errorf("error storing step execution result: %q", err)
	}

	return err
}

func (s *Sequential[T]) print(msg string, level int) {
	if s.log == nil {
		return
	}

	switch level {
	case 1:
		s.log.Info(msg)
	case 2:
		s.log.Error(msg)
	}
}

func (s *Sequential[T]) skipStep(ctx context.Context, stepName string) bool {
	if s.store == nil {
		return false
	}

	if s.correlationID == nil {
		return false
	}

	sts, err := s.store.Get(ctx, stepName, *s.correlationID)
	if err != nil {
		s.print(err.Error(), errorLevel)
		return false
	}

	return sts == SUCCESS
}

func (s *Sequential[T]) storeStepResult(ctx context.Context, stepName string, err error) error {
	if s.store == nil {
		return nil
	}

	if s.correlationID == nil {
		return fmt.Errorf("cannot store step result in DB: %q", ErrMissingCorrelationID)
	}

	var output string
	sts := SUCCESS
	if err != nil {
		sts = FAILED
		output = err.Error()
	}

	return s.store.Save(ctx, stepName, *s.correlationID, sts, &output)
}

func (s *Sequential[T]) clearDB(ctx context.Context, id *string) error {
	if s.store == nil || id == nil {
		return nil
	}

	return s.store.Clear(ctx, *id)
}
