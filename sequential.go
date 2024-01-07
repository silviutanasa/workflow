package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sequential is the representation of a Sequential workflow.
// The Sequential workflow runs the underlying steps in the order provided by the user([]Step order).
// TODO: Add workflowHooksErr to this struct and expose a getter to it, so that the workflow errs are not mixed with the workflow hooks errs.
type Sequential struct {
	name               string
	steps              []Step
	afterWorkflowHooks []Step
	log                Logger
	// retry parameters
	waitBeforeRetry  time.Duration
	maxRetryAttempts uint
}

// NewSequential is the workflow constructor.
func NewSequential(name string, steps []Step, options ...func(*Sequential)) *Sequential {
	s := Sequential{
		name:  name,
		steps: steps,
	}
	for _, option := range options {
		option(&s)
	}

	if s.log == nil {
		s.log = noOpLogger{}
	}

	return &s
}

// WithLoggerOption configures the workflow with the retry option.
func WithLoggerOption(l Logger) func(*Sequential) {
	return func(s *Sequential) {
		s.log = l
	}

}

// WithRetryOption configures the workflow with the retry option.
func WithRetryOption(maxRetryCount uint, waitBeforeRetry time.Duration) func(*Sequential) {
	return func(s *Sequential) {
		s.maxRetryAttempts = maxRetryCount
		s.waitBeforeRetry = waitBeforeRetry
	}
}

// Execute starts processing all the steps from the workflow inner chain of steps.
func (c *Sequential) Execute(ctx context.Context, req interface{}) error {
	c.log.Info(fmt.Sprintf("[START] executing workflow: %v", c.name))
	defer c.log.Info(fmt.Sprintf("[DONE] executing workflow: %v", c.name))

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
func (c *Sequential) AddAfterWorkflowHook(hook Step) {
	c.afterWorkflowHooks = append(c.afterWorkflowHooks, hook)
}

func (c *Sequential) executeAfterWorkflowHooks(ctx context.Context, req interface{}, errs []error) []error {
	if c.afterWorkflowHooks != nil {
		for i := range c.afterWorkflowHooks {
			hook := c.afterWorkflowHooks[i]

			hookName := hook.Name()
			c.log.Info(fmt.Sprintf("[start] executing after workflow hook, step name: %v", hookName))
			err := hook.Execute(ctx, req)
			if err != nil {
				errs = append(errs, err)

				c.log.Error(fmt.Sprintf("error executing after workflow hook, step name: %v, returnedErr: %v", hookName, err))
			}
			c.log.Info(fmt.Sprintf("[done] executing after workflow hook, step name: %v", hookName))
		}
	}

	return errs
}

func (c *Sequential) executeWorkflow(ctx context.Context, req interface{}) []error {
	var errs []error
	for i, step := range c.steps {
		stepNr := i + 1
		stepName := step.Name()

		c.log.Info(fmt.Sprintf("[start] executing step nr. %v from the workflow, step name: %v", stepNr, stepName))
		err := c.executeStep(ctx, step, req)
		c.log.Info(fmt.Sprintf("[done] executing step nr. %v from the workflow, step name: %v", stepNr, stepName))
		if err != nil {
			errs = append(errs, err)
			if step.ContinueWorkflowOnError() {
				c.log.Warn(fmt.Sprintf("the step nr. %v from the workflow, step name: %v, is configured not to not "+
					"stop the workflow on error, so the following steps(if any) will still run", stepNr, stepName))

				continue
			}

			break
		}

		if step.StopWorkflow() {
			c.log.Warn(fmt.Sprintf("workflow execution stopped by step nr. %v from the workflow, step name: %v",
				stepNr,
				stepName,
			))

			break
		}
	}

	return errs
}

func (c *Sequential) executeStep(ctx context.Context, step Step, req interface{}) error {
	stepName := step.Name()

	var err error
	var attempt uint
	for attempt = 0; attempt <= c.maxRetryAttempts; attempt++ {
		// if the attempt is more than 0, then it's a retry
		if attempt > 0 {
			c.log.Info(fmt.Sprintf("--[%v] is configured to retry", stepName))
			c.log.Info(fmt.Sprintf("--[%v] retry attempt count: %v", stepName, attempt))
			// allow some waiting time before trying again
			s := c.waitBeforeRetry
			c.log.Info(fmt.Sprintf("--[%v] waiting for: %v before retry attempt", stepName, s))
			time.Sleep(s)
		}
		err = step.Execute(ctx, req)
		if err == nil {
			c.log.Info(fmt.Sprintf("--[%v] succeeded", stepName))

			break
		}
		c.log.Error(fmt.Sprintf("--[%v] errored, returnedErr: %v", stepName, err))

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

// ListSteps returns a []string with the names(obtained by the Step.Name() call) of all the component steps.
// it can be useful for logging/testing purposes.
func (c *Sequential) ListSteps() []string {
	l := len(c.steps)
	if l == 0 {
		return nil
	}

	cn := make([]string, l)
	for i := range cn {
		cn[i] = c.steps[i].Name()
	}

	return cn
}

// ListHooks returns a []string with the names(obtained by the Step.Name() call) of all the component after workflow hooks.
// it can be useful for logging/testing purposes.
func (c *Sequential) ListHooks() []string {
	l := len(c.afterWorkflowHooks)
	if l == 0 {
		return nil
	}

	cn := make([]string, l)
	for i := range cn {
		cn[i] = c.afterWorkflowHooks[i].Name()
	}

	return cn
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

func (n noOpLogger) Info(_ any)  {}
func (n noOpLogger) Warn(_ any)  {}
func (n noOpLogger) Error(_ any) {}
