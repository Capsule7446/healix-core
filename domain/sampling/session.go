// 包采样模拟一个交互式浏览器采样会话。
package sampling

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type Status string

const (
	StatusCreated   Status = "created"
	StatusRecording Status = "recording"
	// StatusPaused 使内存中的捕获会话保持活动状态。  恢复必须继续相同的操作/节点身份空间，而不是创建新的临时工作流。
	StatusPaused Status = "paused"
	StatusEnded  Status = "ended"
	// StatusInterrupted 表示受控浏览器意外消失。捕获的工件仍然可用，但会话无法恢复。
	StatusInterrupted Status = "interrupted"

	// StatusCompleted 和 StatusFailed 是终端生命周期状态的兼容性别名。新呼叫者应该更喜欢 StatusEnded 和 StatusInterrupted。
	StatusCompleted Status = StatusEnded
	StatusFailed    Status = StatusInterrupted
)

type ActionKind string

const (
	ActionNavigate ActionKind = "navigate"
	ActionClick    ActionKind = "click"
	ActionInput    ActionKind = "input"
	ActionSelect   ActionKind = "select"
	ActionPress    ActionKind = "press"
	// ActionValidate 仅由一次性验证选择器发出。  它像普通捕获一样创建节点标识，但故意不描述页面端交互。
	ActionValidate ActionKind = "validate"
)

// ValidationSample 是浏览器采样器发出的框架中立的语义建议。  它仅包含构建验证步骤所需的值； DOM/框架实现细节保留在采样器适配器中。  工作区将其转换为其持久验证值对象。
type ValidationSample struct {
	Kind           string
	Expected       string
	Actual         string
	Attribute      string
	SupportedKinds []string
	Sensitive      bool
}

type Capture struct {
	CaptureID   string
	NodeUUID    string
	IdentityKey string
	PageURL     string
	Kind        ActionKind
	Value       string
	Values      []string
	Hints       ActionHints
	Spec        fingerprint.NodeSpec
	Validation  *ValidationSample
}

type ActionHints struct {
	Optional bool
	Intent   string
}

type CapturedNode struct {
	UUID string
	Spec fingerprint.NodeSpec
}

type RecordedAction struct {
	UUID       string
	Sequence   int
	Kind       ActionKind
	Value      string
	Values     []string
	Hints      ActionHints
	PageURL    string
	NodeUUID   string
	NodeID     string
	Validation *ValidationSample
}

type CaptureResult struct {
	SessionID  string
	CaptureID  string
	NodeUUID   string
	NodeID     string
	ActionUUID string
	Sequence   int
	Created    bool
}

type Snapshot struct {
	ID         string
	WorkflowID string
	StartURL   string
	Status     Status
	Nodes      []CapturedNode
	Actions    []RecordedAction
}

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

func NewSession(workflowID, startURL string) (*Session, error) {
	if strings.TrimSpace(workflowID) == "" {
		return nil, fmt.Errorf("sampling: workflow ID is required")
	}
	if strings.TrimSpace(startURL) == "" {
		return nil, fmt.Errorf("sampling: start URL is required")
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

func (s *Session) ID() string { return s.id }

func (s *Session) Start() error {
	if s == nil {
		return fmt.Errorf("sampling: nil session cannot start")
	}
	if s.status != StatusCreated {
		return fmt.Errorf("sampling: session cannot start from %q", s.status)
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

func (s *Session) Record(c Capture) (CaptureResult, error) {
	if s == nil || s.status != StatusRecording {
		return CaptureResult{}, fmt.Errorf("sampling: session is not recording")
	}
	if strings.TrimSpace(c.CaptureID) == "" {
		return CaptureResult{}, fmt.Errorf("sampling: capture ID is required")
	}
	if previous, ok := s.byCaptureID[c.CaptureID]; ok {
		return previous, nil
	}
	switch c.Kind {
	case ActionClick, ActionInput, ActionSelect, ActionValidate:
		if strings.TrimSpace(c.IdentityKey) == "" {
			return CaptureResult{}, fmt.Errorf("sampling: node identity key is required")
		}
		if c.Kind == ActionValidate && c.Validation == nil {
			return CaptureResult{}, fmt.Errorf("sampling: validation capture is required")
		}
	case ActionPress:
		if strings.TrimSpace(c.Value) == "" {
			return CaptureResult{}, fmt.Errorf("sampling: press action value is required")
		}
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
	default:
		return CaptureResult{}, fmt.Errorf("sampling: unsupported action kind %q", c.Kind)
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
			return CaptureResult{}, fmt.Errorf("sampling: invalid captured node: %w", err)
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
			return CaptureResult{}, fmt.Errorf("sampling: invalid captured node update: %w", err)
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
		PageURL: c.PageURL, NodeUUID: node.UUID, NodeID: node.Spec.ID, Validation: cloneValidation(c.Validation),
	}
	s.actions = append(s.actions, action)
	result := CaptureResult{
		SessionID: s.id, CaptureID: c.CaptureID, NodeUUID: node.UUID, NodeID: node.Spec.ID,
		ActionUUID: action.UUID, Sequence: action.Sequence, Created: created,
	}
	s.byCaptureID[c.CaptureID] = result
	return result, nil
}

func (s *Session) Complete() error {
	return s.End()
}

// Pause：暂停会停止接受捕获而不关闭会话。  暂停的会话保留其身份映射，因此在恢复后对元素的重复采样仍然重用原始临时节点。
func (s *Session) Pause() error {
	if s == nil {
		return fmt.Errorf("sampling: nil session cannot pause")
	}
	if s.status != StatusRecording {
		return fmt.Errorf("sampling: session cannot pause from %q", s.status)
	}
	s.status = StatusPaused
	return nil
}

func (s *Session) Resume() error {
	if s == nil {
		return fmt.Errorf("sampling: nil session cannot resume")
	}
	if s.status != StatusPaused {
		return fmt.Errorf("sampling: session cannot resume from %q", s.status)
	}
	s.status = StatusRecording
	return nil
}

// End 是正常的终端生命周期转换。  它有意与暂停不同：结束的会话可以编辑/发布，但永远不会恢复。
func (s *Session) End() error {
	if s == nil {
		return fmt.Errorf("sampling: nil session cannot complete")
	}
	if s.status != StatusRecording && s.status != StatusPaused {
		return fmt.Errorf("sampling: session cannot complete from %q", s.status)
	}
	s.status = StatusEnded
	return nil
}

func (s *Session) Fail() {
	s.Interrupt()
}

func (s *Session) Interrupt() {
	if s != nil && (s.status == StatusCreated || s.status == StatusRecording || s.status == StatusPaused) {
		s.status = StatusInterrupted
	}
}

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
		ID: s.id, WorkflowID: s.workflowID, StartURL: s.startURL,
		Status: s.status, Nodes: nodes, Actions: actions,
	}
}

func cloneValidation(input *ValidationSample) *ValidationSample {
	if input == nil {
		return nil
	}
	copy := *input
	copy.SupportedKinds = append([]string(nil), input.SupportedKinds...)
	return &copy
}

func NewUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("sampling: generate UUID: %w", err)
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

func compactUUID(value string) string { return strings.ReplaceAll(value, "-", "") }

func originOf(value string) string {
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func cloneSpec(spec fingerprint.NodeSpec) fingerprint.NodeSpec {
	copy := spec
	copy.Selectors = append([]fingerprint.Selector(nil), spec.Selectors...)
	copy.Fingerprint.Attributes = make(map[string]string, len(spec.Fingerprint.Attributes))
	for key, value := range spec.Fingerprint.Attributes {
		copy.Fingerprint.Attributes[key] = value
	}
	copy.Fingerprint.Path = append([]string(nil), spec.Fingerprint.Path...)
	return copy
}
