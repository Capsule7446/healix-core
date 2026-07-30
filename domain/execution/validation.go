package execution

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/interpolation"
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
				return fmt.Errorf("parameter %q: %w", definition.Name, err)
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
			return fmt.Errorf("parameter %q: %w", definition.Name, err)
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
			return fmt.Errorf("parameter %q: %w", definition.Name, err)
		}
	}
	return nil
}

func (p Draft) Validate() error {
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
			return fmt.Errorf("workflow version %q failed execution preflight: %w", workflow.VersionID, err)
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
				return fmt.Errorf("entry %q parameter snapshot: %w", entry.ExecutionID, err)
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
			return fmt.Errorf("node version %q failed execution preflight: %w", snapshot.VersionID, err)
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
		return fmt.Errorf("execution plan exceeds execution budget: %w", err)
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

func (w WorkflowSnapshot) Validate() error {
	var problems []string
	if strings.TrimSpace(w.FlowFragmentID) == "" || strings.TrimSpace(w.VersionID) == "" || (w.ID != "" && w.ID != w.FlowFragmentID) {
		problems = append(problems, "workflow version does not belong to workflow")
	}
	if strings.TrimSpace(w.DisplayName) == "" {
		problems = append(problems, "display name is required")
	}
	if w.VersionNumber < 1 {
		problems = append(problems, "version number must be >= 1")
	}
	if len(w.Steps) == 0 {
		problems = append(problems, "workflow requires at least one step")
	}
	seen := make(map[string]struct{})
	if err := validateStepBounds(w.Steps); err != nil {
		problems = append(problems, err.Error())
	} else {
		problems = append(problems, validateSteps(w.Steps, true, seen)...)
	}
	parameterNames := make([]string, 0, len(w.Parameters))
	for _, parameter := range w.Parameters {
		if err := parameter.Validate(); err != nil {
			problems = append(problems, fmt.Sprintf("parameter %q: %v", parameter.Name, err))
		}
		parameterNames = append(parameterNames, parameter.Name)
	}
	sort.Strings(parameterNames)
	for i := 1; i < len(parameterNames); i++ {
		if parameterNames[i] == parameterNames[i-1] {
			problems = append(problems, fmt.Sprintf("duplicate parameter %q", parameterNames[i]))
		}
	}
	if len(problems) != 0 {
		return errors.New(strings.Join(problems, "; "))
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
			return fmt.Errorf("workflow reference step %q parameter bindings: %w", step.ID, err)
		}
	}
	return nil
}

func validateSteps(steps []Step, root bool, seen map[string]struct{}) []string {
	var problems []string
	for _, step := range steps {
		if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.DisplayName) == "" {
			problems = append(problems, "step id and display name are required")
		}
		if _, exists := seen[step.ID]; exists {
			problems = append(problems, fmt.Sprintf("duplicate workflow step id %q", step.ID))
		}
		seen[step.ID] = struct{}{}
		if step.Kind != ActionStep && step.Optional {
			problems = append(problems, fmt.Sprintf("step %q only ACTION can be optional", step.DisplayName))
		}
		switch step.Kind {
		case ActionStep:
			problems = append(problems, validateAction(step)...)
		case WaitStep:
			problems = append(problems, validateWait(step)...)
		case RepeatStep:
			problems = append(problems, validateRepeat(step)...)
			problems = append(problems, validateSteps(step.Children, false, seen)...)
		case FlowFragmentReference:
			problems = append(problems, step.Reference.Validate(step)...)
		case ValidationStep:
			if !root {
				problems = append(problems, fmt.Sprintf("validation step %q must be a root step or validation-group member", step.DisplayName))
			}
			problems = append(problems, validateValidationStep(step, false)...)
		case ValidationGroupStep:
			if !root {
				problems = append(problems, fmt.Sprintf("validation group %q must be a root step", step.DisplayName))
			}
			problems = append(problems, step.ValidationGroup.Validate(step, seen)...)
		default:
			problems = append(problems, fmt.Sprintf("step %q has unsupported kind %q", step.DisplayName, step.Kind))
		}
	}
	return problems
}

func validateAction(s Step) []string {
	var p []string
	if s.Validation != nil || s.ValidationGroup != nil || s.Reference != nil || s.WaitKind != "" || s.WaitMS != 0 || s.RepeatCount != 0 || len(s.Children) != 0 {
		p = append(p, fmt.Sprintf("step %q ACTION contains unsupported step configuration", s.DisplayName))
	}
	switch s.Action {
	case "click", "input", "select", "hover", "navigate", "press", "noop", "extract":
	default:
		p = append(p, fmt.Sprintf("step %q has unsupported action %q", s.DisplayName, s.Action))
	}
	if s.Action != "navigate" && s.Action != "press" && strings.TrimSpace(s.ElementTargetID) == "" {
		p = append(p, fmt.Sprintf("step %q requires a node", s.DisplayName))
	}
	if strings.TrimSpace(s.ElementTargetID) != "" && strings.TrimSpace(s.ElementTargetVersionID) == "" {
		p = append(p, fmt.Sprintf("step %q requires an exact node version", s.DisplayName))
	}
	if (s.Action == "navigate" || s.Action == "press" || s.Action == "extract") && strings.TrimSpace(s.Value) == "" {
		p = append(p, fmt.Sprintf("step %q action %s requires a value", s.DisplayName, s.Action))
	}
	if s.Action == "navigate" && strings.TrimSpace(s.Value) != "" {
		if err := validateSealedNavigationURL(s.Value); err != nil {
			p = append(p, fmt.Sprintf("step %q navigate URL: %v", s.DisplayName, err))
		}
	}
	if s.Action == "navigate" || s.Action == "input" || s.Action == "select" || s.Action == "press" {
		for _, value := range append([]string{s.Value}, s.Values...) {
			if _, err := interpolation.Names(value); err != nil {
				p = append(p, fmt.Sprintf("step %q action value: %v", s.DisplayName, err))
			}
		}
	}
	if s.Action == "select" && strings.TrimSpace(s.Value) == "" && len(s.Values) == 0 {
		p = append(p, fmt.Sprintf("step %q select requires at least one value", s.DisplayName))
	}
	return p
}

func validateSealedNavigationURL(value string) error {
	if strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return errors.New("control characters are not allowed")
	}
	names, err := interpolation.Names(value)
	if err != nil {
		return err
	}
	authorityEnd := len(value)
	if scheme := strings.Index(value, "://"); scheme >= 0 {
		authorityEnd = scheme + 3
		if slash := strings.IndexAny(value[authorityEnd:], "/?#"); slash >= 0 {
			authorityEnd += slash
		}
	}
	if strings.Contains(value[:authorityEnd], "${") {
		return errors.New("interpolation is not allowed in URL scheme or authority")
	}
	parseable := value
	for _, name := range names {
		parseable = strings.ReplaceAll(parseable, "${"+name+"}", "placeholder")
	}
	parsed, err := url.ParseRequestURI(parseable)
	if err != nil || parsed.Scheme == "" {
		return errors.New("absolute URL with explicit scheme is required")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("explicit scheme must be HTTP(S)")
	}
	if parsed.User != nil {
		return errors.New("userinfo is not allowed")
	}
	if len(names) == 0 && parsed.Host == "" {
		return errors.New("must be an absolute HTTP(S) URL")
	}
	return nil
}

func validateWait(s Step) []string {
	var p []string
	element := s.WaitKind == "element" || s.WaitKind == "element_visible" || s.WaitKind == "element_invisible"
	if s.Validation != nil || s.ValidationGroup != nil || s.Action != "" || s.Value != "" || len(s.Values) != 0 || s.RepeatCount != 0 || s.Reference != nil || len(s.Children) != 0 || (!element && (s.ElementTargetID != "" || s.ElementTargetVersionID != "")) {
		p = append(p, fmt.Sprintf("step %q WAIT contains unsupported step configuration", s.DisplayName))
	}
	switch s.WaitKind {
	case "", "sleep":
		if s.WaitMS <= 0 || s.WaitMS > MaxWaitMS {
			p = append(p, fmt.Sprintf("step %q fixed wait must be 1-%dms", s.DisplayName, MaxWaitMS))
		}
	case "element", "element_visible", "element_invisible":
		if strings.TrimSpace(s.ElementTargetID) == "" {
			p = append(p, fmt.Sprintf("step %q element wait requires a node", s.DisplayName))
		}
		if strings.TrimSpace(s.ElementTargetVersionID) == "" {
			p = append(p, fmt.Sprintf("step %q element wait requires an exact node version", s.DisplayName))
		}
		if s.WaitMS < 0 || s.WaitMS > MaxWaitMS {
			p = append(p, fmt.Sprintf("step %q timeout must be >= 0", s.DisplayName))
		}
	case "network_idle":
		if s.WaitMS < 0 || s.WaitMS > MaxWaitMS {
			p = append(p, fmt.Sprintf("step %q timeout must be >= 0", s.DisplayName))
		}
	default:
		p = append(p, fmt.Sprintf("step %q has unsupported wait kind %q", s.DisplayName, s.WaitKind))
	}
	return p
}

func validateRepeat(s Step) []string {
	var p []string
	if s.Validation != nil || s.ValidationGroup != nil || s.Action != "" || s.ElementTargetID != "" || s.ElementTargetVersionID != "" || s.Value != "" || len(s.Values) != 0 || s.WaitKind != "" || s.WaitMS != 0 || s.Reference != nil {
		p = append(p, fmt.Sprintf("step %q REPEAT contains unsupported step configuration", s.DisplayName))
	}
	if s.RepeatCount < 1 || len(s.Children) == 0 {
		p = append(p, fmt.Sprintf("step %q repeat requires count and children", s.DisplayName))
	} else if s.RepeatCount > MaxRepeatCount {
		p = append(p, fmt.Sprintf("step %q repeat count exceeds maximum %d", s.DisplayName, MaxRepeatCount))
	}
	return p
}

func (r *Reference) Validate(s Step) []string {
	var p []string
	if s.Validation != nil || s.ValidationGroup != nil || s.Action != "" || s.ElementTargetID != "" || s.ElementTargetVersionID != "" || s.Value != "" || len(s.Values) != 0 || s.WaitKind != "" || s.WaitMS != 0 || s.RepeatCount != 0 || len(s.Children) != 0 {
		p = append(p, fmt.Sprintf("step %q WORKFLOW_REF contains unsupported step configuration", s.DisplayName))
	}
	if r == nil || strings.TrimSpace(r.FlowFragmentID) == "" {
		p = append(p, fmt.Sprintf("step %q requires a workflow reference", s.DisplayName))
	}
	if r != nil {
		for name, binding := range r.ParameterBindings {
			if strings.TrimSpace(name) == "" {
				p = append(p, fmt.Sprintf("step %q has an empty parameter binding", s.DisplayName))
			}
			if _, err := binding.Resolve(nil); err != nil {
				if _, isReference := binding.ParentName(); !isReference {
					p = append(p, fmt.Sprintf("step %q parameter binding %q: %v", s.DisplayName, name, err))
				}
			}
		}
	}
	return p
}

func (v Validation) Validate(waitRequired bool) error {
	if _, err := interpolation.Names(v.Expected); err != nil {
		return fmt.Errorf("validation expected value: %w", err)
	}
	for _, value := range v.ExpectedValues {
		if _, err := interpolation.Names(value); err != nil {
			return fmt.Errorf("validation expected value: %w", err)
		}
	}
	switch v.Kind {
	case "exists", "not_exists", "visible", "not_visible", "value_not_empty", "enabled", "disabled", "checked", "unchecked", "mixed", "selected", "unselected", "pressed", "unpressed":
		if v.Expected != "" || len(v.ExpectedValues) != 0 || v.Attribute != "" || v.IgnoreCase {
			return fmt.Errorf("validation %q does not accept comparison options", v.Kind)
		}
	case "text_equals", "text_contains", "value_equals", "value_contains", "selected_text_equals", "selected_text_contains", "selected_value_equals", "selected_value_contains":
		if len(v.ExpectedValues) != 0 || v.Attribute != "" {
			return fmt.Errorf("validation %q accepts one scalar expected value", v.Kind)
		}
	case "text_matches", "value_matches":
		if len(v.ExpectedValues) != 0 || v.Attribute != "" || v.IgnoreCase {
			return fmt.Errorf("validation %q accepts only a regular expression", v.Kind)
		}
		if !strings.Contains(v.Expected, "${") {
			if _, err := regexp.Compile(v.Expected); err != nil {
				return fmt.Errorf("validation %q has invalid regular expression: %w", v.Kind, err)
			}
		}
	case "selected_set_equals", "selected_set_contains":
		if v.Expected != "" || v.Attribute != "" || v.IgnoreCase {
			return fmt.Errorf("validation %q accepts only expected values", v.Kind)
		}
	case "attribute_equals", "attribute_contains":
		if strings.TrimSpace(v.Attribute) == "" {
			return errors.New("attribute validation requires an attribute name")
		}
		if len(v.ExpectedValues) != 0 {
			return fmt.Errorf("validation %q accepts one scalar expected value", v.Kind)
		}
		if strings.Contains(v.Attribute, "${") {
			return errors.New("attribute validation does not accept variable expressions")
		}
	default:
		return fmt.Errorf("unsupported validation kind %q", v.Kind)
	}
	if waitRequired {
		return validateValidationWait(v.MaxWaitMS, v.StabilityMS)
	}
	if v.MaxWaitMS != 0 || v.StabilityMS != 0 {
		return errors.New("validation group member must inherit the group wait")
	}
	return nil
}

func validateValidationWait(maxWait, stability int) error {
	if maxWait < validationMinWaitMS || maxWait > validationMaxWaitMS {
		return fmt.Errorf("validation maximum wait must be %d-%dms", validationMinWaitMS, validationMaxWaitMS)
	}
	if stability < validationMinStabilityMS || stability > validationMaxStabilityMS {
		return fmt.Errorf("validation stability window must be %d-%dms", validationMinStabilityMS, validationMaxStabilityMS)
	}
	if stability >= maxWait {
		return errors.New("validation stability window must be shorter than maximum wait")
	}
	return nil
}

func validateValidationStep(s Step, member bool) []string {
	var p []string
	if s.Validation == nil {
		return []string{fmt.Sprintf("validation step %q requires validation configuration", s.DisplayName)}
	}
	if s.ValidationGroup != nil || s.Action != "" || s.Reference != nil || s.Value != "" || len(s.Values) != 0 || s.WaitKind != "" || s.WaitMS != 0 || s.RepeatCount != 0 || len(s.Children) != 0 || s.Optional {
		p = append(p, fmt.Sprintf("validation step %q contains unsupported action or child configuration", s.DisplayName))
	}
	if strings.TrimSpace(s.ElementTargetID) == "" || strings.TrimSpace(s.ElementTargetVersionID) == "" {
		p = append(p, fmt.Sprintf("validation step %q requires an exact node reference", s.DisplayName))
	}
	if err := s.Validation.Validate(!member); err != nil {
		p = append(p, fmt.Sprintf("validation step %q: %v", s.DisplayName, err))
	}
	return p
}

func (g *ValidationGroup) Validate(s Step, seen map[string]struct{}) []string {
	if g == nil {
		return []string{fmt.Sprintf("validation group %q requires group configuration", s.DisplayName)}
	}
	var p []string
	if s.Validation != nil || s.Action != "" || s.Reference != nil || s.ElementTargetID != "" || s.ElementTargetVersionID != "" || s.Value != "" || len(s.Values) != 0 || s.WaitKind != "" || s.WaitMS != 0 || s.RepeatCount != 0 || len(s.Children) != 0 || s.Optional {
		p = append(p, fmt.Sprintf("validation group %q contains unsupported step configuration", s.DisplayName))
	}
	if err := validateValidationWait(g.MaxWaitMS, g.StabilityMS); err != nil {
		p = append(p, fmt.Sprintf("validation group %q wait: %v", s.DisplayName, err))
	}
	if len(g.Branches) == 0 || len(g.Branches) > validationMaxBranches {
		p = append(p, fmt.Sprintf("validation group %q requires 1-%d branches", s.DisplayName, validationMaxBranches))
	}
	branchIDs, total := map[string]struct{}{}, 0
	for _, branch := range g.Branches {
		if strings.TrimSpace(branch.ID) == "" || strings.TrimSpace(branch.Name) == "" {
			p = append(p, fmt.Sprintf("validation group %q branch id and name are required", s.DisplayName))
		}
		if _, ok := branchIDs[branch.ID]; ok && branch.ID != "" {
			p = append(p, fmt.Sprintf("validation group %q has duplicate branch id %q", s.DisplayName, branch.ID))
		}
		branchIDs[branch.ID] = struct{}{}
		if len(branch.Steps) == 0 || len(branch.Steps) > validationMaxBranchSteps {
			p = append(p, fmt.Sprintf("validation group %q branch %q requires 1-%d validation steps", s.DisplayName, branch.Name, validationMaxBranchSteps))
		}
		total += len(branch.Steps)
		for _, member := range branch.Steps {
			if _, ok := seen[member.ID]; ok {
				p = append(p, fmt.Sprintf("duplicate step id %q", member.ID))
			}
			seen[member.ID] = struct{}{}
			if strings.TrimSpace(member.ID) == "" || strings.TrimSpace(member.DisplayName) == "" {
				p = append(p, fmt.Sprintf("validation group %q member step id and display name are required", s.DisplayName))
			}
			if member.Kind != ValidationStep {
				p = append(p, fmt.Sprintf("validation group %q branch %q only accepts VALIDATION steps", s.DisplayName, branch.Name))
				continue
			}
			p = append(p, validateValidationStep(member, true)...)
		}
	}
	if total > validationMaxGroupSteps {
		p = append(p, fmt.Sprintf("validation group %q has %d validation steps; maximum is %d", s.DisplayName, total, validationMaxGroupSteps))
	}
	return p
}
