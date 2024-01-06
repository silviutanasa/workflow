package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sequential is the representation of a Sequential workflow.
// The Sequential workflow runs the underlying commands in the order provided by the user([]Command order).
// It holds a chain of commands, a logger, a canRetry flag to decide if the workflow can safely retry execution
// and an optional executionStoppedMsg message which is displayed in case of one of the commands from the workflow
// invokes the request propagation stop.
// TODO: Add workflowHooksErr to this struct and expose a getter to it, so that the workflow errs are not mixed with the workflow hooks errs.
type Sequential struct {
	name               string
	cmds               []Command
	afterWorkflowHooks []Command
	log                Logger
	waitBeforeRetry    time.Duration
	maxRetryCount      uint
}

// NewSequential is the workflow constructor.
func NewSequential(name string, cmds []Command, options ...func(*Sequential)) *Sequential {
	s := Sequential{
		name: name,
		cmds: cmds,
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
		s.maxRetryCount = maxRetryCount
		s.waitBeforeRetry = waitBeforeRetry
	}
}

// Execute starts processing all the commands from the workflow inner chain of commands.
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

// AddAfterWorkflowHook allow adding a command that runs after the workflow finishes.
// It is guaranteed to run even if the workflow fails.
func (c *Sequential) AddAfterWorkflowHook(hook Command) {
	c.afterWorkflowHooks = append(c.afterWorkflowHooks, hook)
}

func (c *Sequential) executeAfterWorkflowHooks(ctx context.Context, req interface{}, errs []error) []error {
	if c.afterWorkflowHooks != nil {
		for i := range c.afterWorkflowHooks {
			hook := c.afterWorkflowHooks[i]

			hookName := hook.Name()
			c.log.Info(fmt.Sprintf("[start] executing after workflow hook, cmd name: %v", hookName))
			err := hook.Execute(ctx, req)
			if err != nil {
				errs = append(errs, err)

				c.log.Error(fmt.Sprintf("error executing after workflow hook, cmd name: %v, returnedErr: %v", hookName, err))
			}
			c.log.Info(fmt.Sprintf("[done] executing after workflow hook, cmd name: %v", hookName))
		}
	}

	return errs
}

func (c *Sequential) executeWorkflow(ctx context.Context, req interface{}) []error {
	var errs []error
	for i, cmd := range c.cmds {
		stepNr := i + 1
		cmdName := cmd.Name()

		c.log.Info(fmt.Sprintf("[start] executing step nr. %v from the workflow, cmd name: %v", stepNr, cmdName))
		err := c.executeCmd(ctx, cmd, req)
		c.log.Info(fmt.Sprintf("[done] executing step nr. %v from the workflow, cmd name: %v", stepNr, cmdName))
		if err != nil {
			errs = append(errs, err)
			if cmd.ContinueWorkflowOnError() {
				c.log.Warn(fmt.Sprintf("the cmd nr. %v from the workflow, cmd name: %v, is configured not to not "+
					"stop the workflow on error, so the following cmds(if any) will still run", stepNr, cmdName))

				continue
			}

			break
		}

		if cmd.StopWorkflow() {
			c.log.Warn(fmt.Sprintf("workflow execution stopped by cmd nr. %v from the workflow, cmd name: %v",
				stepNr,
				cmdName,
			))

			break
		}
	}

	return errs
}

func (c *Sequential) executeCmd(ctx context.Context, cmd Command, req interface{}) error {
	cmdName := cmd.Name()

	var err error
	var attempt uint
	for attempt = 0; attempt <= c.maxRetryCount; attempt++ {
		// if the attempt is more than 0, then it's a retry
		if attempt > 0 {
			c.log.Info(fmt.Sprintf("--[%v] is configured to retry", cmdName))
			c.log.Info(fmt.Sprintf("--[%v] retry attempt count: %v", cmdName, attempt))
			// allow some waiting time before trying again
			s := c.waitBeforeRetry
			c.log.Info(fmt.Sprintf("--[%v] waiting for: %v before retry attempt", cmdName, s))
			time.Sleep(s)
		}
		err = cmd.Execute(ctx, req)
		if err == nil {
			c.log.Info(fmt.Sprintf("--[%v] succeeded", cmdName))

			break
		}
		c.log.Error(fmt.Sprintf("--[%v] errored, returnedErr: %v", cmdName, err))

		// what can happen is at some point in the retry cycle, the error becomes NOT retryable
		// example: an HTTP request first return HTTP 500 status code, which should be retryable,
		// but on the second attempt it returns 400 which means bad data and should not be retried,
		// so the retry cycle stop here in this case, even if there are more attempts left.
		if !cmd.CanRetryOnError() {
			break
		}
	}

	return err
}

// ListSteps returns a []string with the names(obtained by the Command.Name() call) of all the component commands.
// it can be useful for logging/testing purposes.
func (c *Sequential) ListSteps() []string {
	l := len(c.cmds)
	if l == 0 {
		return nil
	}

	cn := make([]string, l)
	for i := range cn {
		cn[i] = c.cmds[i].Name()
	}

	return cn
}

// ListHooks returns a []string with the names(obtained by the Command.Name() call) of all the component after workflow hooks.
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
