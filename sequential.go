package workflow

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"time"
	"unsafe"
)

const (
	succeed = "\u2713"
	failed  = "\u2717"
)

// minStrBuf is used to initialize the Sequential.internalBuf size, to avoid allocations caused by a too small buffer, when logging.
var minStrBuf = make([]byte, 0, 128)

// Sequential is the representation of a Sequential workflow.
// The Sequential workflow runs the underlying stepsConfig in the order provided by the user([]Step order).
type Sequential struct {
	name              string
	stepsConfig       []StepConfig
	postWorkflowHooks []Step

	log         Logger
	internalBuf bytes.Buffer

	retryConfig RetryConfig
}

// NewSequential is the workflow constructor.
func NewSequential(name string, stepsCfg []StepConfig, postHooks []Step, retryCfg RetryConfig, log Logger) *Sequential {
	if log == nil {
		log = noOpLogger{}
	}

	s := Sequential{
		name:              name,
		stepsConfig:       stepsCfg,
		postWorkflowHooks: postHooks,

		log: log,
		// this construction saves an allocation, by taking the value of the Buffer, instead of using the pointer.
		// there is no cost in using this "hack", as the Sequential is used by reference, so the internalBuf can hold
		// a value without worrying about extra memory copy.
		internalBuf: *bytes.NewBuffer(minStrBuf),

		retryConfig: retryCfg,
	}

	return &s
}

// Execute starts processing all the stepsConfig from the workflow inner chain of stepsConfig.
func (s *Sequential) Execute(ctx context.Context, req interface{}) error {
	s.log.Info(s.concatStr("[START] executing workflow: ", s.name))
	defer func() { s.log.Info(s.concatStr("[DONE] executing workflow: ", s.name)) }()

	errs := s.executeWorkflow(ctx, req)
	errs = s.executeAfterWorkflowHooks(ctx, req, errs)
	if errs != nil {
		return errors.Join(errs...)
	}

	return nil
}

func (s *Sequential) concatStr(in ...string) string {
	s.internalBuf.Reset()
	for _, v := range in {
		s.internalBuf.WriteString(v)
	}

	return unsafe.String(unsafe.SliceData(s.internalBuf.Bytes()), len(s.internalBuf.Bytes()))
}

func (s *Sequential) executeWorkflow(ctx context.Context, req interface{}) []error {
	var errs []error
	for _, stepConfig := range s.stepsConfig {
		err := s.executeStep(ctx, stepConfig.Step, req)
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

	return errs
}

func (s *Sequential) executeStep(ctx context.Context, step Step, req interface{}) error {
	stepName := step.Name()
	maxAttempts, attemptDelay := int(s.retryConfig.MaxRetryAttempts), s.retryConfig.WaitBeforeRetry

	var err error
	var attempt int
	for attempt = 0; attempt <= maxAttempts; attempt++ {
		// if the attempt is more than 0, then it's a retry
		if attempt > 0 {
			s.log.Info(
				s.concatStr("step: ", stepName, " is configured to retry", ", retry attempt count: ", strconv.Itoa(attempt)),
			)
			// allow some waiting time before trying again
			s.log.Info(s.concatStr("waiting for: ", attemptDelay.String(), " before retry attempt"))
			time.Sleep(attemptDelay)
		}
		err = step.Execute(ctx, req)
		if err == nil {
			s.log.Info(s.concatStr(succeed, " executing step: ", stepName))

			break
		}
		s.log.Error(s.concatStr(failed, " executing step: ", stepName, ", err: ", err.Error()))

		// what can happen is at some point in the retry cycle, the error becomes NOT retryable
		// example: a HTTP request first return HTTP 500 status code, which should be retryable,
		// but on the second attempt it returns 400 which means bad data and should not be retried,
		// so the retry cycle stop here in this case, even if there are more attempts left.
		if !step.CanRetry() {
			break
		}
	}

	return err
}

func (s *Sequential) executeAfterWorkflowHooks(ctx context.Context, req interface{}, errs []error) []error {
	if s.postWorkflowHooks != nil {
		for _, hook := range s.postWorkflowHooks {
			hookName := hook.Name()
			err := hook.Execute(ctx, req)
			if err != nil {
				errs = append(errs, err)
				s.log.Error(s.concatStr(failed, " executing after workflow hook: ", hookName, ", err ", err.Error()))

				continue
			}
			s.log.Info(s.concatStr(succeed, " executing after workflow hook: ", hookName))
		}
	}

	return errs
}
