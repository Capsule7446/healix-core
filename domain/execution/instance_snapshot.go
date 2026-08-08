package execution

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

// InstanceSnapshotSchema 标识实例快照的编码和环境变量模式版本。
type InstanceSnapshotSchema int

const (
	// InstanceSnapshotSchemaV1 表示使用字符串属性的快照模式。
	InstanceSnapshotSchemaV1 InstanceSnapshotSchema = 1
	// InstanceSnapshotSchemaV2 表示使用类型化变量的快照模式。
	InstanceSnapshotSchemaV2 InstanceSnapshotSchema = 2
	// InstanceSnapshotSchemaCurrent 表示当前写入的实例快照模式。
	InstanceSnapshotSchemaCurrent = InstanceSnapshotSchemaV2
	// ScreenshotPolicyV1 表示截图策略快照版本一。
	ScreenshotPolicyV1 = 1
	// HealerPolicyV1 表示自愈策略快照版本一。
	HealerPolicyV1 = 1
	// MaxSnapshotStringBytes 限制快照中单个字符串的字节数。
	MaxSnapshotStringBytes = 64 * 1024
)

// EnvironmentSnapshot 保存环境身份、修订号以及按模式区分的属性或变量。
type EnvironmentSnapshot struct {
	ID, DisplayName, BaseURL string
	Revision                 uint64
	Properties               map[string]string
	Variables                map[string]parameter.Value
}

// ScreenshotPolicySnapshot 保存截图策略版本、启用状态和目标位置。
type ScreenshotPolicySnapshot struct {
	Version     int
	Enabled     bool
	Destination string
}

// HealerWeightsSnapshot 保存评分器使用的各维度权重。
type HealerWeightsSnapshot struct {
	Tag, ID, RoleName, Class, Attrs, Text, Index, Neighbor, LabelText, Container float64
	// Framework 默认值为 0，因此除非调用方启用，否则不会使用框架维度；它与其他维度一样
	// 被携带、校验并写入摘要，确保评分所需权重都受快照身份保护。
	Framework float64
}

// HealerPolicySnapshot 保存自愈策略版本、审核/应用阈值和评分权重。
type HealerPolicySnapshot struct {
	Version               int
	ReviewCap, AppliedCap float64
	Weights               HealerWeightsSnapshot
}

// DefaultHealerPolicySnapshot 返回版本一的默认自愈策略快照。
func DefaultHealerPolicySnapshot() HealerPolicySnapshot {
	return HealerPolicySnapshot{Version: HealerPolicyV1, ReviewCap: .6, AppliedCap: .85, Weights: HealerWeightsSnapshot{Tag: .15, ID: .2, RoleName: .2, Class: .1, Attrs: .1, Text: .1, Index: .05, Neighbor: .1, LabelText: .15, Container: .1, Framework: 0}}
}

// TestTaskSnapshot 保存测试任务身份快照。
type TestTaskSnapshot struct {
	ID string
}

// ExecutionFlowVersionItemSnapshot 保存测试任务版本中一个有序入口项的身份映射。
type ExecutionFlowVersionItemSnapshot struct {
	ID                string
	TestTaskVersionID string
	SequenceNumber    int
	FlowFragmentID    string
	WorkflowVersionID string
}

// ExecutionFlowVersionSnapshot 保存测试任务版本及其入口项快照。
type ExecutionFlowVersionSnapshot struct {
	ID              string
	ExecutionFlowID string
	VersionNumber   int
	Items           []ExecutionFlowVersionItemSnapshot
}

// InvocationEdgeKey 以父调用路径和步骤 ID 标识工作流引用边。
type InvocationEdgeKey struct {
	ParentPath InvocationPath
	StepID     string
}

// InvocationScopeSnapshot 保存一次工作流调用作用域及其解析后的参数值和绑定。
type InvocationScopeSnapshot struct {
	Path               InvocationPath
	ParentPath         InvocationPath
	ParentVersionID    string
	StepID             string
	FlowFragmentID     string
	WorkflowVersionID  string
	ResolvedFromLatest bool
	Values             map[string]parameter.Value
	Bindings           map[string]parameter.Binding
}

// InstanceSnapshotInput 汇总封存实例快照所需的计划、调用作用域、环境和策略数据。
type InstanceSnapshotInput struct {
	SchemaVersion                      InstanceSnapshotSchema
	InstanceID                         InstanceID
	ExecutionFlowID, TestTaskVersionID string
	TestTaskVersionNumber              int
	ExecutionFlow                      TestTaskSnapshot
	ExecutionFlowVersion               ExecutionFlowVersionSnapshot
	Plan                               PlanSnapshot
	Invocations                        []InvocationScopeSnapshot
	Environment                        EnvironmentSnapshot
	FailurePolicy                      FailurePolicy
	ScreenshotPolicy                   ScreenshotPolicySnapshot
	HealerPolicy                       HealerPolicySnapshot
}

// InstanceSnapshot 保存深复制后的输入和其规范编码摘要；内部输入不可直接修改。
type InstanceSnapshot struct {
	input  InstanceSnapshotInput
	digest string
}

// Digest 返回快照的 sha256 摘要。
func (s InstanceSnapshot) Digest() string { return s.digest }

// SchemaVersion 返回快照模式版本。
func (s InstanceSnapshot) SchemaVersion() InstanceSnapshotSchema { return s.input.SchemaVersion }

// InstanceID 返回快照所属实例 ID。
func (s InstanceSnapshot) InstanceID() InstanceID { return s.input.InstanceID }

// ExecutionFlowID 返回快照所属执行流 ID。
func (s InstanceSnapshot) ExecutionFlowID() string { return s.input.ExecutionFlowID }

// TestTaskVersionID 返回快照所属测试任务版本 ID。
func (s InstanceSnapshot) TestTaskVersionID() string { return s.input.TestTaskVersionID }

// Input 返回实例快照输入的深拷贝，调用方可安全修改结果。
func (s InstanceSnapshot) Input() InstanceSnapshotInput { return cloneSnapshotInput(s.input) }

// Plan 返回计划快照的深拷贝。
func (s InstanceSnapshot) Plan() PlanSnapshot { return cloneDraft(s.input.Plan) }

// Invocations 返回调用作用域快照的深拷贝切片。
func (s InstanceSnapshot) Invocations() []InvocationScopeSnapshot {
	return cloneInvocations(s.input.Invocations)
}

// Invocation 按调用路径查找作用域并返回其深拷贝；未找到时返回 false。
func (s InstanceSnapshot) Invocation(path InvocationPath) (InvocationScopeSnapshot, bool) {
	for _, invocation := range s.input.Invocations {
		if invocation.Path == path {
			return cloneInvocations([]InvocationScopeSnapshot{invocation})[0], true
		}
	}
	return InvocationScopeSnapshot{}, false
}

// Environment 返回环境快照的深拷贝；V1 属性会转换为文本变量视图。
func (s InstanceSnapshot) Environment() EnvironmentSnapshot {
	result := cloneEnvironment(s.input.Environment)
	if s.input.SchemaVersion == InstanceSnapshotSchemaV1 {
		result.Variables = make(map[string]parameter.Value, len(result.Properties))
		for name, value := range result.Properties {
			result.Variables[name] = parameter.TextValue(value)
		}
	}
	return result
}

// SealInstanceSnapshot 深复制、规范排序并校验输入后生成带摘要的快照；未分类形状错误在
// 导出边界归入 EXECUTION_CREATE_INSTANCE_SNAPSHOT_INVALID，已有领域错误保持原分类。
func SealInstanceSnapshot(input InstanceSnapshotInput) (InstanceSnapshot, error) {
	sealed, err := sealInstanceSnapshotShape(input)
	if err != nil {
		return InstanceSnapshot{}, classifyCreateInstanceSnapshot(err)
	}
	return sealed, nil
}

// sealInstanceSnapshotShape 执行资源预检、输入复制、调用排序、策略归一化、结构校验和摘要编码。
func sealInstanceSnapshotShape(input InstanceSnapshotInput) (InstanceSnapshot, error) {
	if err := preflightInstanceSnapshot(input); err != nil {
		return InstanceSnapshot{}, err
	}
	input = cloneSnapshotInput(input)
	sort.Slice(input.Invocations, func(i, j int) bool {
		return input.Invocations[i].Path.String() < input.Invocations[j].Path.String()
	})
	normalizeHealerZeros(&input.HealerPolicy)
	if err := validateSnapshot(input); err != nil {
		return InstanceSnapshot{}, err
	}
	digester := sha256.New()
	encoder := canonicalEncoder{writer: digester}
	encodeSnapshot(&encoder, input)
	return InstanceSnapshot{input: input, digest: "sha256:" + hex.EncodeToString(digester.Sum(nil))}, nil
}

// HydrateInstanceSnapshot 重新封存输入并将摘要与持久化摘要比较；不一致时返回快照冲突错误。
func HydrateInstanceSnapshot(input InstanceSnapshotInput, storedDigest string) (InstanceSnapshot, error) {
	sealed, err := SealInstanceSnapshot(input)
	if err != nil {
		return InstanceSnapshot{}, err
	}
	if storedDigest != sealed.Digest() {
		return InstanceSnapshot{}, createInstanceSnapshotConflictError()
	}
	return sealed, nil
}

// cloneSnapshotInput 深复制计划、调用作用域、环境和版本入口切片，隔离快照所有权。
func cloneSnapshotInput(v InstanceSnapshotInput) InstanceSnapshotInput {
	v.Plan = cloneDraft(v.Plan)
	v.Invocations = cloneInvocations(v.Invocations)
	v.Environment = cloneEnvironment(v.Environment)
	v.ExecutionFlowVersion.Items = append([]ExecutionFlowVersionItemSnapshot(nil), v.ExecutionFlowVersion.Items...)
	return v
}

// cloneEnvironment 深复制属性、变量映射及变量值。
func cloneEnvironment(v EnvironmentSnapshot) EnvironmentSnapshot {
	properties := make(map[string]string, len(v.Properties))
	for name, value := range v.Properties {
		properties[name] = value
	}
	variables := make(map[string]parameter.Value, len(v.Variables))
	for name, value := range v.Variables {
		variables[name] = value.Clone()
	}
	v.Properties = properties
	v.Variables = variables
	return v
}

// cloneInvocations 深复制调用作用域切片及每个作用域的参数值和绑定映射。
func cloneInvocations(source []InvocationScopeSnapshot) []InvocationScopeSnapshot {
	result := make([]InvocationScopeSnapshot, len(source))
	for index, invocation := range source {
		result[index] = invocation
		result[index].Values = cloneParameterValues(invocation.Values)
		result[index].Bindings = cloneBindings(invocation.Bindings)
	}
	return result
}

// normalizeHealerZeros 将零浮点值规范化为正零，确保摘要编码不区分正负零。
func normalizeHealerZeros(policy *HealerPolicySnapshot) {
	values := []*float64{&policy.ReviewCap, &policy.AppliedCap, &policy.Weights.Tag, &policy.Weights.ID, &policy.Weights.RoleName, &policy.Weights.Class, &policy.Weights.Attrs, &policy.Weights.Text, &policy.Weights.Index, &policy.Weights.Neighbor, &policy.Weights.LabelText, &policy.Weights.Container, &policy.Weights.Framework}
	for _, value := range values {
		if *value == 0 {
			*value = 0
		}
	}
}

// maxSnapshotElements 限制快照资源预检中可遍历的集合元素总数。
const maxSnapshotElements = 100000

// maxSnapshotDepth 限制快照资源预检的递归深度。
const maxSnapshotDepth = 64

// preflightInstanceSnapshot 在完整校验前限制计划、字符串、元素数量和递归深度资源。
func preflightInstanceSnapshot(input InstanceSnapshotInput) error {
	if err := validateAggregateInputBounds(input.Plan); err != nil {
		return fmt.Errorf("instance snapshot plan bounds: %w", err)
	}
	remainingBytes := MaxAggregateStringBytes
	remainingElements := maxSnapshotElements
	return consumeSnapshotResources(reflect.ValueOf(input), &remainingBytes, &remainingElements, 0)
}

// consumeSnapshotResources 递归计算快照反射值占用的字符串字节、集合元素和深度预算。
func consumeSnapshotResources(value reflect.Value, bytes, elements *int, depth int) error {
	if depth > maxSnapshotDepth {
		return errors.New("instance snapshot exceeds resource depth limit")
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		return consumeSnapshotResources(value.Elem(), bytes, elements, depth+1)
	}
	if value.CanInterface() {
		switch typed := value.Interface().(type) {
		case parameter.Value:
			strings := []string{string(typed.Type())}
			switch typed.Type() {
			case parameter.Text:
				strings = append(strings, typed.Text())
			case parameter.Number:
				strings = append(strings, typed.Number())
			case parameter.SingleSelect:
				strings = append(strings, typed.SingleSelect())
			case parameter.MultiSelect:
				strings = append(strings, typed.MultiSelect()...)
			}
			return consumeStrings(strings, bytes, elements)
		case parameter.OptionalValue:
			if literal, present := typed.Value(); present {
				return consumeSnapshotResources(reflect.ValueOf(literal), bytes, elements, depth+1)
			}
			return nil
		case parameter.Binding:
			if err := consumeStrings([]string{string(typed.Kind())}, bytes, elements); err != nil {
				return err
			}
			if literal, ok := typed.Literal(); ok {
				return consumeSnapshotResources(reflect.ValueOf(literal), bytes, elements, depth+1)
			}
			if parent, ok := typed.ParentName(); ok {
				return consumeStrings([]string{parent}, bytes, elements)
			}
			return nil
		}
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		return consumeSnapshotResources(value.Elem(), bytes, elements, depth+1)
	}
	switch value.Kind() {
	case reflect.String:
		return consumeStrings([]string{value.String()}, bytes, elements)
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if err := consumeSnapshotResources(value.Field(i), bytes, elements, depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if err := consumeElements(value.Len(), elements); err != nil {
			return err
		}
		for i := 0; i < value.Len(); i++ {
			if err := consumeSnapshotResources(value.Index(i), bytes, elements, depth+1); err != nil {
				return err
			}
		}
	case reflect.Map:
		if err := consumeElements(value.Len(), elements); err != nil {
			return err
		}
		for _, key := range value.MapKeys() {
			if err := consumeSnapshotResources(key, bytes, elements, depth+1); err != nil {
				return err
			}
			if err := consumeSnapshotResources(value.MapIndex(key), bytes, elements, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// consumeStrings 消耗字符串数量和字节预算，并拒绝超过单值或剩余总量上限的值。
func consumeStrings(values []string, bytes, elements *int) error {
	if err := consumeElements(len(values), elements); err != nil {
		return err
	}
	for _, value := range values {
		if len(value) > MaxStringBytes || len(value) > *bytes {
			return errors.New("instance snapshot aggregate string bytes exceed limit")
		}
		*bytes -= len(value)
	}
	return nil
}

// consumeElements 从剩余元素预算中扣除 count，超出或负数时返回错误。
func consumeElements(count int, remaining *int) error {
	if count < 0 || count > *remaining {
		return errors.New("instance snapshot aggregate elements exceed limit")
	}
	*remaining -= count
	return nil
}

// validString 判断字符串是否满足必填条件（如要求）及快照字节长度上限。
func validString(v string, required bool) bool {
	return (!required || strings.TrimSpace(v) != "") && len(v) <= MaxSnapshotStringBytes
}

// validateTestTaskVersionItemEntries 校验测试任务版本入口项与执行入口的数量、顺序和身份映射。
func validateTestTaskVersionItemEntries(versionID string, items []ExecutionFlowVersionItemSnapshot, entries []Entry) error {
	itemsByID := make(map[string]ExecutionFlowVersionItemSnapshot, len(items))
	for index, item := range items {
		if !validString(item.ID, true) || item.TestTaskVersionID != versionID || item.SequenceNumber != index+1 || !validString(item.FlowFragmentID, true) || !validString(item.WorkflowVersionID, true) {
			return errors.New("test-task version item graph is inconsistent")
		}
		if _, exists := itemsByID[item.ID]; exists {
			return errors.New("duplicate test-task version item")
		}
		itemsByID[item.ID] = item
	}
	if len(entries) != len(items) {
		return errors.New("test-task items and execution entries must have equal count")
	}
	matchedItems := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if _, duplicate := matchedItems[entry.TestTaskItemID]; duplicate {
			return errors.New("duplicate execution entry item")
		}
		matchedItems[entry.TestTaskItemID] = struct{}{}
		item, found := itemsByID[entry.TestTaskItemID]
		if !found {
			return errors.New("test-task item execution entry is missing")
		}
		if entry.SequenceNumber != item.SequenceNumber || entry.FlowFragmentID != item.FlowFragmentID || entry.WorkflowVersionID != item.WorkflowVersionID {
			return errors.New("test-task item and execution entry identity mismatch")
		}
	}
	return nil
}

// referenceEdgeKey 以父工作流版本和步骤 ID 索引引用步骤边。
type referenceEdgeKey struct {
	ParentVersionID string
	StepID          string
}

// snapshotValidationIndexes 保存快照校验阶段对计划入口、工作流和引用边的索引。
type snapshotValidationIndexes struct {
	workflows         map[string]WorkflowSnapshot
	entriesByID       map[EntryID]Entry
	entriesByRootPath map[InvocationPath]Entry
	referenceSteps    map[referenceEdgeKey]Step
	referenceByEdge   map[referenceEdgeKey]ReferenceResolution
	stepsByWorkflowID map[string][]Step
}

// buildSnapshotValidationIndexes 构建并校验计划入口、工作流和引用解析边的唯一索引。
func buildSnapshotValidationIndexes(plan PlanSnapshot) (snapshotValidationIndexes, error) {
	indexes := snapshotValidationIndexes{
		workflows:         make(map[string]WorkflowSnapshot, len(plan.Workflows)),
		entriesByID:       make(map[EntryID]Entry, len(plan.Entries)),
		entriesByRootPath: make(map[InvocationPath]Entry, len(plan.Entries)),
		referenceSteps:    make(map[referenceEdgeKey]Step),
		referenceByEdge:   make(map[referenceEdgeKey]ReferenceResolution, len(plan.References)),
		stepsByWorkflowID: make(map[string][]Step, len(plan.Workflows)),
	}
	for _, entry := range plan.Entries {
		if _, exists := indexes.entriesByID[entry.ID]; exists {
			return snapshotValidationIndexes{}, fmt.Errorf("duplicate execution entry %q", entry.ID)
		}
		indexes.entriesByID[entry.ID] = entry
		indexes.entriesByRootPath[RootInvocationPath(entry.ID)] = entry
	}
	for _, workflow := range plan.Workflows {
		if _, exists := indexes.workflows[workflow.VersionID]; exists {
			return snapshotValidationIndexes{}, errors.New("duplicate workflow version")
		}
		indexes.workflows[workflow.VersionID] = workflow
		steps := workflowReferenceSteps(workflow.Steps)
		indexes.stepsByWorkflowID[workflow.VersionID] = steps
		for _, step := range steps {
			key := referenceEdgeKey{ParentVersionID: workflow.VersionID, StepID: step.ID}
			if _, exists := indexes.referenceSteps[key]; exists {
				return snapshotValidationIndexes{}, errors.New("duplicate workflow reference step edge")
			}
			indexes.referenceSteps[key] = step
		}
	}
	for _, resolution := range plan.References {
		key := referenceEdgeKey{ParentVersionID: resolution.ParentVersionID, StepID: resolution.StepID}
		if _, exists := indexes.referenceByEdge[key]; exists {
			return snapshotValidationIndexes{}, errors.New("duplicate reference resolution edge")
		}
		indexes.referenceByEdge[key] = resolution
	}
	return indexes, nil
}

// validateSnapshot 校验实例快照模式、身份图、计划、调用作用域、策略和环境的一致性。
func validateSnapshot(v InstanceSnapshotInput) error {
	if v.SchemaVersion != InstanceSnapshotSchemaV1 && v.SchemaVersion != InstanceSnapshotSchemaV2 {
		return fmt.Errorf("unsupported instance snapshot schema %d", v.SchemaVersion)
	}
	if !validString(v.InstanceID.String(), true) || !validString(v.ExecutionFlowID, true) || !validString(v.TestTaskVersionID, true) || v.TestTaskVersionNumber < 1 {
		return errors.New("instance and test-task version identity is required")
	}
	if v.ExecutionFlow.ID != v.ExecutionFlowID ||
		v.ExecutionFlowVersion.ID != v.TestTaskVersionID || v.ExecutionFlowVersion.ExecutionFlowID != v.ExecutionFlowID ||
		v.ExecutionFlowVersion.VersionNumber != v.TestTaskVersionNumber {
		return errors.New("test-task snapshot graph identity is inconsistent")
	}
	if err := validateTestTaskVersionItemEntries(v.TestTaskVersionID, v.ExecutionFlowVersion.Items, v.Plan.Entries); err != nil {
		return err
	}
	if v.Plan.InstanceID != v.InstanceID || v.Plan.FailurePolicy != v.FailurePolicy {
		return errors.New("execution plan identity is inconsistent")
	}
	if err := v.Plan.Validate(); err != nil {
		return err
	}
	indexes, err := buildSnapshotValidationIndexes(v.Plan)
	if err != nil {
		return err
	}
	paths := make(map[InvocationPath]InvocationScopeSnapshot, len(v.Invocations))
	for _, invocation := range v.Invocations {
		if invocation.Path.Validate() != nil || !validString(invocation.FlowFragmentID, true) || !validString(invocation.WorkflowVersionID, true) {
			return errors.New("invocation identity is invalid")
		}
		if _, exists := paths[invocation.Path]; exists {
			return errors.New("duplicate invocation path")
		}
		paths[invocation.Path] = invocation
	}
	states := make(map[InvocationPath]uint8, len(paths))
	var visit func(InvocationPath) error
	visit = func(path InvocationPath) error {
		switch states[path] {
		case 1:
			return errors.New("invocation parent cycle")
		case 2:
			return nil
		}
		states[path] = 1
		parentPath := paths[path].ParentPath
		if parentPath != (InvocationPath{}) {
			if _, exists := paths[parentPath]; !exists {
				return errors.New("invocation parent path is missing")
			}
			if err := visit(parentPath); err != nil {
				return err
			}
		}
		states[path] = 2
		return nil
	}
	for path := range paths {
		if err := visit(path); err != nil {
			return err
		}
	}
	for _, invocation := range v.Invocations {
		if invocation.ParentPath == (InvocationPath{}) {
			if len(invocation.Bindings) != 0 {
				return errors.New("root invocation cannot have bindings")
			}
			if invocation.ParentVersionID != "" || invocation.StepID != "" {
				return errors.New("root invocation cannot identify a reference edge")
			}
			// 入口与其中的根调用位置是不同概念，即使它们的字符串表示可能相同；此处通过
			// RootInvocationPath 派生关系查找，不把调用路径与入口 ID 视为同一值。
			entry, exists := indexes.entriesByRootPath[invocation.Path]
			if !exists || entry.FlowFragmentID != invocation.FlowFragmentID || entry.WorkflowVersionID != invocation.WorkflowVersionID || !equalValues(entry.Parameters.Values, invocation.Values) {
				return errors.New("root invocation and execution entry scope diverge")
			}
		} else {
			parent, exists := paths[invocation.ParentPath]
			if !exists {
				return errors.New("invocation parent path is missing")
			}
			expectedPath, pathErr := invocation.ParentPath.Child(invocation.StepID)
			if pathErr != nil || invocation.Path != expectedPath {
				return fmt.Errorf("invocation %s path is not canonical for parent %s and step %s", invocation.Path, invocation.ParentPath, invocation.StepID)
			}
			key := referenceEdgeKey{ParentVersionID: invocation.ParentVersionID, StepID: invocation.StepID}
			resolution, exists := indexes.referenceByEdge[key]
			step, stepExists := indexes.referenceSteps[key]
			if !exists || !stepExists || step.Reference == nil || parent.WorkflowVersionID != invocation.ParentVersionID || resolution.FlowFragmentID != invocation.FlowFragmentID || resolution.WorkflowVersionID != invocation.WorkflowVersionID || resolution.ResolvedFromLatest != invocation.ResolvedFromLatest {
				return errors.New("invocation reference edge is inconsistent")
			}
			resolvedValues := make(map[string]parameter.Value, len(invocation.Bindings))
			for name, binding := range invocation.Bindings {
				resolved, err := binding.Resolve(parent.Values)
				if err != nil {
					return wrapOrPropagate(err, func(cause error) error {
						return fmt.Errorf("invocation %s binding %s: %w", invocation.Path, name, cause)
					})
				}
				resolvedValues[name] = resolved
			}
			target, targetExists := indexes.workflows[invocation.WorkflowVersionID]
			if !targetExists {
				return errors.New("invocation workflow version is missing")
			}
			for _, definition := range target.Parameters {
				if _, exists := resolvedValues[definition.Name]; exists {
					continue
				}
				if value, present := definition.Default.Value(); present {
					resolvedValues[definition.Name] = value
				}
			}
			if err := validateSnapshotValues(target.Parameters, resolvedValues); err != nil {
				return wrapOrPropagate(err, func(cause error) error {
					return fmt.Errorf("invocation %s parameter values: %w", invocation.Path, cause)
				})
			}
			if !equalValues(resolvedValues, invocation.Values) {
				return errors.New("invocation values and bindings diverge")
			}
		}
		for _, value := range invocation.Values {
			if err := value.Validate(); err != nil {
				return err
			}
		}
	}
	roots := 0
	childrenByEdge := make(map[InvocationEdgeKey][]InvocationScopeSnapshot)
	for _, invocation := range v.Invocations {
		if invocation.ParentPath == (InvocationPath{}) {
			roots++
			continue
		}
		key := InvocationEdgeKey{ParentPath: invocation.ParentPath, StepID: invocation.StepID}
		childrenByEdge[key] = append(childrenByEdge[key], invocation)
	}
	if roots != len(v.Plan.Entries) {
		return errors.New("one root invocation per execution entry is required")
	}
	for _, parent := range v.Invocations {
		workflow, exists := indexes.workflows[parent.WorkflowVersionID]
		if !exists {
			return errors.New("invocation workflow version is missing")
		}
		for _, step := range indexes.stepsByWorkflowID[workflow.VersionID] {
			children := childrenByEdge[InvocationEdgeKey{ParentPath: parent.Path, StepID: step.ID}]
			if len(children) != 1 {
				return errors.New("one child invocation per concrete reference edge is required")
			}
		}
	}
	if !v.FailurePolicy.IsValid() {
		return errors.New("invalid failure policy")
	}
	if err := validateEnvironmentSnapshot(v.SchemaVersion, v.Environment, v.ScreenshotPolicy, v.HealerPolicy); err != nil {
		return err
	}
	return nil
}

// equalBindings 比较两个参数绑定映射的键集合、绑定种类及字面量或父参数内容。
func equalBindings(left, right map[string]parameter.Binding) bool {
	if len(left) != len(right) {
		return false
	}
	for name, binding := range left {
		other, exists := right[name]
		if !exists || binding.Kind() != other.Kind() {
			return false
		}
		if literal, ok := binding.Literal(); ok {
			otherLiteral, otherOK := other.Literal()
			if !otherOK || !literal.Equal(otherLiteral) {
				return false
			}
			continue
		}
		parent, ok := binding.ParentName()
		otherParent, otherOK := other.ParentName()
		if !ok || !otherOK || parent != otherParent {
			return false
		}
	}
	return true
}

// workflowReferenceSteps 递归收集步骤树和校验分支中的工作流引用步骤。
func workflowReferenceSteps(steps []Step) []Step {
	var result []Step
	for _, step := range steps {
		if step.Kind == FlowFragmentReference {
			result = append(result, step)
		}
		result = append(result, workflowReferenceSteps(step.Children)...)
		if step.ValidationGroup != nil {
			for _, branch := range step.ValidationGroup.Branches {
				result = append(result, workflowReferenceSteps(branch.Steps)...)
			}
		}
	}
	return result
}

// equalValues 比较两个参数值映射的键集合和值内容。
func equalValues(left, right map[string]parameter.Value) bool {
	if len(left) != len(right) {
		return false
	}
	for name, value := range left {
		other, exists := right[name]
		if !exists || !value.Equal(other) {
			return false
		}
	}
	return true
}

// canonicalEncoder 以稳定的长度前缀和大端整数向哈希写入规范快照表示。
type canonicalEncoder struct{ writer hash.Hash }

// raw 写入原始字节；哈希实现的写入错误被刻意忽略，因为标准哈希写入不会失败。
func (e *canonicalEncoder) raw(value []byte) { _, _ = e.writer.Write(value) }

// u64 以大端序写入无符号整数。
func (e *canonicalEncoder) u64(v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	e.raw(b[:])
}

// str 写入带字节长度前缀的字符串。
func (e *canonicalEncoder) str(v string) { e.u64(uint64(len(v))); e.raw([]byte(v)) }

// boolean 以单字节写入布尔值。
func (e *canonicalEncoder) boolean(v bool) {
	if v {
		e.raw([]byte{1})
	} else {
		e.raw([]byte{0})
	}
}

// encodeSnapshot 按固定字段顺序写入实例快照及其策略、环境和计划数据。
// healix.run-snapshot 是线协议标签，不是 Go 名称；摘要契约要求其字节保持稳定。
func encodeSnapshot(e *canonicalEncoder, v InstanceSnapshotInput) {
	e.str("healix.run-snapshot")
	e.u64(uint64(v.SchemaVersion))
	e.str(v.InstanceID.String())
	e.str(v.ExecutionFlowID)
	e.str(v.TestTaskVersionID)
	e.u64(uint64(v.TestTaskVersionNumber))
	e.str(v.ExecutionFlow.ID)
	e.str(v.ExecutionFlowVersion.ID)
	e.str(v.ExecutionFlowVersion.ExecutionFlowID)
	e.u64(uint64(v.ExecutionFlowVersion.VersionNumber))
	e.u64(uint64(len(v.ExecutionFlowVersion.Items)))
	for _, item := range v.ExecutionFlowVersion.Items {
		e.str(item.ID)
		e.str(item.TestTaskVersionID)
		e.u64(uint64(item.SequenceNumber))
		e.str(item.FlowFragmentID)
		e.str(item.WorkflowVersionID)
	}
	encodeCanonical(e, reflect.ValueOf(v.Plan))
	encodeCanonical(e, reflect.ValueOf(v.Invocations))
	e.str(v.Environment.ID)
	e.str(v.Environment.DisplayName)
	e.str(v.Environment.BaseURL)
	e.u64(v.Environment.Revision)
	if v.SchemaVersion == InstanceSnapshotSchemaV1 {
		encodeStrings(e, v.Environment.Properties)
	} else {
		encodeParameterValues(e, v.Environment.Variables)
	}
	e.str(string(v.FailurePolicy))
	e.u64(uint64(v.ScreenshotPolicy.Version))
	e.boolean(v.ScreenshotPolicy.Enabled)
	e.str(v.ScreenshotPolicy.Destination)
	e.u64(uint64(v.HealerPolicy.Version))
	e.u64(math.Float64bits(v.HealerPolicy.ReviewCap))
	e.u64(math.Float64bits(v.HealerPolicy.AppliedCap))
	for _, x := range []float64{v.HealerPolicy.Weights.Tag, v.HealerPolicy.Weights.ID, v.HealerPolicy.Weights.RoleName, v.HealerPolicy.Weights.Class, v.HealerPolicy.Weights.Attrs, v.HealerPolicy.Weights.Text, v.HealerPolicy.Weights.Index, v.HealerPolicy.Weights.Neighbor, v.HealerPolicy.Weights.LabelText, v.HealerPolicy.Weights.Container, v.HealerPolicy.Weights.Framework} {
		e.u64(math.Float64bits(x))
	}
}

// encodeCanonical 递归编码反射值，针对执行坐标和参数值使用稳定的领域表示。
func encodeCanonical(e *canonicalEncoder, value reflect.Value) {
	if value.Kind() == reflect.Interface {
		value = value.Elem()
	}
	if value.CanInterface() {
		switch typed := value.Interface().(type) {
		// 执行坐标按其规范身份字符串编码，而不是按值对象的结构体字段编码；快照摘要是持久化
		// 契约，坐标的领域表示必须保持稳定。
		case InstanceID:
			e.str(typed.String())
			return
		case EntryID:
			e.str(typed.String())
			return
		case StepExecutionID:
			e.str(typed.String())
			return
		case InvocationPath:
			e.str(typed.String())
			return
		case parameter.Value:
			encodeValue(e, typed)
			return
		case parameter.OptionalValue:
			literal, present := typed.Value()
			e.boolean(present)
			if present {
				encodeValue(e, literal)
			}
			return
		case parameter.Binding:
			e.str(string(typed.Kind()))
			if literal, ok := typed.Literal(); ok {
				encodeValue(e, literal)
			} else if name, ok := typed.ParentName(); ok {
				e.str(name)
			}
			return
		}
	}
	if value.Kind() == reflect.Pointer {
		e.boolean(!value.IsNil())
		if !value.IsNil() {
			encodeCanonical(e, value.Elem())
		}
		return
	}
	switch value.Kind() {
	case reflect.Struct:
		e.u64(uint64(value.NumField()))
		for index := 0; index < value.NumField(); index++ {
			encodeCanonical(e, value.Field(index))
		}
	case reflect.Slice:
		e.u64(uint64(value.Len()))
		for index := 0; index < value.Len(); index++ {
			encodeCanonical(e, value.Index(index))
		}
	case reflect.Map:
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		e.u64(uint64(len(keys)))
		for _, key := range keys {
			e.str(key.String())
			encodeCanonical(e, value.MapIndex(key))
		}
	case reflect.String:
		e.str(value.String())
	case reflect.Bool:
		e.boolean(value.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.u64(uint64(value.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		e.u64(value.Uint())
	case reflect.Float64:
		f := value.Float()
		if f == 0 {
			f = 0
		}
		e.u64(math.Float64bits(f))
	default:
		e.str(fmt.Sprint(value.Interface()))
	}
}

// sortedKeys 按字典序返回映射键，保证规范编码和校验顺序确定。
func sortedKeys[V any](m map[string]V) []string {
	k := make([]string, 0, len(m))
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}

// encodeStrings 按排序键编码字符串映射。
func encodeStrings(e *canonicalEncoder, m map[string]string) {
	e.u64(uint64(len(m)))
	for _, k := range sortedKeys(m) {
		e.str(k)
		e.str(m[k])
	}
}

// encodeParameterValues 按排序键编码类型化参数值映射。
func encodeParameterValues(e *canonicalEncoder, values map[string]parameter.Value) {
	e.u64(uint64(len(values)))
	for _, name := range sortedKeys(values) {
		e.str(name)
		encodeValue(e, values[name])
	}
}

// encodeValue 按参数类型编码参数值及其有序多选项。
func encodeValue(e *canonicalEncoder, v parameter.Value) {
	e.str(string(v.Type()))
	switch v.Type() {
	case parameter.Text:
		e.str(v.Text())
	case parameter.Number:
		e.str(v.Number())
	case parameter.Boolean:
		e.boolean(v.Boolean())
	case parameter.SingleSelect:
		e.str(v.SingleSelect())
	case parameter.MultiSelect:
		items := v.MultiSelect()
		e.u64(uint64(len(items)))
		for _, x := range items {
			e.str(x)
		}
	}
}
