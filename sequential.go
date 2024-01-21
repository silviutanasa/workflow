package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// minStrBuf is used to initialize the Sequential.internalBuf size, to avoid allocations caused by a too small buffer, when logging.
var minStrBuf = make([]byte, 0, 128)

func (c *Sequential) stringBuilder(in ...string) string {
	c.internalBuf.Reset()
	for i := range in {
		c.internalBuf.WriteString(in[i])
	}

	return unsafe.String(unsafe.SliceData(c.internalBuf.Bytes()), len(c.internalBuf.Bytes()))
}

// Sequential is the representation of a Sequential workflow.
// The Sequential workflow runs the underlying steps in the order provided by the user([]Step order).
// TODO: Add workflowHooksErr to this struct and expose a getter to it, so that the workflow errs are not mixed with the workflow hooks errs.
type Sequential struct {
	name               string
	steps              []Step
	afterWorkflowHooks []Step

	log         Logger
	retryConfig RetryConfig

	internalBuf bytes.Buffer
}

// RetryConfig is the config for the step retry
type RetryConfig struct {
	MaxRetryAttempts uint
	WaitBeforeRetry  time.Duration
}

// NewSequential is the workflow constructor.
func NewSequential(name string, steps []Step, log Logger, retryConfig RetryConfig) *Sequential {
	if log == nil {
		log = noOpLogger{}
	}

	s := Sequential{
		name:        name,
		steps:       steps,
		log:         log,
		retryConfig: retryConfig,
		// this construction saves an allocation, by taking the value of the Buffer, instead of using the pointer.
		// there is no cost in using this "hack", as the Sequential is used by reference, so the internalBuf can hold
		// a value without worrying about extra memory copy.
		internalBuf: *bytes.NewBuffer(minStrBuf),
	}

	return &s
}

// Execute starts processing all the steps from the workflow inner chain of steps.
func (c *Sequential) Execute(ctx context.Context, req interface{}) error {
	c.log.Info(c.stringBuilder("[START] executing workflow: ", c.name))
	defer c.log.Info(c.stringBuilder("[DONE] executing workflow: ", c.name))

	errs := c.executeWorkflow(ctx, req)

	c.log.Info("[start] executing after workflow hooks")
	errs = c.executeAfterWorkflowHooks(ctx, req, errs)
	c.log.Info("[done] executing after workflow hooks")

	if errs != nil {
		return newWorkflowErr(errs)
	}

	return nil
}

// AddAfterWorkflowHook allow adding a step that runs after the workflow finishes.
// It is guaranteed to run even if the workflow fails.
// TODO: once the Sequential instance is created, it should not change, so try to move this at construct time if possible!!!
func (c *Sequential) AddAfterWorkflowHook(hook Step) {
	c.afterWorkflowHooks = append(c.afterWorkflowHooks, hook)
}

func (c *Sequential) executeAfterWorkflowHooks(ctx context.Context, req interface{}, errs []error) []error {
	if c.afterWorkflowHooks != nil {
		for i := range c.afterWorkflowHooks {
			hook := c.afterWorkflowHooks[i]
			hookName := hook.Name()
			c.log.Info(c.stringBuilder("[start] executing after workflow hook, step name: ", hookName))

			err := hook.Execute(ctx, req)
			if err != nil {
				errs = append(errs, err)
				c.log.Error(c.stringBuilder("error executing after workflow hook, step name: ", hookName, ", err ", err.Error()))
			}
			c.log.Info(c.stringBuilder("[done] executing after workflow hook, step name: ", hookName))
		}
	}

	return errs
}

func (c *Sequential) executeWorkflow(ctx context.Context, req interface{}) []error {
	var errs []error
	for _, step := range c.steps {
		stepName := step.Name()

		c.log.Info(c.stringBuilder("[start] executing step name: ", stepName))
		err := c.executeStep(ctx, step, req)
		if err != nil {
			errs = append(errs, err)
			if step.ContinueWorkflowOnError() {
				c.log.Info(c.stringBuilder(
					"the step name: ",
					stepName,
					", is configured not to stop the workflow on error, so the following steps(if any) will still run",
				))

				continue
			}

			break
		}

		if step.StopWorkflow() {
			c.log.Info(c.stringBuilder("workflow execution stopped by step name: ", stepName))

			break
		}
	}

	return errs
}

func (c *Sequential) executeStep(ctx context.Context, step Step, req interface{}) error {
	stepName := step.Name()
	maxAttempts, attemptDelay := c.retryConfig.MaxRetryAttempts, c.retryConfig.WaitBeforeRetry

	var err error
	var attempt uint
	for attempt = 0; attempt <= maxAttempts; attempt++ {
		// if the attempt is more than 0, then it's a retry
		if attempt > 0 {
			c.log.Info(c.stringBuilder("->[", stepName, "] is configured to retry", ", retry attempt count: ", strconv.Itoa(int(attempt))))
			// allow some waiting time before trying again
			c.log.Info(c.stringBuilder(
				"->[", stepName, "] waiting for: ",
				", retry attempt count: ",
				attemptDelay.String(),
				" before retry attempt"),
			)
			time.Sleep(attemptDelay)
		}
		err = step.Execute(ctx, req)
		if err == nil {
			c.log.Info(c.stringBuilder("->[", stepName, "] succeeded"))

			break
		}
		c.log.Error(c.stringBuilder("->[", stepName, "] errored, err: ", err.Error()))

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

type workflowErr struct {
	errs []error
}

func newWorkflowErr(errs []error) error {
	return workflowErr{
		errs: errs,
	}
}

func (w workflowErr) Error() string {
	var errStrings []string
	for _, e := range w.errs {
		errStrings = append(errStrings, e.Error())
	}

	return fmt.Sprintf("workflow errors: %v", strings.Join(errStrings, ", "))
}

// As method allows searching/matching an error against the error collection
func (w workflowErr) As(i interface{}) bool {
	for _, e := range w.errs {
		if errors.As(e, i) {
			return true
		}
	}

	return false
}

type noOpLogger struct{}

func (n noOpLogger) Info(m string)  {}
func (n noOpLogger) Error(m string) {}
