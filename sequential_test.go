package workflow

import (
	"context"
	"errors"
	"testing"
	"time"
)

var defaultRetryConfigProviderTest = func() (maxAttempts uint, attemptDelay time.Duration) { return 2, time.Nanosecond }

func TestSequentialExecuteBehaviourOnPreservingErrorsType(t *testing.T) {
	anyErr := errors.New("any-error")
	input := []SequentialStepConfig[any]{
		{Step: newStepSuccessful("step1")},
		{Step: newStepFailedNonRetryable("step2", anyErr)},
	}

	c := NewSequential("some-workflow", input)
	actualOutput := c.Execute(context.TODO(), nil)
	expectedOutput := anyErr

	if !errors.Is(actualOutput, expectedOutput) {
		t.Errorf("the workflow error does not wrap the inner errors: \n expected = %#v, \n actual = %#v", expectedOutput, actualOutput)
	}
}

func TestSequentialExecuteBehaviourOnReturningErrors(t *testing.T) {
	anyErr := errors.New("any-err")
	tests := []struct {
		name           string
		input          []SequentialStepConfig[any]
		expectedOutput error
	}{
		{
			name:           "a workflow with an empty step collection should return a nil error",
			input:          nil,
			expectedOutput: nil,
		},
		{
			name: "a workflow with steps returning errors, but configured not to stop on them, should return an error",
			input: []SequentialStepConfig[any]{
				{Step: newStepFailedRetryable("step 1", anyErr), ContinueWorkflowOnError: true},
				{Step: newStepSuccessful("step 2")},
			},
			expectedOutput: anyErr,
		},
		{
			name: "a workflow with steps returning errors, but configured to retry, should not return an error if it succeeds on retry",
			input: []SequentialStepConfig[any]{
				{Step: newStepFailedRetryableRecoverable("step 1", anyErr, 2), RetryConfigProvider: defaultRetryConfigProviderTest},
				{Step: newStepFailedRetryableRecoverable("step 2", anyErr, 3), RetryConfigProvider: defaultRetryConfigProviderTest},
				{Step: newStepSuccessful("step 3")},
			},
			expectedOutput: nil,
		},
		{
			name: "a workflow with steps returning errors, but configured to retry, should return an error if it not succeed on retry",
			input: []SequentialStepConfig[any]{
				{Step: newStepFailedRetryable("step 1", anyErr), RetryConfigProvider: defaultRetryConfigProviderTest},
				{Step: newStepSuccessful("step 2"), RetryConfigProvider: defaultRetryConfigProviderTest},
			},
			expectedOutput: anyErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSequential("some-workflow", tt.input)
			actualOutput := c.Execute(context.TODO(), nil)

			if !errors.Is(actualOutput, tt.expectedOutput) {
				t.Errorf("The workflow returned error behaviour not as expected: \n expected = %#v \n actual = %#v",
					tt.expectedOutput,
					actualOutput,
				)
			}
		})
	}
}

func TestSequentialExecuteBehaviourOnRetry(t *testing.T) {
	anyErr := errors.New("any-err")
	tests := []struct {
		name           string
		input          []SequentialStepConfig[any]
		expectedOutput int
	}{
		{
			name: "a workflow with steps returning errors, but configured to retry, should stop retrying if the step returns false on retry",
			input: []SequentialStepConfig[any]{
				{Step: newStepFailedNonRetryableFromInvocationCount("step 1", anyErr, 1), RetryConfigProvider: defaultRetryConfigProviderTest},
			},
			expectedOutput: 1,
		},
		{
			name: "a workflow with steps returning errors, but configured to retry, should not stop retrying if the step don't change its retry flag",
			input: []SequentialStepConfig[any]{
				{Step: newStepFailedRetryable("step 1", anyErr), RetryConfigProvider: defaultRetryConfigProviderTest},
			},
			expectedOutput: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSequential("some-workflow", tt.input)
			c.Execute(context.TODO(), nil)

			actualOutput := c.stepsConfig[0].Step.(*stepMock).invocationCount
			if tt.expectedOutput != actualOutput {
				t.Errorf("The workflow behaviour on retry, not as expected: \n actual = %#v, \n expected = %#v",
					actualOutput,
					tt.expectedOutput,
				)
			}
		})
	}
}

func TestSequentialExecuteBehaviourOnStoppingWorkflow(t *testing.T) {
	anyErr := errors.New("any-err")
	testResultMap := map[bool]string{
		true:  "stopped",
		false: "not stopped",
	}
	type behaviour struct {
		lastCmdWasInvoked bool
	}

	tests := []struct {
		name           string
		input          []SequentialStepConfig[any]
		expectedOutput behaviour
	}{
		{
			name: "a workflow with steps returning errors, but configured not to stop on them, should invoke all the steps",
			input: []SequentialStepConfig[any]{
				{Step: newStepFailedNonRetryable("step 1", anyErr), ContinueWorkflowOnError: true},
				{Step: newStepFailedNonRetryable("step 2", anyErr), ContinueWorkflowOnError: true},
				{Step: newStepSuccessful("step 3")},
			},
			expectedOutput: behaviour{lastCmdWasInvoked: true},
		},
		{
			name: "a workflow with steps returning errors, but configured to stop on them, should not invoke the remaining steps",
			input: []SequentialStepConfig[any]{
				{Step: newStepSuccessful("step 1")},
				{Step: newStepFailedNonRetryable("step 2", anyErr)},
				{Step: newStepSuccessful("step 3")},
			},
			expectedOutput: behaviour{lastCmdWasInvoked: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSequential("some-workflow", tt.input)
			c.Execute(context.TODO(), nil)

			actualOutput := tt.input[len(tt.input)-1].Step.(*stepMock).invocationCount > 0
			expectedOutput := tt.expectedOutput.lastCmdWasInvoked

			if actualOutput != expectedOutput {
				t.Errorf("The workflow did not behave as expected \n actual result = %#v, \n expected result: %#v \n",
					testResultMap[actualOutput],
					testResultMap[tt.expectedOutput.lastCmdWasInvoked],
				)
			}
		})
	}
}

// BENCHMARKS

// BenchmarkSequentialHappyFlow performs a benchmark for the scenario in which there is no error in the workflow.
// This should produce 0 allocations.
func BenchmarkSequentialExecuteHappyFlow(b *testing.B) {
	stepsCfgW2 := []SequentialStepConfig[any]{
		{Step: newStepSuccessful("log-request-data")},
		{Step: newStepSuccessful("notify-monitoring-system")},
	}
	w2 := NewSequential("monitoring-workflow", stepsCfgW2)
	stepsCfg := []SequentialStepConfig[any]{
		{Step: newStepSuccessful("extract-data-from-data-provider")},
		{Step: newStepSuccessful("transform-data-extracted-from-data-provider")},
		{Step: newStepSuccessful("load-the-data-into-the-data-source")},
		{Step: w2},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := NewSequential("just-execute-these-steps-workflow", stepsCfg)
		s.Execute(context.TODO(), nil)
	}
}

// BenchmarkSequentialErrFlow performs a benchmark for the scenario in which there are errors in the workflow.
// This should produce 3 allocations, corresponding to the error usage (allocation for the error slice and errors.Join)
func BenchmarkSequentialExecuteErrFlowOneErr(b *testing.B) {
	stepsCfgW2 := []SequentialStepConfig[any]{
		{Step: newStepSuccessful("log-request-data")},
		{Step: newStepSuccessful("notify-monitoring-system")},
	}
	w2 := NewSequential("monitoring-workflow", stepsCfgW2)
	stepsCfg := []SequentialStepConfig[any]{
		{Step: newStepSuccessful("extract-data-from-data-provider")},
		{
			Step:                    newStepFailedRetryable("transform-data-extracted-from-data-provider", errors.New("some err")),
			ContinueWorkflowOnError: true,
			RetryConfigProvider:     defaultRetryConfigProviderTest,
		},
		{Step: newStepSuccessful("load-the-data-into-the-data-source")},
		{Step: w2},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := NewSequential("just-execute-these-steps-workflow", stepsCfg)
		s.Execute(context.TODO(), nil)
	}
}

func BenchmarkSequentialExecuteErrFlowMultipleErr(b *testing.B) {
	stepsCfgW2 := []SequentialStepConfig[any]{
		{Step: newStepSuccessful("log-request-data")},
		{Step: newStepSuccessful("notify-monitoring-system")},
	}
	w2 := NewSequential("monitoring-workflow", stepsCfgW2)
	stepsCfg := []SequentialStepConfig[any]{
		//{SequentialStep: newStepSuccessful("extract-data-from-data-provider")},
		{Step: newStepFailedRetryable("extract-data-from-data-provider", errors.New("some err")), ContinueWorkflowOnError: true},
		{
			Step:                    newStepFailedRetryable("transform-data-extracted-from-data-provider", errors.New("some err")),
			ContinueWorkflowOnError: true,
			RetryConfigProvider:     defaultRetryConfigProviderTest,
		},
		{Step: newStepFailedRetryable("load-the-data-into-the-data-source", errors.New("some err")), ContinueWorkflowOnError: true},
		{Step: w2},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := NewSequential("just-execute-these-steps-workflow", stepsCfg, nil)
		s.Execute(context.TODO(), nil)
	}
}

// MOCKS/STUBS
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

func (c *stepMock) InvocationCount() int {
	return c.invocationCount
}

func newStepSuccessful(name string) *stepMock {
	return &stepMock{name: name}
}
func newStepFailedRetryable(name string, failWith error) *stepMock {
	return &stepMock{name: name, execute: failWith, canRetry: true}
}
func newStepFailedNonRetryableFromInvocationCount(name string, failWith error, nonRetryableFromInvocationCount int) *stepMock {
	return &stepMock{name: name, execute: failWith, canRetry: true, canRetryReturnFalseAtInvocationCount: nonRetryableFromInvocationCount}
}
func newStepFailedRetryableRecoverable(name string, failWith error, succeedsAtInvocationCount int) *stepMock {
	return &stepMock{name: name, execute: failWith, canRetry: true, succeedAtInvocationCount: succeedsAtInvocationCount}
}
func newStepFailedNonRetryable(name string, failWith error) *stepMock {
	return &stepMock{name: name, execute: failWith, canRetry: false}
}
