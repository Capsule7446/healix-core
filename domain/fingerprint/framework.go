package fingerprint

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
)

// FrameworkKind 标识可检测到的前端框架种类。
type FrameworkKind string

const (
	// FrameworkReact 表示 React 框架。
	FrameworkReact FrameworkKind = "react"
	// FrameworkVue 表示 Vue 框架。
	FrameworkVue FrameworkKind = "vue"
	// FrameworkAngular 表示 Angular 框架。
	FrameworkAngular FrameworkKind = "angular"
	// FrameworkSvelte 表示 Svelte 框架。
	FrameworkSvelte FrameworkKind = "svelte"
	// FrameworkSolid 表示 Solid 框架。
	FrameworkSolid FrameworkKind = "solid"
	// FrameworkPreact 表示 Preact 框架。
	FrameworkPreact FrameworkKind = "preact"
	// FrameworkUnknown 表示检测到框架但无法归类为已知种类。
	FrameworkUnknown FrameworkKind = "unknown"
)

// FrameworkEvidenceKind 标识框架识别证据的来源。
type FrameworkEvidenceKind string

const (
	// EvidenceScriptLink 表示证据来自脚本链接。
	EvidenceScriptLink FrameworkEvidenceKind = "script_link"
	// EvidenceGlobal 表示证据来自全局标记。
	EvidenceGlobal FrameworkEvidenceKind = "global_marker"
	// EvidenceRootMarker 表示证据来自根节点标记。
	EvidenceRootMarker FrameworkEvidenceKind = "root_marker"
	// EvidenceHydration 表示证据来自 hydration 标记。
	EvidenceHydration FrameworkEvidenceKind = "hydration_marker"
)

// FrameworkInfo 保存框架种类、版本、置信度和识别证据。
type FrameworkInfo struct {
	Kind       FrameworkKind
	Version    string
	Confidence float64
	Evidence   FrameworkEvidenceKind
}

// isSupported 判断框架种类是否属于支持集合。
func (k FrameworkKind) isSupported() bool {
	switch k {
	case FrameworkReact, FrameworkVue, FrameworkAngular, FrameworkSvelte, FrameworkSolid, FrameworkPreact, FrameworkUnknown:
		return true
	default:
		return false
	}
}

// isSupported 判断证据来源是否为空或属于支持集合。
func (e FrameworkEvidenceKind) isSupported() bool {
	switch e {
	case "", EvidenceScriptLink, EvidenceGlobal, EvidenceRootMarker, EvidenceHydration:
		return true
	default:
		return false
	}
}

// Validate 校验框架信息的种类、置信度、版本格式和证据来源。
func (f FrameworkInfo) Validate() error {
	violations := f.appendViolations(nil, "")
	if len(violations) != 0 {
		return frameworkStackInvalidError(violations)
	}
	return nil
}

// appendViolations 将每个框架校验失败降级为承载该信息的聚合违规，避免框架 fault 嵌套
// 在另一个 fault 中。种类、证据类型和版本不会进入公开文本；不支持的值既然不属于
// 封闭集合，就可能是任意调用方输入，回显会造成泄露。
func (f FrameworkInfo) appendViolations(violations []fault.Violation, prefix string) []fault.Violation {
	if !f.Kind.isSupported() {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "kind"), "framework kind is not supported"))
	}
	if math.IsNaN(f.Confidence) || math.IsInf(f.Confidence, 0) || f.Confidence < 0 || f.Confidence > 1 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "confidence"), "framework confidence must be within the inclusive range from zero through one"))
	}
	if strings.TrimSpace(f.Version) != "" && strings.ContainsAny(f.Version, "\r\n") {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "version"), "framework version must not contain a line break"))
	}
	if !f.Evidence.isSupported() {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "evidence"), "framework evidence kind is not supported"))
	}
	return violations
}

// FrameworkStack 是按检测结果排列的框架信息切片。
type FrameworkStack []FrameworkInfo

// Validate 校验栈中每个框架信息及框架种类的唯一性。
func (s FrameworkStack) Validate() error {
	violations := s.appendViolations(nil, "frameworks")
	if len(violations) != 0 {
		return frameworkStackInvalidError(violations)
	}
	return nil
}

// appendViolations 按切片顺序遍历栈，确保违规顺序确定；重复集合只用于单点查询。
// prefix 必须是非空逻辑路径，因为带索引的元素路径不能以数字开头。
func (s FrameworkStack) appendViolations(violations []fault.Violation, prefix string) []fault.Violation {
	seen := make(map[FrameworkKind]struct{}, len(s))
	for index, info := range s {
		element := fmt.Sprintf("%s.%d", prefix, index)
		violations = info.appendViolations(violations, element)
		if _, duplicate := seen[info.Kind]; duplicate {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, joinField(element, "kind"), "framework kind is duplicated"))
		}
		seen[info.Kind] = struct{}{}
	}
	return violations
}

// Clone 复制框架栈切片；nil 输入保持 nil，结果切片与源切片互不共享。
func (s FrameworkStack) Clone() FrameworkStack { return append(FrameworkStack(nil), s...) }

// SortFrameworkStack 返回按置信度降序、种类和版本升序稳定排列的副本，不修改输入栈。
func SortFrameworkStack(stack FrameworkStack) FrameworkStack {
	out := stack.Clone()
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Version < out[j].Version
	})
	return out
}
