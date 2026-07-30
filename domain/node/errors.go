package node

import (
	"github.com/Capsule7446/healix-core/domain/fault"
)

func isExclusiveElementNotFound(err error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !isExclusiveElementNotFound(child) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return isExclusiveElementNotFound(wrapped.Unwrap())
	}
	return err == ErrElementNotFound
}

func isExclusiveTransientDriverFault(err error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !isExclusiveTransientDriverFault(child) {
				return false
			}
		}
		return true
	}
	if nodeFault, ok := err.(*fault.Error); ok {
		return nodeFault.Code() == CodeTransientDriver
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return isExclusiveTransientDriverFault(wrapped.Unwrap())
	}
	return fault.IsCode(err, CodeTransientDriver)
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
	attempts := policy.normalized()
	for attempt := 1; attempt <= attempts; attempt++ {
		err := operation()
		if err == nil {
			return attempt, nil
		}
		if !isExclusiveTransientDriverFault(err) {
			return attempt, err
		}
		if attempt == attempts {
			return attempt, err
		}
	}
	return attempts, nil
}
