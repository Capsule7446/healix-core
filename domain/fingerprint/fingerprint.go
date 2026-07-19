// 包指纹定义了执行、采样和确定性修复所使用的选择器和元素标识值对象。
package fingerprint

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// SelectorType 是一种定位策略，按 Playwright 推荐的稳定性阶梯排序
// （role > testid > css > xpath/text）。
type SelectorType string

const (
	SelectorRole   SelectorType = "role"
	SelectorTestID SelectorType = "testid"
	SelectorCSS    SelectorType = "css"
	SelectorXPath  SelectorType = "xpath"
	SelectorText   SelectorType = "text"
)

// Selector 是 NodeSpec 的一个候选定位策略，按 Priority 升序依次尝试，
// 直到其中一个成功解析为止。
type Selector struct {
	Type     SelectorType
	Value    string
	Priority int
}

// Validate 验证拒绝任何 Driver 适配器无法解析的选择器。
func (s Selector) Validate() error {
	var problems []string
	switch s.Type {
	case SelectorRole, SelectorTestID, SelectorCSS, SelectorXPath, SelectorText:
	default:
		problems = append(problems, fmt.Sprintf("unsupported type %q", s.Type))
	}
	if strings.TrimSpace(s.Value) == "" {
		problems = append(problems, "value is required")
	}
	if s.Priority < 0 {
		problems = append(problems, "priority must be >= 0")
	}
	if len(problems) != 0 {
		return errors.New(strings.Join(problems, "; "))
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
	FormID string
}

// NodeSpec 是为某个 step 撰写/采样得到的一个可定位元素：
// 优先尝试选择器，只有全部选择器失败后才会用到指纹。
type NodeSpec struct {
	UUID        string
	ID          string
	PageURL     string
	Origin      string
	Role        string
	Selectors   []Selector
	Fingerprint Fingerprint
}

// Validate 验证保护所有配置源和 Driver 实现共享的最小身份和定位器不变量。
func (s NodeSpec) Validate() error {
	var problems []string
	if s.UUID != "" && !validUUID(s.UUID) {
		problems = append(problems, "uuid must be a canonical UUID")
	}
	if strings.TrimSpace(s.ID) == "" {
		problems = append(problems, "id is required")
	}
	if len(s.Selectors) == 0 {
		problems = append(problems, "selectors must contain at least 1 item")
	}
	for i, selector := range s.Selectors {
		if err := selector.Validate(); err != nil {
			problems = append(problems, fmt.Sprintf("selectors[%d]: %v", i, err))
		}
	}
	if strings.TrimSpace(s.Fingerprint.Tag) == "" {
		problems = append(problems, "fingerprint.tag is required")
	}
	if s.Fingerprint.Attributes == nil {
		problems = append(problems, "fingerprint.attributes is required")
	}
	if s.Fingerprint.SiblingIndex < 0 {
		problems = append(problems, "fingerprint.sibling_index must be >= 0")
	}
	if len(problems) != 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	_, err := hex.DecodeString(compact)
	return err == nil
}
