package automation

import (
	"reflect"

	"github.com/Capsule7446/healix-core/domain/fault"
)

// CodeAutomationConfigurationInvalid 表示自动化服务依赖未配置。
const CodeAutomationConfigurationInvalid fault.Code = "AUTOMATION_CONFIGURATION_INVALID"

// AutomationConfigurationError 构造自动化服务配置缺失的前置条件错误。
func AutomationConfigurationError() error {
	err, constructionErr := fault.New(
		fault.FailedPrecondition,
		CodeAutomationConfigurationInvalid,
		"automation service is not configured",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// CodeHealCandidateStaleBase 表示自愈候选基线已不是当前版本。
const CodeHealCandidateStaleBase fault.Code = "AUTOMATION_HEAL_CANDIDATE_STALE_BASE"

// HealCandidateStaleBaseError 构造自愈候选基线过期的冲突错误。
func HealCandidateStaleBaseError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeHealCandidateStaleBase,
		"heal candidate base version is no longer current",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// isNilDependency 判断接口值或其可为空底层值是否为 nil。
func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// CodeAutomationRevisionConflict 表示自动化聚合修订与当前状态冲突。
const CodeAutomationRevisionConflict fault.Code = "AUTOMATION_REVISION_CONFLICT"

// AutomationRevisionConflictError 构造自动化聚合修订冲突错误。
func AutomationRevisionConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeAutomationRevisionConflict,
		"automation revision conflicts with current state",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
