package workflow

import "context"

// Command represents a step of execution(data processor).
type Command interface {
	// Name returns the name of the command.
	Name() string
	// Execute is the command's central processing unit.
	// It accepts a context and a request which must be passed by reference.
	// The purpose of the request is to pass data to the chain and also hold chain state across the commands,
	// meaning that any command can also store data into the request and it will be available to the other cmds from the chain.
	Execute(ctx context.Context, request interface{}) error
	// CanRetryOnError decides if the command can retry to execute(in cases of failure for example).
	CanRetryOnError() bool
	// ContinueWorkflowOnError decides if the command can stop the propagation of the request to other commands that ran in a chain
	// in case it returns an error.
	ContinueWorkflowOnError() bool
	// StopWorkflow decides if the command can stop the propagation to other commands that ran in a chain
	// after it completes its work.
	StopWorkflow() bool
}

// Logger is the workflow supported logger.
type Logger interface {
	Info(interface{})
	Warn(interface{})
	Error(interface{})
}

type noOpLogger struct{}

func (n noOpLogger) Info(_ interface{})  {}
func (n noOpLogger) Warn(_ interface{})  {}
func (n noOpLogger) Error(_ interface{}) {}
