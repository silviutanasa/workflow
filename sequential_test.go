package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type testRetryableErr struct {
}

func (t testRetryableErr) Error() string {
	return "any err"
}

func Test_Execute_BehaviourOnPreservingErrorsType(t *testing.T) {
	steps := []Step{
		&stepMock{
			name:                  "cmd2",
			execute:               errors.New("any err"),
			retryOnErr:            false,
			continueWorkflowOnErr: true,
			stopWorkflow:          false,
		},
		&stepMock{
			name:                  "cmd3",
			execute:               testRetryableErr{},
			retryOnErr:            false,
			continueWorkflowOnErr: true,
			stopWorkflow:          false,
		},
	}
	c := NewSequential("order-extractor", steps, WithRetryOption(2, 0))
	actualResult := c.Execute(context.TODO(), struct{}{})

	var expectedErr testRetryableErr
	if !errors.As(actualResult, &expectedErr) {
		t.Errorf("the workflow error does not embed the inner errors(errors returned by component steps)")
	}
}

func Test_Execute_BehaviourOnReturningErrors(t *testing.T) {
	tests := []struct {
		name           string
		input          interface{}
		mock           []Step
		expectedResult error
	}{
		{
			name:           "an workflow with an empty chain should not return an error",
			input:          struct{}{},
			mock:           []Step{},
			expectedResult: nil,
		},
		{
			name:  "a workflow with steps returning errors, but configured not to stop on them, should return an error",
			input: struct{}{},
			mock: []Step{
				&stepMock{
					name:                  "cmd2",
					execute:               cmpopts.AnyError,
					retryOnErr:            false,
					continueWorkflowOnErr: true,
					stopWorkflow:          false,
				},
				&stepMock{
					name:                  "cmd3",
					execute:               nil,
					retryOnErr:            false,
					continueWorkflowOnErr: false,
					stopWorkflow:          false,
				},
			},
			expectedResult: cmpopts.AnyError,
		},
		{
			name:  "a workflow with steps returning error, but configured to retry, should not return an error if it succeeds on retry",
			input: struct{}{},
			mock: []Step{
				&stepMock{
					name:                     "cmd1",
					succeedAtInvocationCount: 2,
					execute:                  errors.New("some err"),
					retryOnErr:               true,
					continueWorkflowOnErr:    false,
					stopWorkflow:             false,
				},
				&stepMock{
					name:                     "cmd2",
					succeedAtInvocationCount: 3,
					execute:                  errors.New("some err"),
					retryOnErr:               true,
					continueWorkflowOnErr:    false,
					stopWorkflow:             false,
				},
				&stepMock{
					name:                  "cmd3",
					execute:               nil,
					retryOnErr:            false,
					continueWorkflowOnErr: false,
					stopWorkflow:          false,
				},
			},
			expectedResult: nil,
		},
		{
			name:  "a workflow with steps returning error, but configured to retry, should return an error if it not succeed on retry",
			input: struct{}{},
			mock: []Step{
				&stepMock{
					name:                     "cmd1",
					succeedAtInvocationCount: 0,
					execute:                  cmpopts.AnyError,
					retryOnErr:               true,
					continueWorkflowOnErr:    false,
					stopWorkflow:             false,
				},
				&stepMock{
					name:                  "cmd3",
					execute:               nil,
					retryOnErr:            false,
					continueWorkflowOnErr: false,
					stopWorkflow:          false,
				},
			},
			expectedResult: cmpopts.AnyError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSequential("some-workflow", tt.mock, WithRetryOption(2, 0))
			actualResult := c.Execute(context.TODO(), tt.input)

			if diff := cmp.Diff(actualResult, tt.expectedResult, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("\n Returned value was not as expected \n actual result = %#v, \n expected result: %#v \n DIFF: %v \n",
					actualResult,
					tt.expectedResult,
					diff,
				)
			}
		})
	}
}

func Test_Execute_BehaviourOnRetry(t *testing.T) {
	type inp struct {
		req             interface{}
		workflowOptions []func(*Sequential)
	}
	tests := []struct {
		name           string
		input          inp
		mock           []Step
		expectedResult int
	}{
		{
			name: "a workflow with steps returning error, but configured to retry, should stop retrying if the step returns false on retry",
			input: inp{
				req:             struct{}{},
				workflowOptions: []func(sequential2 *Sequential){WithRetryOption(2, 0)},
			},
			mock: []Step{
				&stepMock{
					name:                                   "cmd1",
					succeedAtInvocationCount:               0,
					execute:                                cmpopts.AnyError,
					retryOnErr:                             true,
					retryOnErrReturnFalseAtInvocationCount: 1,
					continueWorkflowOnErr:                  false,
					stopWorkflow:                           false,
				},
			},
			expectedResult: 1,
		},
		{
			name: "a workflow with steps returning error, but configured to retry, should not stop retrying if the step don't change its retry flag",
			input: inp{
				req:             struct{}{},
				workflowOptions: []func(sequential2 *Sequential){WithRetryOption(2, 0)},
			},
			mock: []Step{
				&stepMock{
					name:                     "cmd1",
					succeedAtInvocationCount: 0,
					execute:                  cmpopts.AnyError,
					retryOnErr:               true,
					continueWorkflowOnErr:    false,
					stopWorkflow:             false,
				},
			},
			expectedResult: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSequential("order-extractor", tt.mock, tt.input.workflowOptions...)
			_ = c.Execute(context.TODO(), tt.input.req)

			actualResult := c.steps[0].(*stepMock).invocationCount
			if diff := cmp.Diff(tt.expectedResult, actualResult, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("\n Returned value was not as expected \n actual result = %#v, \n expected result: %#v \n DIFF: %v \n",
					actualResult,
					tt.expectedResult,
					diff,
				)
			}
		})
	}
}

func Test_Execute_BehaviourOnWorkflowHooks(t *testing.T) {
	tests := []struct {
		name  string
		input []Step
		mock  []Step
		// number of invoked hooks
		expectedResult int
	}{
		{
			name: "a workflow with HOOKS should always run them no mather what happens in the workflow",
			input: []Step{
				&stepMock{
					name:    "after workflow HOOK cmd1",
					execute: errors.New("some err"),
				},
				&stepMock{
					name:    "after workflow HOOK cmd2",
					execute: nil,
				},
			},
			mock: []Step{
				&stepMock{
					name:    "cmd1",
					execute: nil,
				},
				&stepMock{
					name:    "cmd2",
					execute: nil,
				},
			},
			expectedResult: 2,
		},
		{
			name: "a workflow with HOOKS should always run them no mather what happens in the workflow and even if one of the hooks returns an error",
			input: []Step{
				&stepMock{
					name:    "after workflow HOOK cmd1",
					execute: errors.New("some err"),
				},
				&stepMock{
					name:    "after workflow HOOK cmd2",
					execute: nil,
				},
				&stepMock{
					name:    "after workflow HOOK cmd3",
					execute: nil,
				},
				&stepMock{
					name:    "after workflow HOOK cmd4",
					execute: errors.New("some err"),
				},
			},
			mock: []Step{
				&stepMock{
					name:    "cmd1",
					execute: nil,
				},
				&stepMock{
					name:    "cmd2",
					execute: errors.New("some err"),
				},
			},
			expectedResult: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSequential("order-extractor", tt.mock, WithRetryOption(2, 0))
			for i := range tt.input {
				c.AddAfterWorkflowHook(tt.input[i])
			}
			_ = c.Execute(context.TODO(), struct{}{})
			actualResult := 0
			for j := range tt.input {
				convertedCmd := tt.input[j].(*stepMock)
				actualResult += convertedCmd.invocationCount
			}

			if actualResult != tt.expectedResult {
				t.Errorf("\n Returned value was not as expected \n actual result = %#v, \n expected result: %#v \n",
					actualResult,
					tt.expectedResult,
				)
			}
		})
	}
}

func Test_Execute_BehaviourOnStoppingWorkflow(t *testing.T) {
	type expRes struct {
		lastCmdWasInvoked bool
	}
	tests := []struct {
		name           string
		input          interface{}
		mock           []Step
		expectedResult expRes
	}{
		{
			name:  "a workflow with steps returning errors, but configured not to stop on them, should invoke all the steps",
			input: struct{}{},
			mock: []Step{
				&stepMock{
					name:                  "cmd1",
					execute:               errors.New("some err"),
					retryOnErr:            false,
					continueWorkflowOnErr: true,
					stopWorkflow:          false,
				},
				&stepMock{
					name:                  "cmd2",
					execute:               errors.New("some err"),
					retryOnErr:            false,
					continueWorkflowOnErr: true,
					stopWorkflow:          false,
				},
				&stepMock{
					name:                  "cmd1",
					invocationCount:       0,
					execute:               nil,
					retryOnErr:            false,
					continueWorkflowOnErr: false,
					stopWorkflow:          false,
				},
			},
			expectedResult: expRes{lastCmdWasInvoked: true},
		},
		{
			name:  "a workflow with steps configured to stop it will immediately stop the workflow",
			input: struct{}{},
			mock: []Step{
				&stepMock{
					name:                  "cmd1",
					execute:               nil,
					retryOnErr:            false,
					continueWorkflowOnErr: false,
					stopWorkflow:          false,
				},
				&stepMock{
					name:                  "cmd2",
					execute:               nil,
					retryOnErr:            false,
					continueWorkflowOnErr: true,
					stopWorkflow:          true,
				},
				&stepMock{
					name:                  "cmd3",
					execute:               nil,
					retryOnErr:            false,
					continueWorkflowOnErr: false,
					stopWorkflow:          false,
				},
			},
			expectedResult: expRes{lastCmdWasInvoked: false},
		},
	}
	testResultMap := map[bool]string{
		true:  "stopped",
		false: "not stopped",
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSequential("order-extractor", tt.mock, WithRetryOption(2, 0))
			_ = c.Execute(context.TODO(), tt.input)

			actualResult := tt.mock[len(tt.mock)-1].(*stepMock).invocationCount > 0
			expectedResult := tt.expectedResult.lastCmdWasInvoked

			if actualResult != expectedResult {
				t.Errorf("\n The workflow did not behave as expected \n actual result = %#v, \n expected result: %#v \n",
					testResultMap[actualResult],
					testResultMap[tt.expectedResult.lastCmdWasInvoked],
				)
			}
		})
	}
}

type stepMock struct {
	invocationCount                        int
	succeedAtInvocationCount               int
	name                                   string
	execute                                error
	retryOnErr                             bool
	retryOnErrReturnFalseAtInvocationCount int
	continueWorkflowOnErr                  bool
	stopWorkflow                           bool
}

func (c *stepMock) Name() string {
	return c.name
}

func (c *stepMock) Execute(ctx context.Context, request any) error {
	c.invocationCount++
	if c.succeedAtInvocationCount == c.invocationCount {
		return nil
	}
	return c.execute
}

func (c *stepMock) CanRetry() bool {
	if c.retryOnErrReturnFalseAtInvocationCount == c.invocationCount {
		return false
	}

	return c.retryOnErr
}

func (c *stepMock) ContinueWorkflowOnError() bool {
	return c.continueWorkflowOnErr
}

func (c *stepMock) StopWorkflow() bool {
	return c.stopWorkflow
}
