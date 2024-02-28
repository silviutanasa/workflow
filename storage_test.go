package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// Scenario when the same request is executed twice but only the previously failed steps will run.
// Iteration 1:
// correlation_id_1:
//
//	step1: success
//	step2: failed
//
// -------------
// Iteration 2:
// correlation_id_1: --same correlation id
//
//	step1: skipped
//	step2: success
func Test_Sequential_With_Storage(t *testing.T) {
	anyErr := errors.New("any-error")

	successStep1 := SequentialStepConfig[any]{Step: newStepSuccessful("step1")}
	failedStep2 := SequentialStepConfig[any]{Step: newStepFailedNonRetryable("step2", anyErr)}
	successStep2 := SequentialStepConfig[any]{Step: newStepSuccessful("step2")}

	corID := "abcd-1234-defg-5678"
	storage := NewInMemoryRepository()

	c := NewSequential(
		"some-workflow",
		[]SequentialStepConfig[any]{successStep1, failedStep2},
		WithStorage(storage),
		WithCorrelationID(corID),
	)

	// 1) Step 2 will fail and the workflow will exit with error.
	actualOutput := c.Execute(context.TODO(), nil)
	expectedOutput := anyErr
	if !errors.Is(actualOutput, expectedOutput) {
		t.Errorf("the workflow error does not wrap the inner errors: \n expected = %#v, \n actual = %#v", expectedOutput, actualOutput)
		t.FailNow()
	}

	// 2) The workflow will get reinitialized with the same correlationID. Because the previous workflow steps execution is stored in db,
	// only the step(s) that failed previously will get executed.
	c2 := NewSequential(
		"some-workflow",
		[]SequentialStepConfig[any]{successStep1, successStep2}, // we mark step2 as successful now
		WithStorage(storage),
		WithCorrelationID(corID),
	)

	gotErr := c2.Execute(context.TODO(), nil)
	if gotErr != nil {
		t.Errorf("the workflow returned error: %#v", gotErr)
		t.FailNow()
	}

	// step1 has been passed twice to the workflow, but on the second run it should be skipped.
	successfulStepConverted := (interface{})(successStep1.Step).(*stepMock)
	if successfulStepConverted.invocationCount != 1 {
		t.Errorf("step %s should be processed only once, actual count %d", successStep1.Step.Name(), successfulStepConverted.invocationCount)
		t.FailNow()
	}
}

// --------------------------
// In memory storage mock.

type repositoryStep struct {
	name   string
	status string
	output *string
}

type Repository struct {
	steps      map[string][]repositoryStep
	sync.Mutex // safeguard map with locking mechanism.
}

func NewInMemoryRepository() *Repository {
	return &Repository{
		steps: make(map[string][]repositoryStep),
	}
}

func (r *Repository) Save(ctx context.Context, stepName, correlationID string, status StepStatus, output *string) error {
	r.Lock()
	defer r.Unlock()

	s := repositoryStep{
		name:   clean([]byte(stepName)), // perform sanitization
		status: string(status),
		output: output,
	}
	r.steps[correlationID] = append(r.steps[correlationID], s)

	return nil
}

func (r *Repository) Get(ctx context.Context, stepName, correlationID string) (StepStatus, error) {
	r.Lock()
	defer r.Unlock()

	if _, ok := r.steps[correlationID]; !ok {
		return "", fmt.Errorf("there are no steps stored for correlationID: %s", correlationID)
	}

	cleanStepName := clean([]byte(stepName))
	for _, s := range r.steps[correlationID] {
		if s.name == cleanStepName {
			return StepStatus(s.status), nil
		}
	}

	return "", fmt.Errorf("step %s not found", stepName)
}

func (r *Repository) Clear(ctx context.Context, correlationID string) error {
	r.Lock()
	defer r.Unlock()

	r.steps[correlationID] = []repositoryStep{}
	return nil
}

// clean will remove all non-alphanumeric characters, except space and underscore then replace space with underscore.
func clean(s []byte) string {
	j := 0
	for _, b := range s {
		if ('a' <= b && b <= 'z') ||
			('A' <= b && b <= 'Z') ||
			('0' <= b && b <= '9') ||
			// detect consecutive spaces and append only one space instead
			(b == ' ' && (j != 0 && s[j-1] != ' ')) ||
			// detect consecutive underscore and append only one underscore instead
			(b == '_' && (j != 0 && s[j-1] != '_')) {
			s[j] = b
			j++
		}
	}

	return strings.ReplaceAll(string(s[:j]), " ", "_")
}
