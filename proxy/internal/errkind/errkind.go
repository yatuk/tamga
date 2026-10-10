// Package errkind names what kind of error an error is, for a log line or a
// span that must not hold the error's own text.
//
// A scanner, a decoder or a remote service can quote what it was given in the
// error it returns, and what it was given is a request's text. The kind is
// one of a small fixed set of words, or a Go type name; it is never taken
// from the message.
package errkind

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The kinds that are not a gRPC code or a type name.
const (
	Timeout   = "timeout"
	Cancelled = "cancelled"
	Panic     = "panic"
)

// New returns an error with the message msg whose kind is kind, for a
// sentinel that a log line should name: "circuit_open" says more than the
// type of errors.New.
func New(kind, msg string) error { return &named{kind: kind, msg: msg} }

type named struct{ kind, msg string }

func (e *named) Error() string     { return e.msg }
func (e *named) ErrorKind() string { return e.kind }

// maxLen bounds a kind. Type names are short; one that is not is cut.
const maxLen = 64

// Kinder is implemented by an error that names its own kind. The name must
// be a constant, not something built from what the error was given.
type Kinder interface {
	ErrorKind() string
}

// Of returns the kind of err, and "" for nil. In order: the kind an error in
// the chain gives itself (Kinder), "timeout" or "cancelled" for a context
// that ended, "grpc_<code>" for a gRPC status, and otherwise the Go type of
// the innermost error, for example "*net.OpError".
func Of(err error) string {
	if err == nil {
		return ""
	}
	var k Kinder
	if errors.As(err, &k) {
		return clip(k.ErrorKind())
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return Timeout
	case errors.Is(err, context.Canceled):
		return Cancelled
	}
	if st, ok := status.FromError(err); ok && st.Code() != codes.OK {
		return "grpc_" + snake(st.Code().String())
	}
	return clip(fmt.Sprintf("%T", innermost(err)))
}

// innermost follows the chain to the error that wraps nothing. Where an
// error wraps several, the last is taken: the cause is written last.
func innermost(err error) error {
	for depth := 0; depth < 32; depth++ {
		switch w := err.(type) {
		case interface{ Unwrap() error }:
			next := w.Unwrap()
			if next == nil {
				return err
			}
			err = next
		case interface{ Unwrap() []error }:
			var next error
			for _, e := range w.Unwrap() {
				if e != nil {
					next = e
				}
			}
			if next == nil {
				return err
			}
			err = next
		default:
			return err
		}
	}
	return err
}

// snake turns a gRPC code's name into lower case with underscores:
// "DeadlineExceeded" becomes "deadline_exceeded".
func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func clip(s string) string {
	if len(s) > maxLen {
		s = strings.ToValidUTF8(s[:maxLen], "")
	}
	return s
}
