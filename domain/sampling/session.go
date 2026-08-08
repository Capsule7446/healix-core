// 包采样模拟一个交互式浏览器采样会话。
package sampling

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"

	"github.com/Capsule7446/healix-core/domain/weburl"
)

// Status 表示采样会话的生命周期状态。
type Status string

const (
	// StatusCreated 表示会话已创建但尚未开始记录。
	StatusCreated Status = "created"
	// StatusRecording 表示会话正在接受捕获。
	StatusRecording Status = "recording"
	// StatusPaused 表示会话暂停接受捕获但仍可恢复。
	StatusPaused Status = "paused"
	// StatusEnded 表示会话已正常结束且不可恢复。
	StatusEnded Status = "ended"
	// StatusInterrupted 表示会话被中断且不可恢复。
	StatusInterrupted Status = "interrupted"
)

// ActionKind 表示采样捕获对应的操作类型。
type ActionKind string

const (
	// ActionNavigate 表示导航操作。
	ActionNavigate ActionKind = "navigate"
	// ActionClick 表示点击操作。
	ActionClick ActionKind = "click"
	// ActionInput 表示文本输入操作。
	ActionInput ActionKind = "input"
	// ActionSelect 表示选项选择操作。
	ActionSelect ActionKind = "select"
	// ActionPress 表示按键操作。
	ActionPress ActionKind = "press"
	// ActionValidate 表示验证捕获，不描述页面端交互。
	ActionValidate ActionKind = "validate"
)

// ValidationSample 保存构建验证步骤所需的框架无关语义值。
type ValidationSample struct {
	Kind           string
	Expected       string
	Actual         string
	Attribute      string
	SupportedKinds []string
	Sensitive      bool
}

// Capture 表示采样器提交的一次操作或验证捕获。
type Capture struct {
	CaptureID   string
	NodeUUID    string
	IdentityKey string
	PageURL     string
	Kind        ActionKind
	Value       string
	Values      []string
	Hints       ActionHints
	Spec        fingerprint.ElementTargetSpec
	Validation  *ValidationSample
}

// ActionHints 保存捕获操作的可选标记和意图。
type ActionHints struct {
	Optional bool
	Intent   string
}

// CapturedNode 保存采样期间发现的元素目标及其临时 UUID。
type CapturedNode struct {
	UUID string
	Spec fingerprint.ElementTargetSpec
}

// RecordedAction 保存会话中的有序操作记录及其目标引用。
type RecordedAction struct {
	UUID            string
	Sequence        int
	Kind            ActionKind
	Value           string
	Values          []string
	Hints           ActionHints
	PageURL         string
	NodeUUID        string
	ElementTargetID string
	Validation      *ValidationSample
}

// CaptureResult 返回捕获的幂等结果及创建、顺序和目标身份。
type CaptureResult struct {
	SessionID       string
	CaptureID       string
	NodeUUID        string
	ElementTargetID string
	ActionUUID      string
	Sequence        int
	Created         bool
}

// Snapshot 提供会话的只读深拷贝快照。
type Snapshot struct {
	ID             string
	FlowFragmentID string
	StartURL       string
	Status         Status
	Nodes          []CapturedNode
	Actions        []RecordedAction
}

// Session 管理采样会话状态、捕获节点、操作记录和幂等索引。
type Session struct {
	id          string
	workflowID  string
	startURL    string
	status      Status
	nodes       []CapturedNode
	actions     []RecordedAction
	byIdentity  map[string]int
	byUUID      map[string]int
	byCaptureID map[string]CaptureResult
}

// NewSession 校验工作流与起始 URL，创建处于 StatusCreated 的采样会话。
func NewSession(workflowID, startURL string) (*Session, error) {
	var violations []fault.Violation
	if strings.TrimSpace(workflowID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "flowFragmentId", "flow fragment id is required"))
	}
	if strings.TrimSpace(startURL) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "startUrl", "start url is required"))
		return nil, sessionInputInvalidError(violations)
	}
	if rejection := weburl.Check(startURL); rejection != weburl.Accepted {
		code, message := startURLViolation(rejection)
		violations = append(violations, mustViolation(code, "startUrl", message))
		return nil, wrapSessionInputInvalidError(fmt.Errorf("start url rejected: %s", rejection), violations)
	}
	if len(violations) != 0 {
		return nil, sessionInputInvalidError(violations)
	}
	sessionID, err := NewUUID()
	if err != nil {
		return nil, err
	}
	return &Session{
		id:          sessionID,
		workflowID:  workflowID,
		startURL:    startURL,
		status:      StatusCreated,
		byIdentity:  make(map[string]int),
		byUUID:      make(map[string]int),
		byCaptureID: make(map[string]CaptureResult),
	}, nil
}

// ID 返回会话 ID；nil 接收者返回空字符串。
func (s *Session) ID() string {
	if s == nil {
		return ""
	}
	return s.id
}

// Start 将新会话切换到记录状态，并追加初始导航操作。
func (s *Session) Start() error {
	if s == nil {
		return internalError()
	}
	if s.status != StatusCreated {
		return sessionStateInvalidError()
	}
	actionID, err := NewUUID()
	if err != nil {
		return err
	}
	s.status = StatusRecording
	s.actions = append(s.actions, RecordedAction{
		UUID: actionID, Sequence: 1, Kind: ActionNavigate, Value: s.startURL, PageURL: s.startURL,
	})
	return nil
}

// shapeViolations 按固定字段顺序聚合一次捕获的所有形状错误。
func (c Capture) shapeViolations() []fault.Violation {
	var violations []fault.Violation
	if strings.TrimSpace(c.CaptureID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "captureId", "capture id is required"))
	}
	switch c.Kind {
	case ActionClick, ActionInput, ActionSelect, ActionValidate:
		if strings.TrimSpace(c.IdentityKey) == "" {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, "identityKey", "capture identity key is required"))
		}
		if c.Kind == ActionValidate && c.Validation == nil {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, "validation", "validate capture requires validation detail"))
		}
	case ActionPress:
		if strings.TrimSpace(c.Value) == "" {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, "value", "press capture requires a value"))
		}
	default:
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "kind", "capture action kind is not supported"))
	}
	return violations
}

// Record 校验并幂等记录一次捕获，必要时创建或更新临时元素节点。
func (s *Session) Record(c Capture) (CaptureResult, error) {
	if s == nil {
		return CaptureResult{}, internalError()
	}
	if s.status != StatusRecording {
		return CaptureResult{}, sessionStateInvalidError()
	}
	if strings.TrimSpace(c.CaptureID) != "" {
		if previous, ok := s.byCaptureID[c.CaptureID]; ok {
			return previous, nil
		}
	}
	if violations := c.shapeViolations(); len(violations) != 0 {
		return CaptureResult{}, captureInvalidError(violations)
	}
	switch c.Kind {
	case ActionClick, ActionInput, ActionSelect, ActionValidate:
	case ActionPress:
		actionUUID, err := NewUUID()
		if err != nil {
			return CaptureResult{}, err
		}
		action := RecordedAction{
			UUID: actionUUID, Sequence: len(s.actions) + 1, Kind: c.Kind, Value: c.Value, Hints: c.Hints, PageURL: c.PageURL,
		}
		s.actions = append(s.actions, action)
		result := CaptureResult{
			SessionID: s.id, CaptureID: c.CaptureID, ActionUUID: action.UUID, Sequence: action.Sequence,
		}
		s.byCaptureID[c.CaptureID] = result
		return result, nil
	}
	c.Spec.PageURL = c.PageURL
	c.Spec.Origin = originOf(c.PageURL)

	nodeIndex, exists := s.byUUID[c.NodeUUID]
	if !exists {
		nodeIndex, exists = s.byIdentity[c.IdentityKey]
	}
	created := !exists
	if created {
		nodeUUID, err := NewUUID()
		if err != nil {
			return CaptureResult{}, err
		}
		c.Spec.UUID = nodeUUID
		c.Spec.ID = "node-" + compactUUID(nodeUUID)[:12]
		if err := c.Spec.Validate(); err != nil {
			return CaptureResult{}, err
		}
		nodeIndex = len(s.nodes)
		s.byIdentity[c.IdentityKey] = nodeIndex
		s.byUUID[nodeUUID] = nodeIndex
		s.nodes = append(s.nodes, CapturedNode{UUID: nodeUUID, Spec: cloneSpec(c.Spec)})
	} else {
		current := s.nodes[nodeIndex]
		s.byIdentity[c.IdentityKey] = nodeIndex
		c.Spec.UUID = current.UUID
		c.Spec.ID = current.Spec.ID
		if err := c.Spec.Validate(); err != nil {
			return CaptureResult{}, err
		}
		s.nodes[nodeIndex].Spec = cloneSpec(c.Spec)
	}

	actionUUID, err := NewUUID()
	if err != nil {
		return CaptureResult{}, err
	}
	node := s.nodes[nodeIndex]
	action := RecordedAction{
		UUID: actionUUID, Sequence: len(s.actions) + 1, Kind: c.Kind, Value: c.Value, Values: append([]string(nil), c.Values...), Hints: c.Hints,
		PageURL: c.PageURL, NodeUUID: node.UUID, ElementTargetID: node.Spec.ID, Validation: cloneValidation(c.Validation),
	}
	s.actions = append(s.actions, action)
	result := CaptureResult{
		SessionID: s.id, CaptureID: c.CaptureID, NodeUUID: node.UUID, ElementTargetID: node.Spec.ID,
		ActionUUID: action.UUID, Sequence: action.Sequence, Created: created,
	}
	s.byCaptureID[c.CaptureID] = result
	return result, nil
}

// Pause 将记录中的会话切换为暂停状态，并保留节点身份映射。
func (s *Session) Pause() error {
	if s == nil {
		return internalError()
	}
	if s.status != StatusRecording {
		return sessionStateInvalidError()
	}
	s.status = StatusPaused
	return nil
}

// Resume 将暂停的会话切换回记录状态。
func (s *Session) Resume() error {
	if s == nil {
		return internalError()
	}
	if s.status != StatusPaused {
		return sessionStateInvalidError()
	}
	s.status = StatusRecording
	return nil
}

// End 将记录或暂停的会话切换到不可恢复的结束状态。
func (s *Session) End() error {
	if s == nil {
		return internalError()
	}
	if s.status != StatusRecording && s.status != StatusPaused {
		return sessionStateInvalidError()
	}
	s.status = StatusEnded
	return nil
}

// Interrupt 将尚未结束的会话标记为中断状态。
func (s *Session) Interrupt() {
	if s != nil && (s.status == StatusCreated || s.status == StatusRecording || s.status == StatusPaused) {
		s.status = StatusInterrupted
	}
}

// Snapshot 返回包含节点、操作和验证数据深拷贝的会话快照；nil 接收者返回空快照。
func (s *Session) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	nodes := make([]CapturedNode, len(s.nodes))
	for i := range s.nodes {
		nodes[i] = CapturedNode{UUID: s.nodes[i].UUID, Spec: cloneSpec(s.nodes[i].Spec)}
	}
	actions := append([]RecordedAction(nil), s.actions...)
	for i := range actions {
		actions[i].Values = append([]string(nil), actions[i].Values...)
		actions[i].Validation = cloneValidation(actions[i].Validation)
	}
	return Snapshot{
		ID: s.id, FlowFragmentID: s.workflowID, StartURL: s.startURL,
		Status: s.status, Nodes: nodes, Actions: actions,
	}
}

// cloneValidation 深拷贝验证样本及其支持类型；nil 输入保持 nil。
func cloneValidation(input *ValidationSample) *ValidationSample {
	if input == nil {
		return nil
	}
	copy := *input
	copy.SupportedKinds = append([]string(nil), input.SupportedKinds...)
	return &copy
}

// NewUUID 生成包含时间片段和随机字节的 UUID；熵源失败返回内部错误。
func NewUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", wrapInternalError(err)
	}
	timestamp := uint64(time.Now().UnixMilli())
	value[0] = byte(timestamp >> 40)
	value[1] = byte(timestamp >> 32)
	value[2] = byte(timestamp >> 24)
	value[3] = byte(timestamp >> 16)
	value[4] = byte(timestamp >> 8)
	value[5] = byte(timestamp)
	value[6] = (value[6] & 0x0f) | 0x70
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

// compactUUID 删除 UUID 中的连字符。
func compactUUID(value string) string { return strings.ReplaceAll(value, "-", "") }

// originOf 从 URL 提取 scheme://host；解析失败或缺少任一部分时返回空字符串。
func originOf(value string) string {
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// cloneSpec 深拷贝元素目标规格及其选择器和指纹。
func cloneSpec(spec fingerprint.ElementTargetSpec) fingerprint.ElementTargetSpec {
	copied := spec
	copied.Selectors = append([]fingerprint.Selector(nil), spec.Selectors...)
	copied.Fingerprint = spec.Fingerprint.Clone()
	return copied
}

// startURLViolation 将 URL 拒绝原因映射为不回显 URL 的字段错误码和消息。
func startURLViolation(rejection weburl.Rejection) (fault.Code, string) {
	switch rejection {
	case weburl.RejectScheme:
		return fault.CodeFieldInvalid, "start url scheme must be http or https"
	case weburl.RejectHostMissing:
		return fault.CodeFieldRequired, "start url host is required"
	case weburl.RejectUserinfo:
		return fault.CodeFieldInvalid, "start url cannot contain credentials"
	case weburl.RejectControlChars:
		return fault.CodeFieldInvalid, "start url cannot contain control characters"
	default:
		return fault.CodeFieldInvalid, "start url is not a valid url"
	}
}
