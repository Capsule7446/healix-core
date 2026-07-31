package execution

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

const (
	MaxStepNestingDepth            = 64
	MaxVisitedSteps                = 10_000
	MaxWorkflowReferenceDepth      = 32
	MaxReachableWorkflows          = 1_000
	MaxWorkflowReferenceEdges      = 5_000
	MaxRepeatCount                 = 1_000
	MaxWaitMS                      = 60_000
	MaxExpandedExecutions          = 1_000_000
	MaxCumulativeWaitMS            = 86_400_000
	MaxDraftWorkflows              = 1_000
	MaxDraftNodes                  = 10_000
	MaxDraftReferences             = 5_000
	MaxAggregateSteps              = 10_000
	MaxAggregateParameters         = 10_000
	MaxAggregateSelectors          = 50_000
	MaxAggregateFingerprintKV      = 100_000
	MaxAggregatePathSegments       = 100_000
	MaxAggregateBindings           = 100_000
	MaxAggregateCollectionElements = 100_000
	MaxAggregateStringBytes        = 16 * 1024 * 1024
	MaxStringBytes                 = 64 * 1024

	validationMinWaitMS      = 1_000
	validationMaxWaitMS      = 60_000
	validationMinStabilityMS = 200
	validationMaxStabilityMS = 5_000
	validationMaxBranches    = 5
	validationMaxBranchSteps = 10
	validationMaxGroupSteps  = 20
)

func (p Parameter) Validate() error {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.DisplayName) == "" {
		return errors.New("parameter name and display name are required")
	}
	switch p.Type {
	case parameter.Text, parameter.Number, parameter.Boolean:
		if len(p.Options) != 0 {
			return errors.New("non-select parameter cannot declare options")
		}
	case parameter.SingleSelect, parameter.MultiSelect:
		if len(p.Options) == 0 {
			return errors.New("select parameter requires options")
		}
		seen := map[string]struct{}{}
		for _, option := range p.Options {
			if strings.TrimSpace(option) == "" {
				return errors.New("select option cannot be blank")
			}
			if _, exists := seen[option]; exists {
				return errors.New("duplicate select option")
			}
			seen[option] = struct{}{}
		}
	default:
		return fmt.Errorf("unsupported parameter type %q", p.Type)
	}
	value, present := p.Default.Value()
	if p.Required && present {
		return errors.New("required parameter cannot declare default")
	}
	if !p.Required && !present {
		return errors.New("optional parameter requires default")
	}
	if present {
		return p.validateValue(value)
	}
	return nil
}

func (p Parameter) validateValue(value parameter.Value) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Type() != p.Type {
		return errors.New("parameter value type mismatch")
	}
	allowed := func(candidate string) bool {
		for _, option := range p.Options {
			if option == candidate {
				return true
			}
		}
		return false
	}
	if p.Type == parameter.SingleSelect && !allowed(value.SingleSelect()) {
		return errors.New("single-select value is not an option")
	}
	if p.Type == parameter.MultiSelect {
		selectedValues := value.MultiSelect()
		if p.Required && len(selectedValues) == 0 {
			return errors.New("required multi-select value cannot be empty")
		}
		seen := map[string]struct{}{}
		for _, selected := range selectedValues {
			if !allowed(selected) {
				return errors.New("multi-select value is not an option")
			}
			if _, exists := seen[selected]; exists {
				return errors.New("duplicate multi-select value")
			}
			seen[selected] = struct{}{}
		}
	}
	return nil
}

func validateBindings(parent, child []Parameter, bindings map[string]parameter.Binding) error {
	parents := map[string]Parameter{}
	for _, definition := range parent {
		parents[definition.Name] = definition
	}
	values := map[string]parameter.Value{}
	children := make(map[string]Parameter, len(child))
	for _, definition := range child {
		children[definition.Name] = definition
	}
	for name, binding := range bindings {
		childDefinition, exists := children[name]
		if !exists {
			return fmt.Errorf("parameter %q is unknown", name)
		}
		if literal, ok := binding.Literal(); ok {
			values[name] = literal
			continue
		}
		parentName, ok := binding.ParentName()
		if !ok || parentName == "" {
			return fmt.Errorf("parameter %q has invalid binding", name)
		}
		parentDefinition, exists := parents[parentName]
		if !exists {
			return fmt.Errorf("parameter %q references missing parent parameter %q", name, parentName)
		}
		if parentDefinition.Type != childDefinition.Type {
			return fmt.Errorf("parameter %q parent reference type mismatch", name)
		}
		if childDefinition.Type == parameter.SingleSelect || childDefinition.Type == parameter.MultiSelect {
			for _, option := range parentDefinition.Options {
				if err := childDefinition.validateValue(selectProbe(childDefinition.Type, option)); err != nil {
					return fmt.Errorf("parameter %q parent option mismatch", name)
				}
			}
		}
	}
	for _, definition := range child {
		if _, exists := bindings[definition.Name]; !exists {
			if _, present := definition.Default.Value(); present {
				continue
			}
			return fmt.Errorf("parameter %q is missing", definition.Name)
		}
		if value, exists := values[definition.Name]; exists {
			if err := definition.validateValue(value); err != nil {
				return wrapOrPropagate(err, func(cause error) error {
					return fmt.Errorf("parameter %q: %w", definition.Name, cause)
				})
			}
		}
	}
	return nil
}
func selectProbe(kind parameter.Type, option string) parameter.Value {
	if kind == parameter.SingleSelect {
		return parameter.SingleSelectValue(option)
	}
	return parameter.MultiSelectValue([]string{option})
}

func validateSnapshotValues(definitions []Parameter, values map[string]parameter.Value) error {
	byName := make(map[string]Parameter, len(definitions))
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return wrapOrPropagate(err, func(cause error) error {
				return fmt.Errorf("parameter %q: %w", definition.Name, cause)
			})
		}
		if _, duplicate := byName[definition.Name]; duplicate {
			return fmt.Errorf("duplicate parameter %q", definition.Name)
		}
		byName[definition.Name] = definition
	}
	for name := range values {
		if _, exists := byName[name]; !exists {
			return fmt.Errorf("parameter %q is unknown", name)
		}
	}
	for _, definition := range definitions {
		value, exists := values[definition.Name]
		if !exists {
			return fmt.Errorf("parameter %q is missing", definition.Name)
		}
		if err := definition.validateValue(value); err != nil {
			return wrapOrPropagate(err, func(cause error) error {
				return fmt.Errorf("parameter %q: %w", definition.Name, cause)
			})
		}
	}
	return nil
}

// Validate classifies an execution plan's validation failure at this one
// exported boundary: an uncoded internal-invariant failure becomes
// EXECUTION_CREATE_INSTANCE_PLAN_INVALID with the bare detail retained only on
// the private cause, while a failure already classified by a workflow
// snapshot's own step-shape envelope, a node's fingerprint spec, or a
// parameter contract passes through unchanged.
func (p Draft) Validate() error {
	return classifyCreateInstancePlan(p.validateShape())
}

func (p Draft) validateShape() error {
	if err := validateAggregateInputBounds(p); err != nil {
		return err
	}
	if strings.TrimSpace(p.RunID) == "" {
		return errors.New("execution plan requires a run identity")
	}
	if !p.FailurePolicy.IsValid() {
		return fmt.Errorf("invalid failure policy %q", p.FailurePolicy)
	}
	if len(p.Entries) == 0 {
		return errors.New("execution plan requires at least one entry")
	}
	entryExecutionIDs := make(map[string]struct{}, len(p.Entries))
	entryItemIDs := make(map[string]struct{}, len(p.Entries))
	entrySequences := make(map[int]struct{}, len(p.Entries))
	for _, entry := range p.Entries {
		if entry.SequenceNumber < 1 || entry.SequenceNumber > len(p.Entries) {
			return fmt.Errorf("execution entry sequence %d is outside contiguous range 1..%d", entry.SequenceNumber, len(p.Entries))
		}
		if _, exists := entrySequences[entry.SequenceNumber]; exists {
			return fmt.Errorf("duplicate execution entry sequence %d", entry.SequenceNumber)
		}
		entrySequences[entry.SequenceNumber] = struct{}{}
		if strings.TrimSpace(entry.ExecutionID) == "" || strings.TrimSpace(entry.TestTaskItemID) == "" || strings.TrimSpace(entry.FlowFragmentID) == "" || strings.TrimSpace(entry.WorkflowVersionID) == "" {
			return errors.New("execution entry requires execution, test task item, workflow, and workflow version identities")
		}
		if _, exists := entryExecutionIDs[entry.ExecutionID]; exists {
			return fmt.Errorf("duplicate execution entry %q", entry.ExecutionID)
		}
		entryExecutionIDs[entry.ExecutionID] = struct{}{}
		if _, exists := entryItemIDs[entry.TestTaskItemID]; exists {
			return fmt.Errorf("duplicate test task item entry %q", entry.TestTaskItemID)
		}
		entryItemIDs[entry.TestTaskItemID] = struct{}{}
	}
	workflows := make(map[string]WorkflowSnapshot, len(p.Workflows))
	for _, workflow := range p.Workflows {
		if strings.TrimSpace(workflow.VersionID) == "" {
			return errors.New("run snapshot contains a workflow with an empty version id")
		}
		if _, exists := workflows[workflow.VersionID]; exists {
			return fmt.Errorf("execution plan contains duplicate workflow version %q", workflow.VersionID)
		}
		workflows[workflow.VersionID] = workflow
		if err := workflow.Validate(); err != nil {
			return err
		}
	}
	for _, entry := range p.Entries {
		workflow, exists := workflows[entry.WorkflowVersionID]
		if !exists {
			return fmt.Errorf("entry workflow version %q is missing", entry.WorkflowVersionID)
		}
		if workflow.FlowFragmentID != entry.FlowFragmentID {
			return fmt.Errorf("entry workflow version %q belongs to workflow %q, not %q", entry.WorkflowVersionID, workflow.FlowFragmentID, entry.FlowFragmentID)
		}
		if len(workflow.Parameters) == 0 {
			if entry.Parameters.ID != "" || entry.Parameters.SchemaVersion != 0 || entry.Parameters.WorkflowVersionID != "" || len(entry.Parameters.Values) != 0 {
				return fmt.Errorf("entry %q parameterless workflow requires an empty parameter snapshot", entry.ExecutionID)
			}
		} else {
			if strings.TrimSpace(entry.Parameters.ID) == "" || entry.Parameters.SchemaVersion < 1 || entry.Parameters.WorkflowVersionID != entry.WorkflowVersionID {
				return fmt.Errorf("entry %q parameter snapshot identity is invalid", entry.ExecutionID)
			}
			if err := validateSnapshotValues(workflow.Parameters, entry.Parameters.Values); err != nil {
				return wrapOrPropagate(err, func(cause error) error {
					return fmt.Errorf("entry %q parameter snapshot: %w", entry.ExecutionID, cause)
				})
			}
		}
	}
	nodes := make(map[NodeDependencyKey]struct{}, len(p.Nodes))
	versionOwners := make(map[string]string, len(p.Nodes))
	for _, snapshot := range p.Nodes {
		if strings.TrimSpace(snapshot.ElementTargetID) == "" || strings.TrimSpace(snapshot.VersionID) == "" {
			return errors.New("node dependency requires node and version identities")
		}
		spec := fingerprint.ElementTargetSpec{
			UUID: snapshot.ElementTargetID, ID: snapshot.VersionID, PageURL: snapshot.PageURL,
			Origin: snapshot.Origin, Role: snapshot.Fingerprint.ARIA.Role,
			Selectors: snapshot.Selectors, Fingerprint: snapshot.Fingerprint,
		}
		if err := spec.Validate(); err != nil {
			return err
		}
		if owner, exists := versionOwners[snapshot.VersionID]; exists && owner != snapshot.ElementTargetID {
			return fmt.Errorf("node version %q is owned by different nodes %q and %q", snapshot.VersionID, owner, snapshot.ElementTargetID)
		}
		versionOwners[snapshot.VersionID] = snapshot.ElementTargetID
		key := NodeDependencyKey{ElementTargetID: snapshot.ElementTargetID, VersionID: snapshot.VersionID}
		if _, exists := nodes[key]; exists {
			return fmt.Errorf("duplicate node dependency %q", key)
		}
		nodes[key] = struct{}{}
	}
	resolutions := make(map[WorkflowReferenceKey]ReferenceResolution, len(p.References))
	for _, resolution := range p.References {
		key := WorkflowReferenceKey{ParentVersionID: resolution.ParentVersionID, StepID: resolution.StepID}
		if _, exists := resolutions[key]; exists {
			return fmt.Errorf("duplicate workflow resolution %q", key)
		}
		resolutions[key] = resolution
	}
	roots := make([]string, len(p.Entries))
	for i, entry := range p.Entries {
		roots[i] = entry.WorkflowVersionID
	}
	if err := validateReachableWorkflowReferences(roots, workflows, resolutions); err != nil {
		return err
	}
	if err := validateExecutionBudget(roots, workflows, resolutions); err != nil {
		return err
	}
	for _, workflow := range p.Workflows {
		if err := validateDependencies(workflow, workflows, nodes, resolutions); err != nil {
			return err
		}
	}
	if len(resolutions) > 0 {
		keys := make([]WorkflowReferenceKey, 0, len(resolutions))
		for key := range resolutions {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].ParentVersionID != keys[j].ParentVersionID {
				return keys[i].ParentVersionID < keys[j].ParentVersionID
			}
			return keys[i].StepID < keys[j].StepID
		})
		return fmt.Errorf("workflow resolution %q does not belong to a reference step", keys[0])
	}
	return nil
}

func validateAggregateInputBounds(p Draft) error {
	type stepFrame struct {
		steps []Step
		depth int
	}
	if len(p.Workflows) > MaxDraftWorkflows || len(p.Nodes) > MaxDraftNodes || len(p.References) > MaxDraftReferences {
		return errors.New("execution plan aggregate collection limit exceeded")
	}
	steps, parameters, selectors, attributes, paths, bindings := 0, 0, 0, 0, 0, 0
	remainingCollectionElements := MaxAggregateCollectionElements
	addCollectionElements := func(count int) error {
		if count < 0 || count > remainingCollectionElements {
			return fmt.Errorf("aggregate collection elements exceed maximum %d", MaxAggregateCollectionElements)
		}
		remainingCollectionElements -= count
		return nil
	}
	for _, count := range []int{len(p.Entries), len(p.Workflows), len(p.Nodes), len(p.References)} {
		if err := addCollectionElements(count); err != nil {
			return err
		}
	}
	stringBytes := 0
	addString := func(value string) error {
		if len(value) > MaxStringBytes {
			return fmt.Errorf("execution plan string exceeds maximum %d bytes", MaxStringBytes)
		}
		if len(value) > MaxAggregateStringBytes-stringBytes {
			return fmt.Errorf("aggregate string bytes exceed maximum %d", MaxAggregateStringBytes)
		}
		stringBytes += len(value)
		return nil
	}
	addStrings := func(values ...string) error {
		for _, value := range values {
			if err := addString(value); err != nil {
				return err
			}
		}
		return nil
	}
	addParameterValue := func(value parameter.Value) error {
		switch value.Type() {
		case parameter.Text:
			return addString(value.Text())
		case parameter.Number:
			return addString(value.Number())
		case parameter.SingleSelect:
			return addString(value.SingleSelect())
		case parameter.Boolean:
			return nil
		case parameter.MultiSelect:
			items := value.MultiSelect()
			if err := addCollectionElements(len(items)); err != nil {
				return err
			}
			for _, item := range items {
				if err := addString(item); err != nil {
					return err
				}
			}
			return nil
		default:
			return value.Validate()
		}
	}
	if err := addString(p.RunID); err != nil {
		return err
	}
	for _, entry := range p.Entries {
		if err := addStrings(entry.ExecutionID, entry.TestTaskItemID, entry.FlowFragmentID, entry.WorkflowVersionID, entry.Parameters.ID, entry.Parameters.WorkflowVersionID); err != nil {
			return err
		}
		if err := addCollectionElements(len(entry.Parameters.Values)); err != nil {
			return err
		}
		for name, value := range entry.Parameters.Values {
			if err := addString(name); err != nil {
				return err
			}
			if err := addParameterValue(value); err != nil {
				return err
			}
		}
	}
	stack := make([]stepFrame, 0)
	for _, workflow := range p.Workflows {
		if err := addCollectionElements(len(workflow.Parameters) + len(workflow.Steps)); err != nil {
			return err
		}
		parameters += len(workflow.Parameters)
		if parameters > MaxAggregateParameters {
			return fmt.Errorf("aggregate parameters exceed maximum %d", MaxAggregateParameters)
		}
		if err := addStrings(workflow.ID, workflow.VersionID, workflow.FlowFragmentID, workflow.DisplayName); err != nil {
			return err
		}
		for _, parameter := range workflow.Parameters {
			if err := addCollectionElements(len(parameter.Options)); err != nil {
				return err
			}
			if err := addStrings(parameter.Name, parameter.DisplayName, parameter.Description, string(parameter.Type)); err != nil {
				return err
			}
			if value, present := parameter.Default.Value(); present {
				if err := addParameterValue(value); err != nil {
					return err
				}
			}
			for _, option := range parameter.Options {
				if err := addString(option); err != nil {
					return err
				}
			}
		}
		stack = append(stack, stepFrame{workflow.Steps, 1})
	}
	for len(stack) > 0 {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if frame.depth > MaxStepNestingDepth {
			return fmt.Errorf("step nesting depth exceeds maximum %d", MaxStepNestingDepth)
		}
		if len(frame.steps) > MaxAggregateSteps-steps {
			return fmt.Errorf("aggregate steps exceed maximum %d", MaxAggregateSteps)
		}
		steps += len(frame.steps)
		for i := range frame.steps {
			step := &frame.steps[i]
			if err := addStrings(step.ID, step.DisplayName, string(step.Kind), step.Action, step.ElementTargetID, step.ElementTargetVersionID, step.Value, step.WaitKind); err != nil {
				return err
			}
			if err := addCollectionElements(len(step.Values) + len(step.Children)); err != nil {
				return err
			}
			for _, value := range step.Values {
				if err := addString(value); err != nil {
					return err
				}
			}
			if step.Reference != nil {
				if err := addCollectionElements(len(step.Reference.ParameterBindings)); err != nil {
					return err
				}
				if err := addStrings(step.Reference.FlowFragmentID, step.Reference.WorkflowVersionID); err != nil {
					return err
				}
				if len(step.Reference.ParameterBindings) > MaxAggregateBindings-bindings {
					return fmt.Errorf("aggregate parameter bindings exceed maximum %d", MaxAggregateBindings)
				}
				bindings += len(step.Reference.ParameterBindings)
				for key, binding := range step.Reference.ParameterBindings {
					if err := addString(key); err != nil {
						return err
					}
					if literal, ok := binding.Literal(); ok {
						if err := addParameterValue(literal); err != nil {
							return err
						}
						continue
					}
					parentName, ok := binding.ParentName()
					if !ok {
						return errors.New("invalid parameter binding")
					}
					if err := addString(parentName); err != nil {
						return err
					}
				}
			}
			if step.Validation != nil {
				if err := addCollectionElements(len(step.Validation.ExpectedValues)); err != nil {
					return err
				}
				if err := addStrings(step.Validation.Kind, step.Validation.Expected, step.Validation.Attribute); err != nil {
					return err
				}
				for _, value := range step.Validation.ExpectedValues {
					if err := addString(value); err != nil {
						return err
					}
				}
			}
			if len(step.Children) > 0 {
				stack = append(stack, stepFrame{step.Children, frame.depth + 1})
			}
			if step.ValidationGroup != nil {
				if err := addCollectionElements(len(step.ValidationGroup.Branches)); err != nil {
					return err
				}
				for _, branch := range step.ValidationGroup.Branches {
					if err := addCollectionElements(len(branch.Steps)); err != nil {
						return err
					}
					if err := addStrings(branch.ID, branch.Name); err != nil {
						return err
					}
					stack = append(stack, stepFrame{branch.Steps, frame.depth + 1})
				}
			}
		}
	}
	for _, node := range p.Nodes {
		if err := addCollectionElements(len(node.Selectors) + len(node.Fingerprint.Attributes) + len(node.Fingerprint.Path) + len(node.Fingerprint.Framework)); err != nil {
			return err
		}
		selectors += len(node.Selectors)
		attributes += len(node.Fingerprint.Attributes)
		paths += len(node.Fingerprint.Path)
		if selectors > MaxAggregateSelectors || attributes > MaxAggregateFingerprintKV || paths > MaxAggregatePathSegments {
			return errors.New("execution plan node aggregate limit exceeded")
		}
		f := node.Fingerprint
		if err := addStrings(node.ElementTargetID, node.VersionID, node.DisplayName, node.PageURL, node.Origin, f.Tag, f.Text, f.ARIA.Role, f.ARIA.Name, f.Neighbors.Prev, f.Neighbors.Next, f.Neighbors.ParentTag, f.LabelText, f.FormID); err != nil {
			return err
		}
		for _, selector := range node.Selectors {
			if err := addStrings(string(selector.Type), selector.Value); err != nil {
				return err
			}
		}
		for key, value := range f.Attributes {
			if err := addStrings(key, value); err != nil {
				return err
			}
		}
		for _, value := range f.Path {
			if err := addString(value); err != nil {
				return err
			}
		}
		for _, info := range f.Framework {
			if err := addStrings(string(info.Kind), info.Version, string(info.Evidence)); err != nil {
				return err
			}
		}
	}
	for _, resolution := range p.References {
		if err := addStrings(resolution.ParentVersionID, resolution.StepID, resolution.FlowFragmentID, resolution.WorkflowVersionID); err != nil {
			return err
		}
	}
	return nil
}

func validateStepBounds(steps []Step) error {
	type entry struct {
		steps []Step
		depth int
	}
	stack := []entry{{steps: steps, depth: 1}}
	visited := 0
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.depth > MaxStepNestingDepth {
			return fmt.Errorf("step nesting depth exceeds maximum %d", MaxStepNestingDepth)
		}
		visited += len(current.steps)
		if visited > MaxVisitedSteps {
			return fmt.Errorf("visited steps exceed maximum %d", MaxVisitedSteps)
		}
		for i := range current.steps {
			step := current.steps[i]
			if len(step.Children) > 0 {
				stack = append(stack, entry{step.Children, current.depth + 1})
			}
			if step.ValidationGroup != nil {
				for _, branch := range step.ValidationGroup.Branches {
					stack = append(stack, entry{branch.Steps, current.depth + 1})
				}
			}
		}
	}
	return nil
}

func validateReachableWorkflowReferences(rootVersionIDs []string, workflows map[string]WorkflowSnapshot, resolutions map[WorkflowReferenceKey]ReferenceResolution) error {
	type frame struct {
		versionID string
		depth     int
		exit      bool
	}
	states := make(map[string]uint8, len(workflows))
	reachable, edges := 0, 0
	for _, rootVersionID := range rootVersionIDs {
		if states[rootVersionID] == 2 {
			continue
		}
		stack := []frame{{versionID: rootVersionID, depth: 1}}
		for len(stack) > 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if current.exit {
				states[current.versionID] = 2
				continue
			}
			if current.depth > MaxWorkflowReferenceDepth {
				return fmt.Errorf("workflow reference depth exceeds maximum %d", MaxWorkflowReferenceDepth)
			}
			if states[current.versionID] == 1 {
				return fmt.Errorf("workflow reference cycle includes version %q", current.versionID)
			}
			if states[current.versionID] == 2 {
				continue
			}
			states[current.versionID] = 1
			reachable++
			if reachable > MaxReachableWorkflows {
				return fmt.Errorf("reachable workflows exceed maximum %d", MaxReachableWorkflows)
			}
			stack = append(stack, frame{versionID: current.versionID, depth: current.depth, exit: true})
			workflow := workflows[current.versionID]
			steps := append([]Step(nil), workflow.Steps...)
			for len(steps) > 0 {
				step := steps[len(steps)-1]
				steps = steps[:len(steps)-1]
				steps = append(steps, step.Children...)
				if step.ValidationGroup != nil {
					for _, branch := range step.ValidationGroup.Branches {
						steps = append(steps, branch.Steps...)
					}
				}
				if step.Kind != FlowFragmentReference {
					continue
				}
				edges++
				if edges > MaxWorkflowReferenceEdges {
					return fmt.Errorf("workflow reference edges exceed maximum %d", MaxWorkflowReferenceEdges)
				}
				key := WorkflowReferenceKey{ParentVersionID: current.versionID, StepID: step.ID}
				if resolution, exists := resolutions[key]; exists {
					stack = append(stack, frame{versionID: resolution.WorkflowVersionID, depth: current.depth + 1})
				}
			}
		}
	}
	return nil
}

func validateDependencies(workflow WorkflowSnapshot, workflows map[string]WorkflowSnapshot, nodes map[NodeDependencyKey]struct{}, resolutions map[WorkflowReferenceKey]ReferenceResolution) error {
	stack := append([]Step(nil), workflow.Steps...)
	for len(stack) > 0 {
		step := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		stack = append(stack, step.Children...)
		if step.ValidationGroup != nil {
			for _, branch := range step.ValidationGroup.Branches {
				stack = append(stack, branch.Steps...)
			}
		}
		if step.ElementTargetID != "" {
			if _, exists := nodes[NodeDependencyKey{ElementTargetID: step.ElementTargetID, VersionID: step.ElementTargetVersionID}]; !exists {
				return fmt.Errorf("step %q targets missing node version", step.ID)
			}
		}
		if step.Kind != FlowFragmentReference {
			continue
		}
		if step.Reference == nil || strings.TrimSpace(step.Reference.WorkflowVersionID) == "" {
			return fmt.Errorf("workflow reference step %q requires an exact workflow version", step.ID)
		}
		key := WorkflowReferenceKey{ParentVersionID: workflow.VersionID, StepID: step.ID}
		resolution, exists := resolutions[key]
		if !exists {
			return fmt.Errorf("workflow reference step %q has no resolution", step.ID)
		}
		if resolution.ParentVersionID != workflow.VersionID || resolution.StepID != step.ID || resolution.FlowFragmentID != step.Reference.FlowFragmentID || resolution.WorkflowVersionID != step.Reference.WorkflowVersionID {
			return fmt.Errorf("workflow reference step %q resolution disagrees with fixed reference", step.ID)
		}
		delete(resolutions, key)
		target, exists := workflows[resolution.WorkflowVersionID]
		if !exists || target.FlowFragmentID != resolution.FlowFragmentID {
			return fmt.Errorf("workflow reference step %q targets missing workflow version", step.ID)
		}
		if err := validateBindings(workflow.Parameters, target.Parameters, step.Reference.ParameterBindings); err != nil {
			return wrapOrPropagate(err, func(cause error) error {
				return fmt.Errorf("workflow reference step %q parameter bindings: %w", step.ID, cause)
			})
		}
	}
	return nil
}
