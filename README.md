# Workflow

A library that allows you to decompose the business logic in steps and orchestrate them in a workflow. \
The abstraction based on the <strong>composite</strong> pattern and <strong>chain of responsibility/commands</strong>
allows for infinite nesting, as the workflow itself can be a component of another workflow and so on.

**Sequential** is a workflow that runs all of its steps/commands in a predefined order/sequence. \
Every step can be configured to stop the workflow on failure(default behaviour) or let it process the rest of the
steps. \
It has the ability to retry at the step level, with a configured number of attempts and delay.

<strong>Installation:</strong>

```
go get github.com/silviutanasa/workflow
```

Note that the minimum supported version is Go v1.20.

<strong>Performance</strong>:

```
happy flow(no errors on steps):          0 allocs/op
error flow(errors on steps and retries): 3 allocs/op
```

<strong>Usage:</strong>\
example 1: A set of steps sharing the same request as common state

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
	wfConfig := []workflow.StepConfig{
		{Step: &step1},
		{Step: &step2},
		{Step: &step3},
	}
	wf := workflow.NewSequential("ETL", wfConfig, nil)
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

func (e *extractData) Execute(ctx context.Context, req any) error {
	r := req.(*request)
	inp, err := io.ReadAll(e.dataSource)
	if err != nil {
		return err
	}
	r.inputData = inp

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

func (t *transformData) Execute(ctx context.Context, req any) error {
	r := req.(*request)
	r.inputData = bytes.ToUpper(r.inputData)

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

func (s *sendData) Execute(ctx context.Context, req any) error {
	r := req.(*request)
	fmt.Printf("%s", r.inputData)

	return nil
}

```

example 2: A set of steps with no shared state, that reacts to an external event (chain of responsibility). All the
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
	wfConfig := []workflow.StepConfig{
		{Step: &step1, ContinueWorkflowOnError: true},
		{Step: &step2, ContinueWorkflowOnError: true},
		{Step: &step3, ContinueWorkflowOnError: true},
		{Step: &step4, ContinueWorkflowOnError: true},
	}
	wf := workflow.NewSequential("ETL", wfConfig, nil)
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

func (e *notifySalesDepartment) Execute(ctx context.Context, req any) error {
	ev := req.(event)
	if ev.name != "client created" {
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

func (t *notifyManagementDepartment) Execute(ctx context.Context, req any) error {
	ev := req.(event)
	if ev.name != "contract cancelled" {
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

func (s *notifyPagerDuty) Execute(ctx context.Context, req any) error {
	ev := req.(event)
	if ev.name != "critical error" {
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

func (n *notifyOnboardingDepartment) Execute(ctx context.Context, req any) error {
	ev := req.(event)
	if ev.name != "client created" {
		return nil
	}
	fmt.Println("Notifying onboarding department")

	return nil
}

```