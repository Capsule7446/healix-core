package node

import (
	"github.com/Capsule7446/healix-core/domain/fault"
)

// isExclusiveElementNotFound 判断错误链是否只包含元素未找到错误；nil 或空聚合返回 false。
func isExclusiveElementNotFound(err error) bool {
	if err == nil {
		return false
	}
	if nodeFault, ok := err.(*fault.Error); ok {
		return nodeFault.Code() == CodeElementNotFound
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
	return fault.IsCode(err, CodeElementNotFound)
}

// isExclusiveTransientDriverFault 判断错误链是否只包含可重试的临时驱动错误。
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

// RetryPolicy 配置一次操作允许的最大尝试次数。
type RetryPolicy struct {
	Attempts int
}

// normalized 将小于 1 的尝试次数归一化为一次。
func (p RetryPolicy) normalized() int {
	if p.Attempts < 1 {
		return 1
	}
	return p.Attempts
}

// Retry 按策略运行操作，并在临时驱动错误时重试。
func Retry(policy RetryPolicy, operation func() error) error {
	_, err := RetryWithAttempts(policy, operation)
	return err
}

// RetryWithAttempts 执行带重试的操作，并返回实际尝试次数及最终错误。
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
