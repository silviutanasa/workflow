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

var bufPool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return new(bytes.Buffer)
	},
}

// Sequential is the representation of a Sequential workflow.
// The Sequential workflow runs the underlying stepsConfig in the order provided by the user([]Step order).
type Sequential struct {
	name        string
	stepsConfig []StepConfig
	log         Logger
}

// NewSequential is the workflow constructor.
func NewSequential(name string, stepsCfg []StepConfig, log Logger) *Sequential {
	if log == nil {
		log = noOpLogger{}
	}

	s := Sequential{
		name:        name,
		stepsConfig: stepsCfg,
		log:         log,
	}

	return &s
}

// Name starts processing all the stepsConfig from the workflow inner chain of stepsConfig.
func (s *Sequential) Name() string {
	return s.name
}

// Execute starts processing all the stepsConfig from the workflow inner chain of stepsConfig.
func (s *Sequential) Execute(ctx context.Context, req any) error {
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

func (s *Sequential) concatStr(in ...string) string {
	b := bufPool.Get().(*bytes.Buffer)
	b.Reset()
	defer bufPool.Put(b)

	for _, v := range in {
		b.WriteString(v)
	}

	return unsafe.String(unsafe.SliceData(b.Bytes()), len(b.Bytes()))
}

func (s *Sequential) executeStep(ctx context.Context, stepCfg StepConfig, req any) error {
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
		// if the attempt is more than 0, then it's a retry
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

		if stepR, ok := step.(Retryer); !ok || (ok && !stepR.CanRetry()) {
			break
		}
	}

	return err
}
