package workflow

import (
	"context"
	"errors"
	"testing"
)

func TestPipeExecuteBehaviourOnPreservingErrorsType(t *testing.T) {
	anyErr := errors.New("any-error")
	input := []PipeStepConfig[any]{
		{Step: newPipeStepSuccessful[any]("cmd1")},
		{Step: newPipeStepFailedNonRetryable[any]("cmd2", anyErr)},
	}

	c := NewPipe("some-workflow", input, nil)
	_, actualOutputErr := c.Execute(context.TODO(), nil)
	expectedOutputErr := anyErr

	if !errors.Is(actualOutputErr, expectedOutputErr) {
		t.Errorf("The workflow error does not wrap the inner errors: \n expected = %#v, \n actual = %#v", expectedOutputErr, actualOutputErr)
	}
}

func TestPipeExecuteBehaviourOnReturningErrors(t *testing.T) {
	anyErr := errors.New("any-err")
	tests := []struct {
		name           string
		input          []PipeStepConfig[any]
		expectedOutput error
	}{
		{
			name:           "an workflow with an empty step collection should return a nil error",
			input:          nil,
			expectedOutput: nil,
		},
		{
			name: "a workflow with steps returning errors, but configured not to stop on them, should return an error",
			input: []PipeStepConfig[any]{
				{Step: newPipeStepFailedRetryable[any]("step 1", anyErr)},
				{Step: newPipeStepSuccessful[any]("step 2")},
			},
			expectedOutput: anyErr,
		},
		{
			name: "a workflow with steps returning errors, but configured to retry, should not return an error if it succeeds on retry",
			input: []PipeStepConfig[any]{
				{Step: newPipeStepFailedRetryableRecoverable[any]("step 1", anyErr, 2), RetryConfigProvider: defaultRetryConfigProviderTest},
				{Step: newPipeStepFailedRetryableRecoverable[any]("step 2", anyErr, 3), RetryConfigProvider: defaultRetryConfigProviderTest},
				{Step: newPipeStepSuccessful[any]("step 3")},
			},
			expectedOutput: nil,
		},
		{
			name: "a workflow with steps returning errors, but configured to retry, should return an error if it not succeed on retry",
			input: []PipeStepConfig[any]{
				{Step: newPipeStepFailedRetryable[any]("step 1", anyErr), RetryConfigProvider: defaultRetryConfigProviderTest},
				{Step: newPipeStepSuccessful[any]("step 2"), RetryConfigProvider: defaultRetryConfigProviderTest},
			},
			expectedOutput: anyErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewPipe("some-workflow", tt.input, nil)
			_, actualOutput := c.Execute(context.TODO(), nil)

			if !errors.Is(actualOutput, tt.expectedOutput) {
				t.Errorf("The workflow returned error behaviour not as expected: \n expected = %#v \n actual = %#v",
					tt.expectedOutput,
					actualOutput,
				)
			}
		})
	}
}

func TestPipeExecuteBehaviourOnRetry(t *testing.T) {
	anyErr := errors.New("any-err")
	tests := []struct {
		name           string
		input          []PipeStepConfig[any]
		expectedOutput int
	}{
		{
			name: "a workflow with steps returning errors, but configured to retry, should stop retrying if the step returns false on retry",
			input: []PipeStepConfig[any]{
				{Step: newPipeStepFailedNonRetryableFromInvocationCount[any]("step 1", anyErr, 1), RetryConfigProvider: defaultRetryConfigProviderTest},
			},
			expectedOutput: 1,
		},
		{
			name: "a workflow with steps returning errors, but configured to retry, should not stop retrying if the step don't change its retry flag",
			input: []PipeStepConfig[any]{
				{Step: newPipeStepFailedRetryable[any]("step 1", anyErr), RetryConfigProvider: defaultRetryConfigProviderTest},
			},
			expectedOutput: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewPipe("some-workflow", tt.input, nil)
			c.Execute(context.TODO(), nil)

			actualOutput := c.stepsConfig[0].Step.(*pipeStepMock[any]).invocationCount
			if tt.expectedOutput != actualOutput {
				t.Errorf("The workflow behaviour on retry, not as expected: \n actual = %#v, \n expected = %#v",
					actualOutput,
					tt.expectedOutput,
				)
			}
		})
	}
}

func TestPipeExecuteBehaviourOnStoppingWorkflow(t *testing.T) {
	anyErr := errors.New("any-err")
	testResultMap := map[bool]string{
		false: "stopped",
		true:  "not stopped",
	}
	type behaviour struct {
		lastCmdWasInvoked bool
	}

	tests := []struct {
		name           string
		input          []PipeStepConfig[any]
		expectedOutput behaviour
	}{
		{
			name: "a workflow with steps returning errors, should stop at the first step error",
			input: []PipeStepConfig[any]{
				{Step: newPipeStepFailedNonRetryable[any]("step 1", anyErr)},
				{Step: newPipeStepFailedNonRetryable[any]("step 2", anyErr)},
				{Step: newPipeStepSuccessful[any]("step 3")},
			},
			expectedOutput: behaviour{lastCmdWasInvoked: false},
		},
		{
			name: "a workflow with no failing steps, should run all of them",
			input: []PipeStepConfig[any]{
				{Step: newPipeStepSuccessful[any]("step 1")},
				{Step: newPipeStepSuccessful[any]("step 2")},
				{Step: newPipeStepSuccessful[any]("step 3")},
			},
			expectedOutput: behaviour{lastCmdWasInvoked: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewPipe("some-workflow", tt.input, nil)
			c.Execute(context.TODO(), nil)

			actualOutput := tt.input[len(tt.input)-1].Step.(*pipeStepMock[any]).invocationCount > 0
			expectedOutput := tt.expectedOutput.lastCmdWasInvoked

			if actualOutput != expectedOutput {
				t.Errorf("The workflow did not behave as expected \n actual result = %#v, \n expected result: %#v \n",
					testResultMap[actualOutput],
					testResultMap[expectedOutput],
				)
			}
		})
	}
}

// BENCHMARKS

// BenchmarkPipeExecuteHappyFlow performs a benchmark for the scenario in which there is no error in the workflow.
// This should produce 0 allocations.
func BenchmarkPipeExecuteHappyFlow(b *testing.B) {
	stepsCfgW2 := []PipeStepConfig[any]{
		{Step: newPipeStepSuccessful[any]("log-request-data")},
		{Step: newPipeStepSuccessful[any]("notify-monitoring-system")},
	}
	w2 := NewPipe("monitoring-workflow", stepsCfgW2, nil)
	stepsCfg := []PipeStepConfig[any]{
		{Step: newPipeStepSuccessful[any]("extract-data-from-data-provider")},
		{Step: newPipeStepSuccessful[any]("transform-data-extracted-from-data-provider")},
		{Step: newPipeStepSuccessful[any]("load-the-data-into-the-data-source")},
		{Step: w2},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := NewPipe("just-execute-these-steps-workflow", stepsCfg, nil)
		s.Execute(context.TODO(), nil)
	}
}

// BenchmarkPipeExecuteErrFlowOneErr performs a benchmark for the scenario in which there are errors in the workflow.
// This should produce 3 allocations, corresponding to the error usage (allocation for the error slice and errors.Join)
func BenchmarkPipeExecuteErrFlowErr(b *testing.B) {
	stepsCfgW2 := []PipeStepConfig[any]{
		{Step: newPipeStepSuccessful[any]("log-request-data")},
		{Step: newPipeStepSuccessful[any]("notify-monitoring-system")},
	}
	w2 := NewPipe("monitoring-workflow", stepsCfgW2, nil)
	stepsCfg := []PipeStepConfig[any]{
		{Step: newPipeStepSuccessful[any]("extract-data-from-data-provider")},
		{
			Step:                newPipeStepFailedRetryable[any]("transform-data-extracted-from-data-provider", errors.New("some err")),
			RetryConfigProvider: defaultRetryConfigProviderTest,
		},
		{Step: newPipeStepSuccessful[any]("load-the-data-into-the-data-source")},
		{Step: w2},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := NewPipe("just-execute-these-steps-workflow", stepsCfg, nil)
		s.Execute(context.TODO(), nil)
	}
}

// MOCKS/STUBS
type pipeStepMock[T any] struct {
	invocationCount                      int
	succeedAtInvocationCount             int
	name                                 string
	execute                              executeOutput[T]
	canRetry                             bool
	canRetryReturnFalseAtInvocationCount int
}
type executeOutput[T any] struct {
	val   T
	error error
}

func (c *pipeStepMock[T]) Name() string {
	return c.name
}

func (c *pipeStepMock[T]) Execute(ctx context.Context, request T) (T, error) {
	c.invocationCount++
	if c.succeedAtInvocationCount == c.invocationCount {
		return c.execute.val, nil
	}
	return c.execute.val, c.execute.error
}

func (c *pipeStepMock[T]) CanRetry() bool {
	if c.canRetryReturnFalseAtInvocationCount == c.invocationCount {
		return false
	}

	return c.canRetry
}

func newPipeStepSuccessful[T any](name string) *pipeStepMock[T] {
	return &pipeStepMock[T]{name: name}
}
func newPipeStepFailedRetryable[T any](name string, failWith error) *pipeStepMock[T] {
	return &pipeStepMock[T]{name: name, execute: executeOutput[T]{error: failWith}, canRetry: true}
}
func newPipeStepFailedNonRetryableFromInvocationCount[T any](name string, failWith error, nonRetryableFromInvocationCount int) *pipeStepMock[T] {
	return &pipeStepMock[T]{name: name, execute: executeOutput[T]{error: failWith}, canRetry: true, canRetryReturnFalseAtInvocationCount: nonRetryableFromInvocationCount}
}
func newPipeStepFailedRetryableRecoverable[T any](name string, failWith error, succeedsAtInvocationCount int) *pipeStepMock[T] {
	return &pipeStepMock[T]{name: name, execute: executeOutput[T]{error: failWith}, canRetry: true, succeedAtInvocationCount: succeedsAtInvocationCount}
}
func newPipeStepFailedNonRetryable[T any](name string, failWith error) *pipeStepMock[T] {
	return &pipeStepMock[T]{name: name, execute: executeOutput[T]{error: failWith}, canRetry: false}
}
