// Copyright 2016 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package repl

import "fmt"

// Error is the error type returned by the REPL.
type Error struct {
	Code    ErrCode
	Message string
}

func (err *Error) Error() string {
	return fmt.Sprintf("code %v: %v", err.Code, err.Message)
}

// ErrCode represents the collection of errors that may be returned by the REPL.
type ErrCode int

const (
	// BadArgsErr indicates bad arguments were provided to a built-in REPL
	// command.
	BadArgsErr ErrCode = iota
)

func newBadArgsErr(f string, a ...interface{}) *Error {
	return &Error{
		Code:    BadArgsErr,
		Message: fmt.Sprintf(f, a...),
	}
}

// stop is returned by the 'exit' command to indicate to the REPL that it should
// break and return.
type stop struct{}

func (stop) Error() string {
	return "<stop>"
}
