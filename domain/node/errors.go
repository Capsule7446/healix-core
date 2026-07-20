package node

import (
	"context"
	"errors"
)

// ErrorKind classifies execution failures without coupling the domain to a browser driver.
type ErrorKind string

const (
	ErrorNotFound        ErrorKind = "not_found"
	ErrorNotVisible      ErrorKind = "not_visible"
	ErrorNotInteractable ErrorKind = "not_interactable"
	ErrorTimeout         ErrorKind = "timeout"
	ErrorNavigation      ErrorKind = "navigation"
	ErrorAssertion       ErrorKind = "assertion"
	ErrorContextClosed   ErrorKind = "context_closed"
	ErrorTransientDriver ErrorKind = "transient_driver"
	ErrorUnknown         ErrorKind = "unknown"
)

// ClassifiedError preserves the original driver error while exposing stable retry semantics.
type ClassifiedError struct {
	Kind      ErrorKind
	Operation string
	Err       error
}

func (e *ClassifiedError) Error() string {
	if e == nil {
		return "<nil>"
	}
	message := "unspecified error"
	if e.Err != nil {
		message = e.Err.Error()
	}
	if e.Operation == "" {
		return string(e.Kind) + ": " + message
	}
	return e.Operation + " (" + string(e.Kind) + "): " + message
}

func (e *ClassifiedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func TransientError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &ClassifiedError{Kind: ErrorTransientDriver, Operation: operation, Err: err}
}

func ClassifyError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*ClassifiedError); ok {
		return err
	}
	kind := ErrorUnknown
	switch {
	case errors.Is(err, context.Canceled):
		kind = ErrorContextClosed
	case errors.Is(err, context.DeadlineExceeded):
		kind = ErrorTimeout
	case errors.Is(err, ErrElementNotFound):
		kind = ErrorNotFound
	}
	return &ClassifiedError{Kind: kind, Operation: operation, Err: err}
}

func errorKind(err error) ErrorKind {
	if err == nil {
		return ""
	}
	var classified *ClassifiedError
	if errors.As(err, &classified) {
		return classified.Kind
	}
	classified = ClassifyError("operation", err).(*ClassifiedError)
	return classified.Kind
}

type RetryPolicy struct {
	Attempts int
}

func (p RetryPolicy) normalized() int {
	if p.Attempts < 1 {
		return 1
	}
	return p.Attempts
}

func Retry(policy RetryPolicy, operation func() error) error {
	_, err := RetryWithAttempts(policy, operation)
	return err
}

func RetryWithAttempts(policy RetryPolicy, operation func() error) (int, error) {
	var err error
	attempts := policy.normalized()
	for attempt := 1; attempt <= attempts; attempt++ {
		err = operation()
		if err == nil {
			return attempt, nil
		}
		var classified *ClassifiedError
		if !errors.As(err, &classified) || classified.Kind != ErrorTransientDriver {
			return attempt, err
		}
	}
	return attempts, err
}
