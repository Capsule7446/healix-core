package engine

import (
	"errors"
	"fmt"
	"time"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/node"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// StepMetadata 将执行树 ID 映射回拥有证据、截图和面向用户进度信息的
// 不可变工作区步骤。
type StepMetadata struct {
	FlowFragmentStepID     string
	DisplayName            string
	Kind                   string
	HierarchyPath          string
	ElementTargetID        string
	ElementTargetVersionID string
	CaptureScreenshot      bool
	InvocationPath         execution.InvocationPath
}

// RuntimeNodeIdentity 将运行时 ElementTargetSpec ID（即精确的 ElementTargetVersion ID）映射到
// 其稳定的工作区 ElementTarget 标识。
type RuntimeNodeIdentity struct {
	ElementTargetID        string
	ElementTargetVersionID string
}

// compiledExecutionIdentity 保存编译条目绑定的执行实例、快照摘要和入口身份。
type compiledExecutionIdentity struct {
	instanceID     execution.InstanceID
	snapshotDigest string
	entryID        execution.EntryID
}

// CompiledEntry 保存一个已编译执行入口及其不可变快照身份、运行时元数据和内部程序。
type CompiledEntry struct {
	InstanceID        execution.InstanceID
	SnapshotDigest    string
	EntryID           execution.EntryID
	TestTaskItemID    string
	SequenceNumber    int
	FlowFragmentID    string
	WorkflowVersionID string
	Metadata          map[string]StepMetadata
	RuntimeNodes      map[string]RuntimeNodeIdentity
	program           node.Program
	identity          compiledExecutionIdentity
}

// CompiledPlan 保存按执行顺序排列的编译入口及其私有 ID 索引。
type CompiledPlan struct {
	entries []CompiledEntry
	byID    map[execution.EntryID]int
}

// Entries 返回按执行顺序排列的编译入口；返回切片及每个入口的导出映射均归调用方所有。
func (r CompiledPlan) Entries() []CompiledEntry {
	entries := make([]CompiledEntry, len(r.entries))
	for index, entry := range r.entries {
		entries[index] = cloneCompiledEntry(entry)
	}
	return entries
}

// Entry 返回由 entryID 标识的编译入口，不暴露运行时私有索引；入口不存在或身份校验失败时返回 false。
func (r CompiledPlan) Entry(entryID execution.EntryID) (CompiledEntry, bool) {
	index, ok := r.byID[entryID]
	if !ok || index < 0 || index >= len(r.entries) {
		return CompiledEntry{}, false
	}
	entry := r.entries[index]
	if !entry.hasIdentity(entryID) {
		return CompiledEntry{}, false
	}
	return cloneCompiledEntry(entry), true
}

// hasIdentity 校验编译入口的入口 ID、实例 ID 和快照摘要与私有绑定身份一致。
func (entry CompiledEntry) hasIdentity(entryID execution.EntryID) bool {
	return entryID.Validate() == nil &&
		entry.InstanceID.Validate() == nil && entry.InstanceID == entry.identity.instanceID &&
		entry.SnapshotDigest != "" && entry.SnapshotDigest == entry.identity.snapshotDigest &&
		entry.EntryID == entryID && entry.EntryID == entry.identity.entryID
}

// cloneCompiledEntry 复制编译入口的导出映射，保留内部程序和身份绑定不变。
func cloneCompiledEntry(entry CompiledEntry) CompiledEntry {
	metadataSource := entry.Metadata
	entry.Metadata = make(map[string]StepMetadata, len(metadataSource))
	for id, metadata := range metadataSource {
		entry.Metadata[id] = metadata
	}
	runtimeNodesSource := entry.RuntimeNodes
	entry.RuntimeNodes = make(map[string]RuntimeNodeIdentity, len(runtimeNodesSource))
	for id, identity := range runtimeNodesSource {
		entry.RuntimeNodes[id] = identity
	}
	return entry
}

// planUnsealedError 返回 domain/execution 为计划未封存状态注册的错误码和固定消息。
func planUnsealedError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, execution.CodePlanUnsealed, "execution plan must be sealed")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// CompilePlan 仅根据不可变执行实例快照负载编译计划；每个返回入口都绑定快照中的运行、摘要和执行 ID。
func CompilePlan(snapshot execution.InstanceSnapshot) (CompiledPlan, error) {
	if snapshot.Digest() == "" {
		return CompiledPlan{}, planUnsealedError()
	}
	return compileSnapshotDraft(snapshot.Plan(), snapshot)
}

// compileSnapshotDraft 校验快照中的工作流、引用、节点和调用范围，并构造入口索引与运行时程序。
func compileSnapshotDraft(draft execution.PlanSnapshot, snapshot execution.InstanceSnapshot) (CompiledPlan, error) {
	versions := make(map[string]execution.WorkflowSnapshot, len(draft.Workflows))
	resolutions := make(map[execution.WorkflowReferenceKey]execution.ReferenceResolution, len(draft.References))
	nodes := make(map[execution.NodeDependencyKey]execution.NodeSnapshot, len(draft.Nodes))
	for _, workflow := range draft.Workflows {
		if _, exists := versions[workflow.VersionID]; exists {
			return CompiledPlan{}, fmt.Errorf("duplicate workflow version %s", workflow.VersionID)
		}
		versions[workflow.VersionID] = workflow
	}
	for _, resolution := range draft.References {
		key := referenceKey(resolution.ParentVersionID, resolution.StepID)
		if _, exists := resolutions[key]; exists {
			return CompiledPlan{}, fmt.Errorf("duplicate reference resolution for workflow version %s step %s", resolution.ParentVersionID, resolution.StepID)
		}
		resolutions[key] = resolution
	}
	for _, snapshot := range draft.Nodes {
		key := nodeDependencyIdentity(snapshot.ElementTargetID, snapshot.VersionID)
		if _, exists := nodes[key]; exists {
			return CompiledPlan{}, fmt.Errorf("duplicate node dependency %s version %s", snapshot.ElementTargetID, snapshot.VersionID)
		}
		nodes[key] = snapshot
	}
	compiledNodes := 0
	invocations := snapshot.Invocations()
	invocationsByEdge := invocationIndex(invocations)
	invocationsByPath := make(map[execution.InvocationPath]execution.InvocationScopeSnapshot, len(invocations))
	for _, invocation := range invocations {
		if _, exists := invocationsByPath[invocation.Path]; exists {
			return CompiledPlan{}, fmt.Errorf("duplicate invocation path %s", invocation.Path)
		}
		invocationsByPath[invocation.Path] = invocation
	}
	environment := snapshot.Environment()
	result := CompiledPlan{entries: make([]CompiledEntry, 0, len(draft.Entries)), byID: make(map[execution.EntryID]int, len(draft.Entries))}
	for _, entry := range draft.Entries {
		entryID := entry.ID
		if _, exists := result.byID[entryID]; exists {
			return CompiledPlan{}, fmt.Errorf("duplicate execution %s", entryID)
		}
		compiler := executionCompiler{
			versions: versions, resolutions: resolutions, nodes: nodes, invocations: invocationsByEdge,
			programSpecs: make(map[string]fingerprint.ElementTargetSpec),
			metadata:     make(map[string]StepMetadata), runtimeNodes: make(map[string]RuntimeNodeIdentity),
			compiledNodes: &compiledNodes,
		}
		rootPath := encodeRuntimeComponent(entryID.String())
		root, err := compiler.compileWorkflow(entry.WorkflowVersionID, rootPath, execution.RootInvocationPath(entryID), 1)
		if err != nil {
			// 内层失败已是分类错误；此处保持原错误，避免掩盖分类并将执行 ID 拼入公共文本。
			return CompiledPlan{}, err
		}
		root.OwnsParameterScope = true
		invocation, exists := invocationsByPath[execution.RootInvocationPath(entryID)]
		if !exists {
			return CompiledPlan{}, fmt.Errorf("compile execution %s: root invocation is missing", entryID)
		}
		root.Parameters = cloneParameterValues(invocation.Values)
		if len(environment.Variables) > 0 && root.Parameters == nil {
			root.Parameters = make(map[string]parameter.Value, len(environment.Variables))
		}
		for name, value := range environment.Variables {
			key := "env." + name
			if _, collision := root.Parameters[key]; collision {
				// 不在消息中带执行 ID 或参数名；该碰撞属于未分类输入错误，公共文本不回显身份。
				return CompiledPlan{}, errors.New("compile execution: environment parameter collides with workflow scope")
			}
			root.Parameters[key] = value.Clone()
		}
		compiledEntry := CompiledEntry{
			InstanceID: snapshot.InstanceID(), SnapshotDigest: snapshot.Digest(),
			EntryID: entryID, TestTaskItemID: entry.TestTaskItemID, SequenceNumber: entry.SequenceNumber,
			FlowFragmentID: entry.FlowFragmentID, WorkflowVersionID: entry.WorkflowVersionID,
			program:  node.Program{Root: root, Specs: compiler.programSpecs},
			Metadata: compiler.metadata, RuntimeNodes: compiler.runtimeNodes,
			identity: compiledExecutionIdentity{instanceID: snapshot.InstanceID(), snapshotDigest: snapshot.Digest(), entryID: entryID},
		}
		result.byID[entryID] = len(result.entries)
		result.entries = append(result.entries, compiledEntry)
	}
	return result, nil
}

// executionCompiler 持有快照依赖和编译期间累积的节点规格、元数据与运行时映射。
type executionCompiler struct {
	versions      map[string]execution.WorkflowSnapshot
	resolutions   map[execution.WorkflowReferenceKey]execution.ReferenceResolution
	nodes         map[execution.NodeDependencyKey]execution.NodeSnapshot
	programSpecs  map[string]fingerprint.ElementTargetSpec
	metadata      map[string]StepMetadata
	runtimeNodes  map[string]RuntimeNodeIdentity
	invocations   map[execution.InvocationEdgeKey]execution.InvocationScopeSnapshot
	compiledNodes *int
}

// compileWorkflow 将指定工作流版本展开为运行时工作流节点，并递归处理引用深度。
func (c *executionCompiler) compileWorkflow(versionID, invocationPath string, scopePath execution.InvocationPath, depth int) (*node.WorkflowNode, error) {
	if depth > execution.MaxWorkflowReferenceDepth {
		return nil, fmt.Errorf("compile depth exceeds maximum %d", execution.MaxWorkflowReferenceDepth)
	}
	dependency, ok := c.versions[versionID]
	if !ok {
		return nil, fmt.Errorf("workflow version %s is missing from the instance snapshot", versionID)
	}
	workflowRuntimeID := "workflow|" + invocationPath
	c.metadata[workflowRuntimeID] = StepMetadata{FlowFragmentStepID: versionID, DisplayName: dependency.DisplayName,
		Kind: "WORKFLOW", HierarchyPath: dependency.DisplayName, InvocationPath: scopePath}
	children, err := c.compileSteps(versionID, invocationPath, scopePath, dependency.Steps, dependency.DisplayName, depth)
	if err != nil {
		return nil, err
	}
	return &node.WorkflowNode{NodeID: workflowRuntimeID, Children: children}, nil
}

// compileSteps 将工作流步骤编译为运行时节点，维护层级路径、调用路径和展开数量上限。
func (c *executionCompiler) compileSteps(parentVersionID, invocationPath string, scopePath execution.InvocationPath, steps []execution.Step, hierarchy string, depth int) ([]node.Node, error) {
	result := make([]node.Node, 0, len(steps))
	for _, step := range steps {
		*c.compiledNodes++
		if *c.compiledNodes > execution.MaxExpandedExecutions {
			return nil, fmt.Errorf("compiled nodes exceed maximum %d", execution.MaxExpandedExecutions)
		}
		path := hierarchy + " / " + step.DisplayName
		runtimeID := runtimeInvocationStepID(invocationPath, step.ID)
		c.metadata[runtimeID] = StepMetadata{FlowFragmentStepID: step.ID, DisplayName: step.DisplayName,
			Kind: string(step.Kind), HierarchyPath: path, ElementTargetID: step.ElementTargetID,
			ElementTargetVersionID: step.ElementTargetVersionID, CaptureScreenshot: step.CaptureScreenshot,
			InvocationPath: scopePath}

		var compiled node.Node
		var err error
		switch step.Kind {
		case execution.ActionStep:
			var target fingerprint.ElementTargetSpec
			if step.ElementTargetID != "" {
				target, err = c.spec(step.ElementTargetID, step.ElementTargetVersionID)
				if err != nil {
					return nil, err
				}
			}
			compiled = &node.StepNode{NodeID: runtimeID, Target: target,
				Action: node.Action{Kind: node.ActionKind(step.Action), Value: step.Value,
					Values: append([]string(nil), step.Values...)}, Optional: step.Optional}
		case execution.ValidationStep:
			compiled, err = c.compileValidation(runtimeID, step, "", "", nil)
		case execution.ValidationGroupStep:
			compiled, err = c.compileValidationGroup(invocationPath, runtimeID, path, step, scopePath)
		case execution.WaitStep:
			compiled, err = c.compileWait(runtimeID, step)
		case execution.RepeatStep:
			var children []node.Node
			children, err = c.compileSteps(parentVersionID, invocationPath, scopePath, step.Children, path, depth)
			compiled = &node.RepeatNode{NodeID: runtimeID, Times: step.RepeatCount, Children: children}
		case execution.FlowFragmentReference:
			compiled, err = c.compileWorkflowCall(parentVersionID, invocationPath, scopePath, runtimeID, step, depth)
		default:
			err = fmt.Errorf("step %s has unsupported kind %q", step.ID, step.Kind)
		}
		if err != nil {
			return nil, err
		}
		result = append(result, compiled)
	}
	return result, nil
}

// compileValidation 将验证步骤及其断言、等待和稳定性配置编译为验证节点。
func (c *executionCompiler) compileValidation(runtimeID string, step execution.Step,
	groupID, branchID string, inherited *execution.Validation) (*node.ValidationNode, error) {
	if step.Validation == nil {
		return nil, fmt.Errorf("validation step %s has no configuration", step.ID)
	}
	target, err := c.spec(step.ElementTargetID, step.ElementTargetVersionID)
	if err != nil {
		return nil, err
	}
	wait := *step.Validation
	if inherited != nil {
		wait = *inherited
	}
	assertion := step.Validation
	return &node.ValidationNode{NodeID: runtimeID, GroupID: groupID, BranchID: branchID, Target: target,
		Assertion: node.ValidationAssertion{Kind: string(assertion.Kind), Expected: assertion.Expected,
			ExpectedValues: append([]string(nil), assertion.ExpectedValues...), Attribute: assertion.Attribute,
			IgnoreCase: assertion.IgnoreCase}, MaxWait: time.Duration(wait.MaxWaitMS) * time.Millisecond,
		Stability: time.Duration(wait.StabilityMS) * time.Millisecond}, nil
}

// compileValidationGroup 将验证分组及其分支成员编译为验证组节点，并继承组级等待配置。
func (c *executionCompiler) compileValidationGroup(invocationPath, runtimeID, path string,
	step execution.Step, scopePath execution.InvocationPath) (*node.ValidationGroupNode, error) {
	if step.ValidationGroup == nil {
		return nil, fmt.Errorf("validation group %s has no configuration", step.ID)
	}
	group := step.ValidationGroup
	branches := make([]node.ValidationBranch, 0, len(group.Branches))
	for _, branch := range group.Branches {
		members := make([]*node.ValidationNode, 0, len(branch.Steps))
		for _, member := range branch.Steps {
			*c.compiledNodes++
			if *c.compiledNodes > execution.MaxExpandedExecutions {
				return nil, fmt.Errorf("compiled node count exceeds maximum %d", execution.MaxExpandedExecutions)
			}
			memberRuntimeID := runtimeInvocationStepID(invocationPath, member.ID)
			c.metadata[memberRuntimeID] = StepMetadata{FlowFragmentStepID: member.ID,
				DisplayName: member.DisplayName, Kind: string(member.Kind),
				HierarchyPath:   path + " / " + branch.Name + " / " + member.DisplayName,
				ElementTargetID: member.ElementTargetID, ElementTargetVersionID: member.ElementTargetVersionID,
				InvocationPath: scopePath}
			validation, err := c.compileValidation(memberRuntimeID, member, runtimeID, branch.ID, &execution.Validation{MaxWaitMS: group.MaxWaitMS, StabilityMS: group.StabilityMS})
			if err != nil {
				return nil, err
			}
			members = append(members, validation)
		}
		branches = append(branches, node.ValidationBranch{ID: branch.ID, Nodes: members})
	}
	return &node.ValidationGroupNode{NodeID: runtimeID, Branches: branches,
		MaxWait:   time.Duration(group.MaxWaitMS) * time.Millisecond,
		Stability: time.Duration(group.StabilityMS) * time.Millisecond}, nil
}

// compileWait 将等待步骤转换为睡眠、元素、可见性或网络空闲等待节点。
func (c *executionCompiler) compileWait(runtimeID string, step execution.Step) (node.Node, error) {
	duration, err := millisecondsDuration(int64(step.WaitMS))
	if err != nil {
		return nil, fmt.Errorf("wait step %s: %w", step.ID, err)
	}
	switch step.WaitKind {
	case "", "sleep":
		return &node.WaitNode{NodeID: runtimeID, Kind: node.WaitSleep,
			Duration: duration}, nil
	case "element":
		target, err := c.spec(step.ElementTargetID, step.ElementTargetVersionID)
		if err != nil {
			return nil, err
		}
		return &node.WaitNode{NodeID: runtimeID, Kind: node.WaitElement, Target: target,
			Timeout: duration}, nil
	case "element_visible", "element_invisible":
		target, err := c.spec(step.ElementTargetID, step.ElementTargetVersionID)
		if err != nil {
			return nil, err
		}
		kind := node.WaitElementVisible
		if step.WaitKind == "element_invisible" {
			kind = node.WaitElementInvisible
		}
		return &node.WaitNode{NodeID: runtimeID, Kind: kind, Target: target,
			Timeout: duration}, nil
	case "network_idle":
		return &node.WaitNode{NodeID: runtimeID, Kind: node.WaitNetworkIdle,
			Timeout: duration}, nil
	default:
		return nil, fmt.Errorf("wait step %s has unsupported kind %q", step.ID, step.WaitKind)
	}
}

// compileWorkflowCall 解析锁定的工作流版本和具体调用范围，并编译带绑定值与约束的调用节点。
func (c *executionCompiler) compileWorkflowCall(parentVersionID, invocationPath string, scopePath execution.InvocationPath, runtimeID string, step execution.Step, depth int) (node.Node, error) {
	if step.Reference == nil {
		return nil, fmt.Errorf("workflow reference step %s has no reference", step.ID)
	}
	resolution, ok := c.resolutions[referenceKey(parentVersionID, step.ID)]
	if !ok {
		return nil, fmt.Errorf("workflow reference step %s has no locked version resolution", step.ID)
	}
	childDependency, ok := c.versions[resolution.WorkflowVersionID]
	if !ok {
		return nil, fmt.Errorf("workflow reference step %s resolved version %s is missing", step.ID, resolution.WorkflowVersionID)
	}
	childWorkflowID := childDependency.FlowFragmentID
	if childDependency.ID != "" {
		childWorkflowID = childDependency.ID
	}
	if childWorkflowID != step.Reference.FlowFragmentID || resolution.FlowFragmentID != step.Reference.FlowFragmentID {
		return nil, fmt.Errorf("workflow reference step %s resolution does not match workflow %s", step.ID, step.Reference.FlowFragmentID)
	}
	childPath := invocationPath + encodeRuntimeComponent(step.ID) + encodeRuntimeComponent(resolution.WorkflowVersionID)
	concrete, exists := c.invocations[execution.InvocationEdgeKey{ParentPath: scopePath, StepID: step.ID}]
	if !exists {
		return nil, fmt.Errorf("workflow reference step %s has no concrete invocation", step.ID)
	}
	childScopePath := concrete.Path
	target, err := c.compileWorkflow(resolution.WorkflowVersionID, childPath, childScopePath, depth+1)
	if err != nil {
		return nil, err
	}
	bindings := make(map[string]parameter.Binding, len(concrete.Bindings))
	for name, binding := range concrete.Bindings {
		bindings[name] = binding.Clone()
	}
	constraints := make(map[string]parameter.Constraint, len(childDependency.Parameters))
	for _, definition := range childDependency.Parameters {
		constraints[definition.Name] = parameter.Constraint{Type: definition.Type, Options: append([]string(nil), definition.Options...)}
	}
	return &node.WorkflowCallNode{NodeID: runtimeID, Target: target, Bindings: bindings, Values: cloneParameterValues(concrete.Values), Constraints: constraints}, nil
}

// invocationIndex 将非根调用范围按父调用路径和步骤 ID 建立查找索引。
func invocationIndex(values []execution.InvocationScopeSnapshot) map[execution.InvocationEdgeKey]execution.InvocationScopeSnapshot {
	result := make(map[execution.InvocationEdgeKey]execution.InvocationScopeSnapshot, len(values))
	for _, value := range values {
		if value.ParentPath != (execution.InvocationPath{}) {
			result[execution.InvocationEdgeKey{ParentPath: value.ParentPath, StepID: value.StepID}] = value
		}
	}
	return result
}

// cloneParameterValues 深拷贝参数值映射；nil 输入仍返回 nil。
func cloneParameterValues(source map[string]parameter.Value) map[string]parameter.Value {
	if source == nil {
		return nil
	}
	result := make(map[string]parameter.Value, len(source))
	for name, value := range source {
		result[name] = value.Clone()
	}
	return result
}

// spec 从快照节点依赖构造并缓存元素目标规格，同时校验版本 ID 未映射到不同稳定节点。
func (c *executionCompiler) spec(nodeID, versionID string) (fingerprint.ElementTargetSpec, error) {
	identity := nodeDependencyIdentity(nodeID, versionID)
	dependency, ok := c.nodes[identity]
	if !ok {
		return fingerprint.ElementTargetSpec{}, fmt.Errorf("node %s version %s is missing from the instance snapshot", nodeID, versionID)
	}
	if existing, ok := c.programSpecs[versionID]; ok {
		mapped := c.runtimeNodes[versionID]
		if mapped.ElementTargetID != nodeID {
			return fingerprint.ElementTargetSpec{}, fmt.Errorf("node version %s is shared by different stable nodes", versionID)
		}
		return existing, nil
	}
	version := dependency
	fp := version.Fingerprint
	spec := fingerprint.ElementTargetSpec{UUID: dependency.ElementTargetID, ID: version.VersionID,
		PageURL: version.PageURL, Origin: version.Origin, Role: fp.ARIA.Role,
		Selectors:   append([]fingerprint.Selector(nil), version.Selectors...),
		Fingerprint: fp.Clone()}
	if err := spec.Validate(); err != nil {
		// spec.Validate 已返回带有有序违规明细的 FINGERPRINT_ELEMENT_TARGET_SPEC_INVALID；此处保持原错误，
		// 不在外层回显两个身份。
		return fingerprint.ElementTargetSpec{}, err
	}
	c.programSpecs[versionID] = spec
	c.runtimeNodes[versionID] = RuntimeNodeIdentity{ElementTargetID: nodeID, ElementTargetVersionID: versionID}
	return spec, nil
}

// millisecondsDuration 将毫秒转换为 time.Duration，并拒绝负值或超出可表示范围的输入。
func millisecondsDuration(milliseconds int64) (time.Duration, error) {
	const maxMilliseconds = int64(^uint64(0)>>1) / int64(time.Millisecond)
	if milliseconds < 0 || milliseconds > maxMilliseconds {
		return 0, fmt.Errorf("duration milliseconds %d is out of range", milliseconds)
	}
	return time.Duration(milliseconds) * time.Millisecond, nil
}

// impossibleCompilerState 构造编译器内部不变量被破坏时的错误。
func impossibleCompilerState(format string, args ...any) error {
	return fmt.Errorf("compiler reached impossible state: "+format, args...)
}

// nodeDependencyIdentity 构造元素目标节点及其版本的快照依赖键。
func nodeDependencyIdentity(nodeID, versionID string) execution.NodeDependencyKey {
	return execution.NodeDependencyKey{ElementTargetID: nodeID, VersionID: versionID}
}

// runtimeFlowFragmentStepID 构造工作流版本步骤在运行时的稳定 ID。
func runtimeFlowFragmentStepID(workflowVersionID, flowFragmentStepID string) string {
	return runtimeInvocationStepID(encodeRuntimeComponent(workflowVersionID), flowFragmentStepID)
}

// runtimeInvocationStepID 构造调用路径下步骤的长度编码运行时 ID。
func runtimeInvocationStepID(invocationPath, flowFragmentStepID string) string {
	return "step|" + invocationPath + encodeRuntimeComponent(flowFragmentStepID)
}

// encodeRuntimeComponent 以长度前缀编码运行时 ID 组件，保持组件边界无歧义。
func encodeRuntimeComponent(value string) string { return fmt.Sprintf("%d:%s", len(value), value) }

// referenceKey 构造工作流父版本与步骤之间的引用解析键。
func referenceKey(parentVersionID, stepID string) execution.WorkflowReferenceKey {
	return execution.WorkflowReferenceKey{ParentVersionID: parentVersionID, StepID: stepID}
}
