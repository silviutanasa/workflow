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
	steps := []StepConfig{
		{
			Step: &stepMock{
				name:     "cmd2",
				execute:  errors.New("any err"),
				canRetry: false,
			},
			ContinueWorkflowOnError: true,
		},
		{
			Step: &stepMock{
				name:     "cmd3",
				execute:  testRetryableErr{},
				canRetry: false,
			},
			ContinueWorkflowOnError: true,
		},
	}

	c := NewSequential("order-extractor", steps, nil, RetryConfig{2, 0}, nil)
	actualResult := c.Execute(context.TODO(), nil)

	var expectedErr testRetryableErr
	if !errors.As(actualResult, &expectedErr) {
		t.Errorf("the workflow error does not embed the inner errors(errors returned by component stepsConfig)")
	}
}

func Test_Execute_BehaviourOnReturningErrors(t *testing.T) {
	tests := []struct {
		name           string
		input          interface{}
		mock           []StepConfig
		expectedResult error
	}{
		{
			name:           "an workflow with an empty chain should not return an error",
			input:          struct{}{},
			mock:           []StepConfig{},
			expectedResult: nil,
		},
		{
			name:  "a workflow with stepsConfig returning errors, but configured not to stop on them, should return an error",
			input: struct{}{},
			mock: []StepConfig{
				{
					Step: &stepMock{
						name:     "cmd2",
						execute:  cmpopts.AnyError,
						canRetry: false,
					},
					ContinueWorkflowOnError: true,
				},
				{
					Step: &stepMock{
						name:     "cmd3",
						execute:  nil,
						canRetry: false,
					},
					ContinueWorkflowOnError: false,
				},
			},
			expectedResult: cmpopts.AnyError,
		},
		{
			name:  "a workflow with stepsConfig returning error, but configured to retry, should not return an error if it succeeds on retry",
			input: struct{}{},
			mock: []StepConfig{
				{
					Step: &stepMock{
						name:                     "cmd1",
						succeedAtInvocationCount: 2,
						execute:                  errors.New("some err"),
						canRetry:                 true,
					},
					ContinueWorkflowOnError: false,
				},
				{
					Step: &stepMock{
						name:                     "cmd2",
						succeedAtInvocationCount: 3,
						execute:                  errors.New("some err"),
						canRetry:                 true,
					},
					ContinueWorkflowOnError: false,
				},
				{
					Step: &stepMock{
						name:     "cmd3",
						execute:  nil,
						canRetry: false,
					},
					ContinueWorkflowOnError: false,
				},
			},
			expectedResult: nil,
		},
		{
			name:  "a workflow with stepsConfig returning error, but configured to retry, should return an error if it not succeed on retry",
			input: struct{}{},
			mock: []StepConfig{
				{
					Step: &stepMock{
						name:                     "cmd1",
						succeedAtInvocationCount: 0,
						execute:                  cmpopts.AnyError,
						canRetry:                 true,
					},
					ContinueWorkflowOnError: false,
				},
				{
					Step: &stepMock{
						name:     "cmd3",
						execute:  nil,
						canRetry: false,
					},
					ContinueWorkflowOnError: false,
				},
			},
			expectedResult: cmpopts.AnyError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSequential("some-workflow", tt.mock, nil, RetryConfig{2, 0}, nil)
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
		req interface{}
	}
	tests := []struct {
		name           string
		input          inp
		mock           []StepConfig
		expectedResult int
	}{
		{
			name: "a workflow with stepsConfig returning error, but configured to retry, should stop retrying if the step returns false on retry",
			input: inp{
				req: struct{}{},
			},
			mock: []StepConfig{
				{
					Step: &stepMock{
						name:                                 "cmd1",
						succeedAtInvocationCount:             0,
						execute:                              cmpopts.AnyError,
						canRetry:                             true,
						canRetryReturnFalseAtInvocationCount: 1,
					},
					ContinueWorkflowOnError: false,
				},
			},
			expectedResult: 1,
		},
		{
			name: "a workflow with stepsConfig returning error, but configured to retry, should not stop retrying if the step don't change its retry flag",
			input: inp{
				req: struct{}{},
			},
			mock: []StepConfig{
				{
					Step: &stepMock{
						name:                     "cmd1",
						succeedAtInvocationCount: 0,
						execute:                  cmpopts.AnyError,
						canRetry:                 true,
					},
					ContinueWorkflowOnError: false,
				},
			},
			expectedResult: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSequential("zorder-extractor", tt.mock, nil, RetryConfig{2, 0}, nil)
			_ = c.Execute(context.TODO(), tt.input.req)

			actualResult := c.stepsConfig[0].Step.(*stepMock).invocationCount
			if diff := cmp.Diff(tt.expectedResult, actualResult); diff != "" {
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
		input []StepConfig
		mock  []Step
		// number of invoked hooks
		expectedResult int
	}{
		{
			name: "a workflow with HOOKS should always run them no mather what happens in the workflow",
			input: []StepConfig{
				{
					Step: &stepMock{
						name:    "after workflow HOOK cmd1",
						execute: errors.New("some err"),
					},
					ContinueWorkflowOnError: false,
				},
				{
					Step: &stepMock{
						name:    "after workflow HOOK cmd2",
						execute: nil,
					},
					ContinueWorkflowOnError: false,
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
			input: []StepConfig{
				{
					Step: &stepMock{
						name:    "after workflow HOOK cmd1",
						execute: nil,
					},
					ContinueWorkflowOnError: false,
				},
				{
					Step: &stepMock{
						name:    "after workflow HOOK cmd2",
						execute: nil,
					},
					ContinueWorkflowOnError: false,
				},
				{
					Step: &stepMock{
						name:    "after workflow HOOK cmd3",
						execute: nil,
					},
					ContinueWorkflowOnError: false,
				},
				{
					Step: &stepMock{
						name:    "after workflow HOOK cmd4",
						execute: errors.New("some err"),
					},
					ContinueWorkflowOnError: false,
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
			expectedResult: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSequential("order-extractor", tt.input, tt.mock, RetryConfig{2, 0}, nil)
			_ = c.Execute(context.TODO(), struct{}{})
			actualResult := 0
			for j := range tt.mock {
				convertedCmd := tt.mock[j].(*stepMock)
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
		mock           []StepConfig
		expectedResult expRes
	}{
		{
			name:  "a workflow with stepsConfig returning errors, but configured not to stop on them, should invoke all the stepsConfig",
			input: struct{}{},
			mock: []StepConfig{
				{
					Step: &stepMock{
						name:     "cmd1",
						execute:  errors.New("some err"),
						canRetry: false,
					},
					ContinueWorkflowOnError: true,
				},
				{
					Step: &stepMock{
						name:     "cmd2",
						execute:  errors.New("some err"),
						canRetry: false,
					},
					ContinueWorkflowOnError: true,
				},
				{
					Step: &stepMock{
						name:            "cmd1",
						invocationCount: 0,
						execute:         nil,
						canRetry:        false,
					},
					ContinueWorkflowOnError: false,
				},
			},
			expectedResult: expRes{lastCmdWasInvoked: true},
		},
	}
	testResultMap := map[bool]string{
		true:  "stopped",
		false: "not stopped",
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSequential("order-extractor", tt.mock, nil, RetryConfig{2, 0}, nil)
			_ = c.Execute(context.TODO(), tt.input)

			actualResult := tt.mock[len(tt.mock)-1].Step.(*stepMock).invocationCount > 0
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
	invocationCount                      int
	succeedAtInvocationCount             int
	name                                 string
	execute                              error
	canRetry                             bool
	canRetryReturnFalseAtInvocationCount int
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
	if c.canRetryReturnFalseAtInvocationCount == c.invocationCount {
		return false
	}

	return c.canRetry
}

// BENCHMARKS
func BenchmarkSequential(b *testing.B) {
	s1 := stepMock{
		name:     "cmd1",
		execute:  nil,
		canRetry: false,
	}
	s2 := stepMock{
		name:     "cmd2",
		execute:  nil,
		canRetry: false,
	}
	s3 := stepMock{
		name:     "cmd3",
		execute:  nil,
		canRetry: false,
	}
	stepsCfg := []StepConfig{
		{
			Step:                    &s1,
			ContinueWorkflowOnError: false,
		},
		{
			Step:                    &s2,
			ContinueWorkflowOnError: false,
		},
		{
			Step:                    &s3,
			ContinueWorkflowOnError: false,
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		seq := NewSequential("test-service", stepsCfg, nil, RetryConfig{2, 0}, nil)
		_ = seq.Execute(context.TODO(), struct{}{})
	}
}
