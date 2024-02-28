package workflow

import (
	"bytes"
	"sync"
	"unsafe"
)

const (
	succeed = "\u2713"
	failed  = "\u2717"

	// used for logging
	infoLevel  = 1
	errorLevel = 2
)

// bufPool is used by the internal logging system, to compute the string messages.
// the reason fo this choice is to reduce allocations.
var bufPool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return new(bytes.Buffer)
	},
}

// RetryDecider signals if an operation is retryable.
type RetryDecider interface {
	CanRetry() bool
}

// Logger is the workflow supported logger.
type Logger interface {
	Info(msg string)
	Error(msg string)
}

// noOpLogger is the internal, default logger, and is a no op.
// It exists only to allow the user to disable logging, by providing a nil logger to the Sequential constructor.
type noOpLogger struct{}

// Info is the Info level log.
func (n noOpLogger) Info(_ string) {}

// Error is the Error level log.
func (n noOpLogger) Error(_ string) {}

// concatStr produces a 0 allocation string concatenation, by taking the best parts from both bytes.Buffer and strings.Builder.
// The resulting string must be consumed ASAP, otherwise the content is not guaranteed to stay the same.
func concatStr(in ...string) string {
	b := bufPool.Get().(*bytes.Buffer)
	b.Reset()
	defer bufPool.Put(b)

	for _, v := range in {
		b.WriteString(v)
	}
	// keep in mind that, as the package name suggests, this approach is not safe, and the string should be
	// "consumed" ASAP after this return, otherwise the content is not guaranteed.
	// it works well in the non-concurrent pre logging string composition, as we send the content to the writer, right
	// after this return.
	return unsafe.String(unsafe.SliceData(b.Bytes()), len(b.Bytes()))
}
