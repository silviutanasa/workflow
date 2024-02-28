# Workflow

[![GoDoc][doc-img]][doc]
[![Go Report Card][go-report-img]][go-report]

A library that allows you to decompose the business logic in steps and orchestrate them in a workflow. \
The abstraction, based on the <strong>composite</strong> pattern and <strong>chain of responsibility/commands</strong>,
allows for infinite nesting, as the workflow itself can be a component of another workflow and so on.

**Sequential** is a workflow that runs all of its steps/commands in a predefined order/sequence. \
Every step can be configured to stop the workflow on failure(default behaviour) or let it process the rest of the
steps. \
It has the ability to retry at the step level, with a configured number of attempts and delay. \
<u>Optionally</u> you can pass a storage option to the constructor to be able to replay the same request without the \
fear of duplicating successful steps. This becomes handy on microservice arch. when is needed to replay an event.


**Pipe** is a workflow that runs all of its steps/commands in a predefined order/sequence. \
Except for the first step, which receives the initial request as input, every subsequent step receives as an input, the
output of the previous step. Any failing step stops the workflow. \
It has the ability to retry at the step level, with a configured number of attempts and delay.

## Installation:

```
go get github.com/silviutanasa/workflow
```

Note that the minimum supported version is Go v1.20.

## Performance:

```
sequential happy flow(no errors on steps):                  0 allocs/op
seqential error flow(1 error and steps retries):            1 allocs/op
seqential error flow(multiple errors and steps retries):    3 allocs/op

pipe happy flow(no errors on steps):                        0 allocs/op
pipe error flow(1 error and steps retries):                 0 allocs/op
```

## Usage:
<details>
<summary>example 1: A set of steps sharing the same request as common state.</summary>

```Go
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/silviutanasa/workflow"
)

func main() {
	step1 := extractData{
		name:       "extract-some-data",
		dataSource: strings.NewReader("some string in lowercase"),
	}
	step2 := transformData{name: "transform-some-data"}
	step3 := sendData{name: "send-some-data"}
	wfConfig := []workflow.SequentialStepConfig[*request]{
		{Step: &step1},
		{Step: &step2},
		{Step: &step3},
	}
	wf := workflow.NewSequential("ETL", wfConfig)
	req := request{id: "1"}
	err := wf.Execute(context.Background(), &req)
	if err != nil {
		fmt.Printf("Workflow processed with errors: %v", err)
	}
}

type request struct {
	id        string
	inputData []byte
}

/*
EXTRACT
*/
type extractData struct {
	name       string
	dataSource io.Reader
}

func (e *extractData) Name() string {
	return e.name
}

func (e *extractData) Execute(ctx context.Context, req *request) error {
	inp, err := io.ReadAll(e.dataSource)
	if err != nil {
		return err
	}
	req.inputData = inp

	return nil
}

/*
TRANSFORM
*/
type transformData struct {
	name string
}

func (t *transformData) Name() string {
	return t.name
}

func (t *transformData) Execute(ctx context.Context, req *request) error {
	req.inputData = bytes.ToUpper(req.inputData)

	return nil
}

/*
LOAD
*/
type sendData struct {
	name string
}

func (s *sendData) Name() string {
	return s.name
}

func (s *sendData) Execute(ctx context.Context, req *request) error {
	fmt.Printf("%s", req.inputData)

	return nil
}


```
</details>
<details>
<summary>example 2: A set of steps with no shared state, that reacts to an external event (chain of responsibility).</summary> All the
steps have the chance to process the event(by setting "ContinueWorkflowOnError: true" for every StepConfig).

```Go
package main

import (
	"context"
	"fmt"

	"github.com/silviutanasa/workflow"
)

func main() {
	step1 := notifySalesDepartment{name: "notify-sales-department"}
	step2 := notifyManagementDepartment{name: "notify-management-department"}
	step3 := notifyPagerDuty{name: "notify-pager-duty"}
	step4 := notifyOnboardingDepartment{name: "notify-onboarding-department"}
	wfConfig := []workflow.SequentialStepConfig[event]{
		{Step: &step1, ContinueWorkflowOnError: true},
		{Step: &step2, ContinueWorkflowOnError: true},
		{Step: &step3, ContinueWorkflowOnError: true},
		{Step: &step4, ContinueWorkflowOnError: true},
	}
	wf := workflow.NewSequential("ETL", wfConfig)
	req := event{name: "client created"}
	err := wf.Execute(context.Background(), req)
	if err != nil {
		fmt.Printf("Workflow processed with errors: %v", err)
	}
}

type event struct {
	name string
}

/*
NOTIFY SALES DEPARTMENT
*/
type notifySalesDepartment struct {
	name string
}

func (e *notifySalesDepartment) Name() string {
	return e.name
}

func (e *notifySalesDepartment) Execute(ctx context.Context, req event) error {
	if req.name != "client created" {
		return nil
	}
	fmt.Println("Notifying sales department")

	return nil
}

/*
NOTIFY MANAGEMENT DEPARTMENT
*/
type notifyManagementDepartment struct {
	name string
}

func (t *notifyManagementDepartment) Name() string {
	return t.name
}

func (t *notifyManagementDepartment) Execute(ctx context.Context, req event) error {
	if req.name != "contract cancelled" {
		return nil
	}
	fmt.Println("Notifying management department")

	return nil
}

/*
NOTIFY PAGER DUTY
*/
type notifyPagerDuty struct {
	name string
}

func (s *notifyPagerDuty) Name() string {
	return s.name
}

func (s *notifyPagerDuty) Execute(ctx context.Context, req event) error {
	if req.name != "critical error" {
		return nil
	}
	fmt.Println("Notifying pager duty")

	return nil
}

/*
NOTIFY ONBOARDING DEPARTMENT
*/
type notifyOnboardingDepartment struct {
	name string
}

func (n *notifyOnboardingDepartment) Name() string {
	return n.name
}

func (n *notifyOnboardingDepartment) Execute(ctx context.Context, req event) error {
	if req.name != "client created" {
		return nil
	}
	fmt.Println("Notifying onboarding department")

	return nil
}

```
</details>
<details>
<summary>example 3: A set of steps that process a string in a "pipe like" way.</summary> All the
steps have the chance to process the event(by setting "ContinueWorkflowOnError: true" for every StepConfig).

```Go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/silviutanasa/workflow"
)

func main() {
	stepsCfg := []workflow.PipeStepConfig[string]{
		{Step: &trimSpaces{name: "trim-spaces"}},
		{Step: &removeCommas{name: "remove-commas"}},
		{Step: &removeDots{name: "remove-dots"}},
		{Step: &transformToUppercase{name: "transform-to-upper"}},
	}

	wf := workflow.NewPipe("ETL", stepsCfg, nil)
	output, err := wf.Execute(context.TODO(), "    I. am. the, string. to be transformed,   ,     ")
	if err != nil {
		fmt.Printf("Workflow processed with errors: %v", err)
	}
}

type trimSpaces struct {
	name string
}

func (s *trimSpaces) Name() string {
	return s.name
}

func (s *trimSpaces) Execute(ctx context.Context, req string) (string, error) {
	return strings.TrimSpace(req), nil
}

type transformToUppercase struct {
	name string
}

func (s *transformToUppercase) Name() string {
	return s.name
}

func (s *transformToUppercase) Execute(ctx context.Context, req string) (string, error) {
	return strings.ToUpper(req), nil
}

type removeCommas struct {
	name string
}

func (s *removeCommas) Name() string {
	return s.name
}

func (s *removeCommas) Execute(ctx context.Context, req string) (string, error) {
	return strings.ReplaceAll(req, ",", ""), nil
}

type removeDots struct {
	name string
}

func (s *removeDots) Name() string {
	return s.name
}

func (s *removeDots) Execute(ctx context.Context, req string) (string, error) {
	return strings.ReplaceAll(req, ".", ""), nil
}

```
</details>

<details>
<summary>example 4: Use Sequential workflow with storage to be able to store each step </summary> 
Each step result is stored and if the same request gets replayed the workflow will execute only previous failed steps.

See: `storage_test.go`
</details>

[doc-img]: https://pkg.go.dev/badge/silviutanasa/workflow
[doc]: https://pkg.go.dev/github.com/silviutanasa/workflow
[go-report-img]: https://goreportcard.com/badge/github.com/silviutanasa/workflow
[go-report]: https://goreportcard.com/report/github.com/silviutanasa/workflow