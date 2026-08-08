// 包指纹定义了执行、采样和确定性修复所使用的选择器和元素标识值对象。
package fingerprint

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
)

// SelectorType 是一种定位策略，按 Playwright 推荐的稳定性阶梯排序
// （role > testid > css > xpath/text）。
type SelectorType string

const (
	// SelectorRole 使用 ARIA role 作为定位策略。
	SelectorRole SelectorType = "role"
	// SelectorTestID 使用 data-testid 作为定位策略。
	SelectorTestID SelectorType = "testid"
	// SelectorCSS 使用 CSS 选择器作为定位策略。
	SelectorCSS SelectorType = "css"
	// SelectorXPath 使用 XPath 作为定位策略。
	SelectorXPath SelectorType = "xpath"
	// SelectorText 使用文本匹配作为定位策略。
	SelectorText SelectorType = "text"
)

// Selector 是 ElementTargetSpec 的一个候选定位策略，按 Priority 升序依次尝试，
// 直到其中一个成功解析为止。
type Selector struct {
	Type     SelectorType
	Value    string
	Priority int
}

// isSupported 判断选择器类型是否属于共享支持集合。
func (t SelectorType) isSupported() bool {
	switch t {
	case SelectorRole, SelectorTestID, SelectorCSS, SelectorXPath, SelectorText:
		return true
	default:
		return false
	}
}

// Validate 验证拒绝任何 Driver 适配器无法解析的选择器。
// 选择器值是用户输入，任何失败细节都不得进入公共文本。
func (s Selector) Validate() error {
	if !s.Type.isSupported() || strings.TrimSpace(s.Value) == "" || s.Priority < 0 {
		return selectorInvalidError()
	}
	return nil
}

// ARIA 记录无障碍访问的 role/name 组合——
// 这是与 data-testid 并列的两个权重最高、最稳定的自愈信号之一。
type ARIA struct {
	Role string
	Name string
}

// Neighbors 记录紧邻的 DOM 兄弟节点/父标签，供 w_neighbor
// 自愈打分维度使用。
type Neighbors struct {
	Prev      string
	Next      string
	ParentTag string
}

// Fingerprint 是为某个元素记录的完整特征集合，使其在选择器全部失效后，
// 仍可通过纯算法打分重新定位。不记录视口边界框（bbox）——
// 页面布局、视口尺寸、缩放比例在不同录制/回放之间无法保证一致，位置信号
// 并不稳定，纳入进来只会引入噪音。
type Fingerprint struct {
	Tag          string
	Attributes   map[string]string
	Text         string
	ARIA         ARIA
	Path         []string // 祖先标签链，例如 html/body/div#app/form#loginForm/button
	SiblingIndex int
	Neighbors    Neighbors
	// LabelText 是与该元素关联的表单标签文本：优先取 <label for>/
	// aria-labelledby 指向的元素，其次取包裹该元素的 <label> 自身文本。
	// 表单场景下这是比布局位置稳定得多的身份信号。
	LabelText string
	// FormID 是最近的祖先 <form> 的 id（没有 id 则退化为 name），
	// 不在任何表单内则为空。用于区分同一页面里长得相似的多个表单。
	FormID    string
	Framework FrameworkStack
}

// Clone 返回 Fingerprint 的深拷贝；结果独占 Attributes、Path 和 Framework 的引用数据，
// ARIA 与 Neighbors 这类全字符串结构体按值复制。
func (f Fingerprint) Clone() Fingerprint {
	cloned := f
	cloned.Path = append([]string(nil), f.Path...)
	cloned.Framework = f.Framework.Clone()
	if f.Attributes != nil {
		cloned.Attributes = make(map[string]string, len(f.Attributes))
		for key, value := range f.Attributes {
			cloned.Attributes[key] = value
		}
	}
	return cloned
}

// Validate 校验匹配和自愈共用的指纹身份不变量，并以描述符自身错误码返回违规项。
// 描述符可能由 domain/heal 直接校验，不一定经过元素目标规格。
func (f Fingerprint) Validate() error {
	violations := f.appendViolations(nil, "")
	if len(violations) != 0 {
		return descriptorInvalidError(violations)
	}
	return nil
}

// appendViolations 将框架校验失败降级为承载该描述符的聚合违规，避免嵌套 fault。
func (f Fingerprint) appendViolations(violations []fault.Violation, prefix string) []fault.Violation {
	if strings.TrimSpace(f.Tag) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, joinField(prefix, "tag"), "fingerprint tag is required"))
	}
	if f.Attributes == nil {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, joinField(prefix, "attributes"), "fingerprint attributes are required"))
	}
	if f.SiblingIndex < 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "siblingIndex"), "fingerprint sibling index must not be negative"))
	}
	return f.Framework.appendViolations(violations, joinField(prefix, "framework"))
}

// ElementTargetSpec 描述元素的身份、定位器和备用指纹；优先尝试选择器，全部失败后才使用指纹。
type ElementTargetSpec struct {
	UUID        string
	ID          string
	PageURL     string
	Origin      string
	Role        string
	Selectors   []Selector
	Fingerprint Fingerprint
}

// Validate 验证保护所有配置源和 Driver 实现共享的最小身份和定位器不变量。
// 子校验失败降级为本聚合的有序 Violation，不产出嵌套 fault、不拼接文本。
func (s ElementTargetSpec) Validate() error {
	var violations []fault.Violation
	if s.UUID != "" && !validUUID(s.UUID) {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "uuid", "uuid must be a canonical UUID"))
	}
	if strings.TrimSpace(s.ID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "id", "id is required"))
	}
	if len(s.Selectors) == 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "selectors", "selectors must contain at least one entry"))
	}
	for index, selector := range s.Selectors {
		if selector.Validate() != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, fmt.Sprintf("selectors.%d", index), "element selector is invalid"))
		}
	}
	violations = s.Fingerprint.appendViolations(violations, "fingerprint")
	if len(violations) != 0 {
		return elementTargetSpecInvalidError(violations)
	}
	return nil
}

// validUUID 判断字符串是否为 36 字符、带标准连字符布局且可解码的 UUID。
func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	_, err := hex.DecodeString(compact)
	return err == nil
}
