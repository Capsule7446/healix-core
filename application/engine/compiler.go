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
}

// RuntimeNodeIdentity 将运行时 ElementTargetSpec ID（即精确的 ElementTargetVersion ID）映射到
// 其稳定的工作区 ElementTarget 标识。
type RuntimeNodeIdentity struct {
	ElementTargetID        string
	ElementTargetVersionID string
}

type compiledExecutionIdentity struct {
	instanceID     execution.InstanceID
	snapshotDigest string
	entryID        execution.EntryID
}

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

type CompiledPlan struct {
	entries []CompiledEntry
	byID    map[execution.EntryID]int
}

// Entries returns the compiled entries in execution order. The returned slice
// and each entry's exported maps are owned by the caller.
func (r CompiledPlan) Entries() []CompiledEntry {
	entries := make([]CompiledEntry, len(r.entries))
	for index, entry := range r.entries {
		entries[index] = cloneCompiledEntry(entry)
	}
	return entries
}

// Entry returns the compiled entry identified by entryID without exposing
// the run's private lookup index.
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

func (entry CompiledEntry) hasIdentity(entryID execution.EntryID) bool {
	return entryID.Validate() == nil &&
		entry.InstanceID.Validate() == nil && entry.InstanceID == entry.identity.instanceID &&
		entry.SnapshotDigest != "" && entry.SnapshotDigest == entry.identity.snapshotDigest &&
		entry.EntryID == entryID && entry.EntryID == entry.identity.entryID
}

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

// planUnsealedError reuses the code domain/execution already publishes for this
// exact condition, rather than minting a second one for the same meaning. The
// message must stay identical to that row, which the registry guard enforces.
func planUnsealedError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, execution.CodePlanUnsealed, "execution plan must be sealed")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// CompilePlan compiles solely from the immutable run snapshot payload. Every
// returned entry is bound to the snapshot's Run, digest, and Execution ID.
func CompilePlan(snapshot execution.InstanceSnapshot) (CompiledPlan, error) {
	if snapshot.Digest() == "" {
		return CompiledPlan{}, planUnsealedError()
	}
	return compileSnapshotDraft(snapshot.Plan(), snapshot)
}

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
			// The inner failure is already a classified fault. This wrapper both hid
			// that classification and welded the execution id into public text.
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
				// No execution id or parameter name in the message: neither is a code
				// this batch owns (EXECUTION_CREATE_INSTANCE_PLAN_INVALID belongs to a
				// parallel domain/execution migration), so this stays an uncoded error
				// with the identities dropped rather than echoed. The integrator should
				// route this through that code once it lands.
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
		Kind: "WORKFLOW", HierarchyPath: dependency.DisplayName}
	children, err := c.compileSteps(versionID, invocationPath, scopePath, dependency.Steps, dependency.DisplayName, depth)
	if err != nil {
		return nil, err
	}
	return &node.WorkflowNode{NodeID: workflowRuntimeID, Children: children}, nil
}

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
			ElementTargetVersionID: step.ElementTargetVersionID, CaptureScreenshot: step.CaptureScreenshot}

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
			compiled, err = c.compileValidationGroup(invocationPath, runtimeID, path, step)
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

func (c *executionCompiler) compileValidationGroup(invocationPath, runtimeID, path string,
	step execution.Step) (*node.ValidationGroupNode, error) {
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
				ElementTargetID: member.ElementTargetID, ElementTargetVersionID: member.ElementTargetVersionID}
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

func invocationIndex(values []execution.InvocationScopeSnapshot) map[execution.InvocationEdgeKey]execution.InvocationScopeSnapshot {
	result := make(map[execution.InvocationEdgeKey]execution.InvocationScopeSnapshot, len(values))
	for _, value := range values {
		if value.ParentPath != (execution.InvocationPath{}) {
			result[execution.InvocationEdgeKey{ParentPath: value.ParentPath, StepID: value.StepID}] = value
		}
	}
	return result
}

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
		Selectors: append([]fingerprint.Selector(nil), version.Selectors...),
		Fingerprint: fingerprint.Fingerprint{Tag: fp.Tag, Attributes: cloneStrings(fp.Attributes), Text: fp.Text,
			ARIA: fp.ARIA, Path: append([]string(nil), fp.Path...), SiblingIndex: fp.SiblingIndex,
			Neighbors: fp.Neighbors, LabelText: fp.LabelText, FormID: fp.FormID, Framework: fp.Framework.Clone()}}
	if err := spec.Validate(); err != nil {
		// spec.Validate returns FINGERPRINT_ELEMENT_TARGET_SPEC_INVALID with its own
		// ordered violations; this wrapper hid it behind two echoed identities.
		return fingerprint.ElementTargetSpec{}, err
	}
	c.programSpecs[versionID] = spec
	c.runtimeNodes[versionID] = RuntimeNodeIdentity{ElementTargetID: nodeID, ElementTargetVersionID: versionID}
	return spec, nil
}

func millisecondsDuration(milliseconds int64) (time.Duration, error) {
	const maxMilliseconds = int64(^uint64(0)>>1) / int64(time.Millisecond)
	if milliseconds < 0 || milliseconds > maxMilliseconds {
		return 0, fmt.Errorf("duration milliseconds %d is out of range", milliseconds)
	}
	return time.Duration(milliseconds) * time.Millisecond, nil
}

func impossibleCompilerState(format string, args ...any) error {
	return fmt.Errorf("compiler reached impossible state: "+format, args...)
}

func nodeDependencyIdentity(nodeID, versionID string) execution.NodeDependencyKey {
	return execution.NodeDependencyKey{ElementTargetID: nodeID, VersionID: versionID}
}

func runtimeFlowFragmentStepID(workflowVersionID, flowFragmentStepID string) string {
	return runtimeInvocationStepID(encodeRuntimeComponent(workflowVersionID), flowFragmentStepID)
}

func runtimeInvocationStepID(invocationPath, flowFragmentStepID string) string {
	return "step|" + invocationPath + encodeRuntimeComponent(flowFragmentStepID)
}

func encodeRuntimeComponent(value string) string { return fmt.Sprintf("%d:%s", len(value), value) }

func referenceKey(parentVersionID, stepID string) execution.WorkflowReferenceKey {
	return execution.WorkflowReferenceKey{ParentVersionID: parentVersionID, StepID: stepID}
}

func cloneStrings(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
