package fingerprint

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
)

// PageObservation 是框架检测使用的安全浏览器到领域投影。
// 它有意不包含原始 DOM、JS 对象、浏览器句柄或任意页面文本。
type PageObservation struct {
	PageURL       string
	ScriptURLs    []string
	GlobalMarkers []string
	RootMarkers   []string
	Hydration     []string
}

// FrameworkMatch 保存检测器识别出的框架信息。
type FrameworkMatch struct {
	Info FrameworkInfo
}

// FrameworkDetector 是宿主提供的框架检测端口。
type FrameworkDetector interface {
	// Detect 根据页面观测返回框架匹配；错误应使用 fault 领域错误分类。
	Detect(context.Context, PageObservation) ([]FrameworkMatch, error)
}

// DetectFrameworks 运行检测器，合并重复结果并返回已校验、顺序确定的框架栈。
func DetectFrameworks(ctx context.Context, observation PageObservation, detectors []FrameworkDetector) (FrameworkStack, error) {
	stack := make(FrameworkStack, 0)
	for index, detector := range detectors {
		if isNilDetector(detector) {
			return nil, frameworkStackInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldRequired, fmt.Sprintf("detectors.%d", index), "framework detector is required"),
			})
		}
		matches, err := detector.Detect(ctx, observation)
		if err != nil {
			// 已分类的检测器错误保持原分类；再次包装会嵌套两个 fault，迫使宿主在路由前
			// 额外解包。
			if _, classified := fault.CodeOf(err); classified {
				return nil, err
			}
			return nil, frameworkDetectorFailedError(err)
		}
		stack = append(stack, matchInfos(matches)...)
	}
	stack = mergeFrameworkStack(SortFrameworkStack(stack))
	// 此处的栈来自检测器输出而非调用方输入，因此形状失败表示端口违反契约，而不是
	// 调用方传入错误。报告面向调用方的栈错误码会错误地要求调用方修复它从未提供的数据。
	if err := stack.Validate(); err != nil {
		return nil, frameworkDetectorFailedError(err)
	}
	return stack, nil
}

// isNilDetector 识别直接为 nil 或承载 typed nil 的检测器端口。
func isNilDetector(detector FrameworkDetector) bool {
	if detector == nil {
		return true
	}
	value := reflect.ValueOf(detector)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// mergeFrameworkStack 按首次出现顺序去除相同框架种类的重复信息。
func mergeFrameworkStack(stack FrameworkStack) FrameworkStack {
	seen := make(map[FrameworkKind]struct{}, len(stack))
	out := make(FrameworkStack, 0, len(stack))
	for _, info := range stack {
		if _, ok := seen[info.Kind]; ok {
			continue
		}
		seen[info.Kind] = struct{}{}
		out = append(out, info)
	}
	return out
}

// matchInfos 将检测匹配映射为框架栈，并规范化版本字符串的首尾空白。
func matchInfos(matches []FrameworkMatch) FrameworkStack {
	stack := make(FrameworkStack, 0, len(matches))
	for _, match := range matches {
		match.Info.Version = strings.TrimSpace(match.Info.Version)
		stack = append(stack, match.Info)
	}
	return stack
}
