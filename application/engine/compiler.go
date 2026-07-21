package engine

import (
	"fmt"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/node"
	workspace "github.com/Capsule7446/healix-core/domain/workspace"
)

// StepMetadata 将执行树 ID 映射回拥有证据、截图和面向用户进度信息的
// 不可变工作区步骤。
type StepMetadata struct {
	WorkflowStepID    string
	DisplayName       string
	Kind              string
	HierarchyPath     string
	NodeID            string
	NodeVersionID     string
	CaptureScreenshot bool
}

// RuntimeNodeIdentity 将运行时 NodeSpec ID（即精确的 NodeVersion ID）映射到
// 其稳定的工作区 Node 标识。
type RuntimeNodeIdentity struct {
	NodeID        string
	NodeVersionID string
}

// CompiledExecution 是供 RunProgram 和宿主证据适配器使用的内存执行产物。
type CompiledExecution struct {
	Program      node.Program
	Metadata     map[string]StepMetadata
	RuntimeNodes map[string]RuntimeNodeIdentity
}

// CompileExecution 直接从 TestTaskRun 快照编译一个已锁定的 WorkflowExecution。
// 它不执行任何文件系统或序列化操作。
func CompileExecution(plan workspace.TestTaskRunPlan, execution workspace.WorkflowExecutionPlan) (CompiledExecution, error) {
	compiler := executionCompiler{
		versions:     make(map[string]workspace.WorkflowDependencySnapshot, len(plan.Workflows)),
		resolutions:  make(map[string]workspace.WorkflowReferenceResolution, len(plan.References)),
		nodes:        make(map[string]workspace.NodeDependencySnapshot, len(plan.Nodes)),
		programSpecs: make(map[string]fingerprint.NodeSpec),
		metadata:     make(map[string]StepMetadata),
		runtimeNodes: make(map[string]RuntimeNodeIdentity),
		workflows:    make(map[string]*node.WorkflowNode),
		state:        make(map[string]compileState),
	}
	for _, dependency := range plan.Workflows {
		if dependency.Version.ID == "" {
			return CompiledExecution{}, fmt.Errorf("run snapshot contains a workflow with an empty version id")
		}
		if _, exists := compiler.versions[dependency.Version.ID]; exists {
			return CompiledExecution{}, fmt.Errorf("run snapshot contains duplicate workflow version %s", dependency.Version.ID)
		}
		compiler.versions[dependency.Version.ID] = dependency
	}
	for _, resolution := range plan.References {
		key := referenceKey(resolution.ParentWorkflowVersionID, resolution.StepID)
		if _, exists := compiler.resolutions[key]; exists {
			return CompiledExecution{}, fmt.Errorf("run snapshot contains duplicate workflow resolution %s", key)
		}
		compiler.resolutions[key] = resolution
	}
	for _, dependency := range plan.Nodes {
		if dependency.Node.ID == "" || dependency.Version.ID == "" || dependency.Version.NodeID != dependency.Node.ID {
			return CompiledExecution{}, fmt.Errorf("node version %s does not belong to node %s", dependency.Version.ID, dependency.Node.ID)
		}
		identity := workspace.NodeDependencyIdentity(dependency.Node.ID, dependency.Version.ID)
		if _, exists := compiler.nodes[identity]; exists {
			return CompiledExecution{}, fmt.Errorf("run snapshot contains duplicate node dependency %s", dependency.Version.ID)
		}
		compiler.nodes[identity] = dependency
	}

	root, err := compiler.compileWorkflow(execution.WorkflowVersionID)
	if err != nil {
		return CompiledExecution{}, fmt.Errorf("compile execution %s: %w", execution.ID, err)
	}
	return CompiledExecution{
		Program:  node.Program{Root: root, Specs: compiler.programSpecs},
		Metadata: compiler.metadata, RuntimeNodes: compiler.runtimeNodes,
	}, nil
}

type compileState uint8

const (
	compileVisiting compileState = iota + 1
	compileDone
)

type executionCompiler struct {
	versions     map[string]workspace.WorkflowDependencySnapshot
	resolutions  map[string]workspace.WorkflowReferenceResolution
	nodes        map[string]workspace.NodeDependencySnapshot
	programSpecs map[string]fingerprint.NodeSpec
	metadata     map[string]StepMetadata
	runtimeNodes map[string]RuntimeNodeIdentity
	workflows    map[string]*node.WorkflowNode
	state        map[string]compileState
}

func (c *executionCompiler) compileWorkflow(versionID string) (*node.WorkflowNode, error) {
	if c.state[versionID] == compileVisiting {
		return nil, fmt.Errorf("workflow reference cycle includes version %s", versionID)
	}
	if c.state[versionID] == compileDone {
		return c.workflows[versionID], nil
	}
	dependency, ok := c.versions[versionID]
	if !ok {
		return nil, fmt.Errorf("workflow version %s is missing from the run snapshot", versionID)
	}
	if dependency.Version.WorkflowID != dependency.Workflow.ID {
		return nil, fmt.Errorf("workflow version %s does not belong to workflow %s", versionID, dependency.Workflow.ID)
	}
	if err := dependency.Version.ValidateFor(dependency.Workflow); err != nil {
		return nil, fmt.Errorf("workflow version %s failed execution preflight: %w", versionID, err)
	}
	c.state[versionID] = compileVisiting
	c.metadata[versionID] = StepMetadata{WorkflowStepID: versionID, DisplayName: dependency.Workflow.DisplayName,
		Kind: "WORKFLOW", HierarchyPath: dependency.Workflow.DisplayName}
	children, err := c.compileSteps(versionID, dependency.Version.Definition.Steps, dependency.Workflow.DisplayName)
	if err != nil {
		delete(c.state, versionID)
		return nil, err
	}
	workflow := &node.WorkflowNode{NodeID: versionID, Children: children}
	c.workflows[versionID] = workflow
	c.state[versionID] = compileDone
	return workflow, nil
}

func (c *executionCompiler) compileSteps(parentVersionID string, steps []workspace.WorkflowStep, hierarchy string) ([]node.Node, error) {
	result := make([]node.Node, 0, len(steps))
	for _, step := range steps {
		path := hierarchy + " / " + step.DisplayName
		runtimeID := runtimeWorkflowStepID(parentVersionID, step.ID)
		c.metadata[runtimeID] = StepMetadata{WorkflowStepID: step.ID, DisplayName: step.DisplayName,
			Kind: string(step.Kind), HierarchyPath: path, NodeID: step.NodeID,
			NodeVersionID: step.NodeVersionID, CaptureScreenshot: step.CaptureScreenshot}

		var compiled node.Node
		var err error
		switch step.Kind {
		case workspace.StepAction:
			var target fingerprint.NodeSpec
			if step.NodeID != "" {
				target, err = c.spec(step.NodeID, step.NodeVersionID)
				if err != nil {
					return nil, fmt.Errorf("step %s: %w", step.ID, err)
				}
			}
			compiled = &node.StepNode{NodeID: runtimeID, Target: target,
				Action: node.Action{Kind: node.ActionKind(step.Action), Value: step.Value,
					Values: append([]string(nil), step.Values...)}, Optional: step.Optional}
		case workspace.StepValidation:
			compiled, err = c.compileValidation(runtimeID, step, "", "", nil)
		case workspace.StepValidationGroup:
			compiled, err = c.compileValidationGroup(parentVersionID, runtimeID, path, step)
		case workspace.StepWait:
			compiled, err = c.compileWait(runtimeID, step)
		case workspace.StepRepeat:
			var children []node.Node
			children, err = c.compileSteps(parentVersionID, step.Children, path)
			compiled = &node.RepeatNode{NodeID: runtimeID, Times: step.RepeatCount, Children: children}
		case workspace.StepWorkflowRef:
			compiled, err = c.compileWorkflowCall(parentVersionID, runtimeID, step)
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

func (c *executionCompiler) compileValidation(runtimeID string, step workspace.WorkflowStep,
	groupID, branchID string, inherited *workspace.ValidationWait) (*node.ValidationNode, error) {
	if step.Validation == nil {
		return nil, fmt.Errorf("validation step %s has no configuration", step.ID)
	}
	target, err := c.spec(step.NodeID, step.NodeVersionID)
	if err != nil {
		return nil, fmt.Errorf("validation step %s: %w", step.ID, err)
	}
	wait := step.Validation.Wait
	if inherited != nil {
		wait = *inherited
	}
	assertion := step.Validation.Assertion
	return &node.ValidationNode{NodeID: runtimeID, GroupID: groupID, BranchID: branchID, Target: target,
		Assertion: node.ValidationAssertion{Kind: string(assertion.Kind), Expected: assertion.Expected,
			ExpectedValues: append([]string(nil), assertion.ExpectedValues...), Attribute: assertion.Attribute,
			IgnoreCase: assertion.IgnoreCase}, MaxWait: time.Duration(wait.MaxWaitMS) * time.Millisecond,
		Stability: time.Duration(wait.StabilityMS) * time.Millisecond}, nil
}

func (c *executionCompiler) compileValidationGroup(parentVersionID, runtimeID, path string,
	step workspace.WorkflowStep) (*node.ValidationGroupNode, error) {
	if step.ValidationGroup == nil {
		return nil, fmt.Errorf("validation group %s has no configuration", step.ID)
	}
	group := step.ValidationGroup
	branches := make([]node.ValidationBranch, 0, len(group.Branches))
	for _, branch := range group.Branches {
		members := make([]*node.ValidationNode, 0, len(branch.Steps))
		for _, member := range branch.Steps {
			memberRuntimeID := runtimeWorkflowStepID(parentVersionID, member.ID)
			c.metadata[memberRuntimeID] = StepMetadata{WorkflowStepID: member.ID,
				DisplayName: member.DisplayName, Kind: string(member.Kind),
				HierarchyPath: path + " / " + branch.Name + " / " + member.DisplayName,
				NodeID:        member.NodeID, NodeVersionID: member.NodeVersionID}
			validation, err := c.compileValidation(memberRuntimeID, member, runtimeID, branch.ID, &group.Wait)
			if err != nil {
				return nil, err
			}
			members = append(members, validation)
		}
		branches = append(branches, node.ValidationBranch{ID: branch.ID, Nodes: members})
	}
	return &node.ValidationGroupNode{NodeID: runtimeID, Branches: branches,
		MaxWait:   time.Duration(group.Wait.MaxWaitMS) * time.Millisecond,
		Stability: time.Duration(group.Wait.StabilityMS) * time.Millisecond}, nil
}

func (c *executionCompiler) compileWait(runtimeID string, step workspace.WorkflowStep) (node.Node, error) {
	switch step.WaitKind {
	case "", "sleep":
		return &node.WaitNode{NodeID: runtimeID, Kind: node.WaitSleep,
			Duration: time.Duration(step.WaitMS) * time.Millisecond}, nil
	case "element":
		target, err := c.spec(step.NodeID, step.NodeVersionID)
		if err != nil {
			return nil, fmt.Errorf("wait step %s: %w", step.ID, err)
		}
		return &node.WaitNode{NodeID: runtimeID, Kind: node.WaitElement, Target: target,
			Timeout: time.Duration(step.WaitMS) * time.Millisecond}, nil
	case "element_visible", "element_invisible":
		target, err := c.spec(step.NodeID, step.NodeVersionID)
		if err != nil {
			return nil, fmt.Errorf("wait step %s: %w", step.ID, err)
		}
		kind := node.WaitElementVisible
		if step.WaitKind == "element_invisible" {
			kind = node.WaitElementInvisible
		}
		return &node.WaitNode{NodeID: runtimeID, Kind: kind, Target: target,
			Timeout: time.Duration(step.WaitMS) * time.Millisecond}, nil
	case "network_idle":
		return &node.WaitNode{NodeID: runtimeID, Kind: node.WaitNetworkIdle,
			Timeout: time.Duration(step.WaitMS) * time.Millisecond}, nil
	default:
		return nil, fmt.Errorf("wait step %s has unsupported kind %q", step.ID, step.WaitKind)
	}
}

func (c *executionCompiler) compileWorkflowCall(parentVersionID, runtimeID string, step workspace.WorkflowStep) (node.Node, error) {
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
	if childDependency.Workflow.ID != step.Reference.WorkflowID || resolution.WorkflowID != step.Reference.WorkflowID {
		return nil, fmt.Errorf("workflow reference step %s resolution does not match workflow %s", step.ID, step.Reference.WorkflowID)
	}
	target, err := c.compileWorkflow(resolution.WorkflowVersionID)
	if err != nil {
		return nil, err
	}
	bindings := cloneStrings(step.Reference.ParameterBindings)
	for _, parameter := range childDependency.Version.Definition.Parameters {
		if _, exists := bindings[parameter.Name]; !exists {
			bindings[parameter.Name] = parameter.DefaultValue
		}
	}
	return &node.WorkflowCallNode{NodeID: runtimeID, Target: target, Bindings: bindings}, nil
}

func (c *executionCompiler) spec(nodeID, versionID string) (fingerprint.NodeSpec, error) {
	identity := workspace.NodeDependencyIdentity(nodeID, versionID)
	dependency, ok := c.nodes[identity]
	if !ok {
		return fingerprint.NodeSpec{}, fmt.Errorf("node %s version %s is missing from the run snapshot", nodeID, versionID)
	}
	if existing, ok := c.programSpecs[versionID]; ok {
		mapped := c.runtimeNodes[versionID]
		if mapped.NodeID != nodeID {
			return fingerprint.NodeSpec{}, fmt.Errorf("node version %s is shared by different stable nodes", versionID)
		}
		return existing, nil
	}
	version := dependency.Version
	fp := version.Fingerprint
	spec := fingerprint.NodeSpec{UUID: dependency.Node.ID, ID: version.ID,
		PageURL: version.PageURL, Origin: version.Origin, Role: fp.ARIA.Role,
		Selectors: append([]fingerprint.Selector(nil), version.Selectors...),
		Fingerprint: fingerprint.Fingerprint{Tag: fp.Tag, Attributes: cloneStrings(fp.Attributes), Text: fp.Text,
			ARIA: fp.ARIA, Path: append([]string(nil), fp.Path...), SiblingIndex: fp.SiblingIndex,
			Neighbors: fp.Neighbors, LabelText: fp.LabelText, FormID: fp.FormID, Framework: fp.Framework.Clone()}}
	if err := spec.Validate(); err != nil {
		return fingerprint.NodeSpec{}, fmt.Errorf("node %s version %s: %w", nodeID, versionID, err)
	}
	c.programSpecs[versionID] = spec
	c.runtimeNodes[versionID] = RuntimeNodeIdentity{NodeID: nodeID, NodeVersionID: versionID}
	return spec, nil
}

func runtimeWorkflowStepID(workflowVersionID, workflowStepID string) string {
	return workflowVersionID + "::" + workflowStepID
}

func referenceKey(parentVersionID, stepID string) string { return parentVersionID + ":" + stepID }

func cloneStrings(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
