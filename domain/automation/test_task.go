package automation

import (
	"errors"
	"fmt"
	"github.com/Capsule7446/healix-core/domain/parameter"
	"reflect"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/interpolation"
)

const (
	IssueInvalidTask       = "INVALID_TEST_TASK"
	IssueInvalidItem       = "INVALID_TEST_TASK_ITEM"
	IssueWorkflowMissing   = "WORKFLOW_UNAVAILABLE"
	IssueVersionMissing    = "WORKFLOW_VERSION_UNAVAILABLE"
	IssueNodeVersion       = "NODE_VERSION_UNAVAILABLE"
	IssueWorkflowCycle     = "WORKFLOW_CYCLE"
	IssueParameter         = "PARAMETER_INCOMPATIBLE"
	IssueEnvironment       = "ENVIRONMENT_KEY_MISSING"
	IssueDependencyChanged = "DEPENDENCY_CHANGED"
)

// ValidationIssue 可以安全地返回到 UI：它标识键和路径，但从不包含参数或环境值。
type ValidationIssue struct {
	Code           string
	ItemSequence   int
	WorkflowPath   string
	Location       string
	Recommendation string
}

type ValidationIssues []ValidationIssue

func (issues ValidationIssues) Error() string {
	if len(issues) == 0 {
		return ""
	}
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		message := issue.Code
		if issue.Location != "" {
			message += " at " + issue.Location
		}
		if issue.Recommendation != "" {
			message += ": " + issue.Recommendation
		}
		parts = append(parts, message)
	}
	return strings.Join(parts, "; ")
}

// Validate reports every field failure through one aggregate envelope. Field
// paths are logical and locale-neutral, and item indexes are 0-based so they
// address the slice the caller passed.
func (t ExecutionFlow) Validate() error {
	var violations []fault.Violation
	if strings.TrimSpace(t.ID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "id", "execution flow id is required"))
	}
	if strings.TrimSpace(t.DisplayName) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "displayName", "execution flow display name is required"))
	}
	if strings.TrimSpace(t.CurrentVersionID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "currentVersionId", "execution flow current version id is required"))
	}
	if t.CreatedAt <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "createdAt", "execution flow created timestamp is required"))
	}
	if t.UpdatedAt <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "updatedAt", "execution flow updated timestamp is required"))
	}
	if len(violations) > 0 {
		return executionFlowInvalidError(violations)
	}
	return nil
}

// Validate reports every field failure through one aggregate envelope. Sub
// validation failures degrade into violations of this version rather than
// nesting another fault, and no identity, key or enum value reaches public text.
func (v ExecutionFlowVersion) Validate() error {
	var violations []fault.Violation
	if strings.TrimSpace(v.ID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "id", "execution flow version id is required"))
	}
	if strings.TrimSpace(v.ExecutionFlowID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "executionFlowId", "execution flow version owner id is required"))
	}
	if v.VersionNumber < 1 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "versionNumber", "execution flow version number must be positive"))
	}
	if v.CreatedAt <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "createdAt", "execution flow version created timestamp is required"))
	}
	if !v.FailurePolicy.IsValid() {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "failurePolicy", "execution flow version failure policy is not supported"))
	}
	if len(v.Items) == 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "items", "execution flow version requires at least one item"))
	}
	seenEnvironmentKeys := map[string]bool{}
	for index, key := range v.RequiredEnvironmentKeys {
		field := fmt.Sprintf("requiredEnvironmentKeys.%d", index)
		switch {
		case strings.TrimSpace(key) == "":
			violations = append(violations, mustViolation(fault.CodeFieldRequired, field, "required environment key must not be blank"))
		case seenEnvironmentKeys[key]:
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field, "required environment key is duplicated"))
		}
		seenEnvironmentKeys[key] = true
	}
	seenIDs := map[string]bool{}
	seenSequences := map[int]bool{}
	for index, item := range v.Items {
		field := func(name string) string { return fmt.Sprintf("items.%d.%s", index, name) }
		switch {
		case strings.TrimSpace(item.ID) == "":
			violations = append(violations, mustViolation(fault.CodeFieldRequired, field("id"), "execution flow item id is required"))
		case seenIDs[item.ID]:
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field("id"), "execution flow item id is duplicated"))
		}
		seenIDs[item.ID] = true
		if item.TestTaskVersionID != v.ID {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field("executionFlowVersionId"), "execution flow item belongs to another version"))
		}
		if item.SequenceNumber != index+1 || seenSequences[item.SequenceNumber] {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, field("sequenceNumber"), "execution flow item sequence numbers must be unique and contiguous from 1"))
		}
		seenSequences[item.SequenceNumber] = true
		if strings.TrimSpace(item.FlowFragmentID) == "" {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, field("flowFragmentId"), "execution flow item flow fragment id is required"))
		}
		if item.VersionPolicy.Validate() != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, field("versionPolicy"), "execution flow item version policy is not supported"))
		}
		switch item.VersionPolicy {
		case FlowFragmentVersionFixed:
			if strings.TrimSpace(item.WorkflowVersionID) == "" {
				violations = append(violations, mustViolation(fault.CodeFieldRequired, field("flowFragmentVersionId"), "fixed version policy requires a flow fragment version id"))
			}
		case FlowFragmentVersionLatest:
			if strings.TrimSpace(item.WorkflowVersionID) != "" {
				violations = append(violations, mustViolation(fault.CodeFieldMismatch, field("flowFragmentVersionId"), "latest version policy must not persist a flow fragment version id"))
			}
		}
	}
	if len(violations) > 0 {
		return executionFlowInvalidError(violations)
	}
	return nil
}

func (a ExecutionFlowAggregate) Validate() error {
	if err := a.Task.Validate(); err != nil {
		return err
	}
	if len(a.Versions) == 0 {
		return errors.New("test task requires version history")
	}
	seenIDs := map[string]bool{}
	seenNumbers := map[int]bool{}
	byID := map[string]ExecutionFlowVersion{}
	highest := ExecutionFlowVersion{}
	for _, version := range a.Versions {
		if err := version.Validate(); err != nil {
			return err
		}
		if version.ExecutionFlowID != a.Task.ID {
			return errors.New("test task version belongs to another task")
		}
		if seenIDs[version.ID] || seenNumbers[version.VersionNumber] {
			return errors.New("test task history contains duplicate version identity")
		}
		seenIDs[version.ID] = true
		seenNumbers[version.VersionNumber] = true
		byID[version.ID] = version
		if version.VersionNumber > highest.VersionNumber {
			highest = version
		}
	}
	for number := 1; number <= len(a.Versions); number++ {
		if !seenNumbers[number] {
			return errors.New("test task version numbers must be contiguous from 1")
		}
	}
	for _, version := range a.Versions {
		if version.VersionNumber == 1 {
			if version.SourceVersionID != "" {
				return errors.New("test task version 1 cannot have a source version")
			}
			continue
		}
		source, ok := byID[version.SourceVersionID]
		if !ok || source.VersionNumber >= version.VersionNumber {
			return fmt.Errorf("test task version %s source must be an earlier version", version.ID)
		}
	}
	if a.Current.ID != a.Task.CurrentVersionID || a.Current.ID != highest.ID {
		return errors.New("test task current version must match the latest history version")
	}
	if !reflect.DeepEqual(a.Current, highest) {
		return errors.New("test task current version content must match history")
	}
	return nil
}

func ResolveParameterValues(definitions []ParameterDefinition, supplied map[string]parameter.Value) (map[string]parameter.Value, error) {
	byName := make(map[string]ParameterDefinition, len(definitions))
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return nil, fmt.Errorf("parameter %q: %w", definition.Name, err)
		}
		if _, duplicate := byName[definition.Name]; duplicate {
			return nil, fmt.Errorf("duplicate parameter %q", definition.Name)
		}
		byName[definition.Name] = definition
	}
	for name := range supplied {
		if _, exists := byName[name]; !exists {
			return nil, fmt.Errorf("parameter %q is unknown", name)
		}
	}
	resolved := make(map[string]parameter.Value, len(definitions))
	for _, definition := range definitions {
		value, exists := supplied[definition.Name]
		if !exists {
			if fallback, present := definition.Default.Value(); present {
				value = fallback
			} else {
				return nil, fmt.Errorf("parameter %q is required", definition.Name)
			}
		}
		if err := definition.ValidateValue(value); err != nil {
			return nil, fmt.Errorf("parameter %q: %w", definition.Name, err)
		}
		resolved[definition.Name] = value.Clone()
	}
	return resolved, nil
}

func validateReferenceBindings(parent, child []ParameterDefinition, bindings map[string]parameter.Binding) error {
	parents := map[string]ParameterDefinition{}
	for _, definition := range parent {
		parents[definition.Name] = definition
	}
	for _, definition := range child {
		binding, exists := bindings[definition.Name]
		if !exists {
			if _, hasDefault := definition.Default.Value(); hasDefault {
				continue
			}
			return fmt.Errorf("parameter %q is required", definition.Name)
		}
		if literal, ok := binding.Literal(); ok {
			if err := definition.ValidateValue(literal); err != nil {
				return fmt.Errorf("parameter %q: %w", definition.Name, err)
			}
			continue
		}
		name, ok := binding.ParentName()
		if !ok || name == "" {
			return fmt.Errorf("parameter %q has invalid binding", definition.Name)
		}
		parentDefinition, exists := parents[name]
		if !exists {
			return fmt.Errorf("parent parameter %q is missing", name)
		}
		if parentDefinition.Type != definition.Type {
			return fmt.Errorf("parameter %q parent reference type mismatch", definition.Name)
		}
	}
	for name := range bindings {
		found := false
		for _, definition := range child {
			if name == definition.Name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("parameter %q is unknown", name)
		}
	}
	return nil
}

func (p ResolvedExecutionFlow) Validate() error {
	if err := p.Task.Validate(); err != nil {
		return err
	}
	if err := p.Version.Validate(); err != nil {
		return err
	}
	if p.Version.ExecutionFlowID != p.Task.ID {
		return errors.New("test task publication candidate identity is inconsistent")
	}
	if p.Version.VersionNumber == 1 {
		if p.ExpectedExecutionFlowRevision != 0 || p.Version.SourceVersionID != "" {
			return errors.New("new test task must publish version 1 without a source version")
		}
	} else if err := p.ExpectedExecutionFlowRevision.ValidatePersisted(); err != nil || strings.TrimSpace(p.Version.SourceVersionID) == "" {
		return errors.New("subsequent test task version requires source and expected revision")
	}
	workflows := map[string]FlowFragmentDependencySnapshot{}
	for _, dependency := range p.Workflows {
		if dependency.FlowFragment.ID == "" || dependency.Version.ID == "" ||
			dependency.Version.FlowFragmentID != dependency.FlowFragment.ID || dependency.Version.VersionNumber < 1 {
			return errors.New("workflow dependency snapshot identity is invalid")
		}
		key := dependency.FlowFragment.ID + "\x00" + dependency.Version.ID
		if _, exists := workflows[key]; exists {
			return errors.New("duplicate workflow dependency snapshot")
		}
		workflows[key] = dependency
	}
	nodes := map[string]bool{}
	for _, dependency := range p.Nodes {
		if dependency.ElementTarget.ID == "" || dependency.Version.ID == "" ||
			dependency.Version.ElementTargetID != dependency.ElementTarget.ID || dependency.Version.VersionNumber < 1 {
			return errors.New("node dependency snapshot identity is invalid")
		}
		key := ElementTargetDependencyIdentity(dependency.ElementTarget.ID, dependency.Version.ID)
		if nodes[key] {
			return errors.New("duplicate node dependency snapshot")
		}
		nodes[key] = true
	}
	for _, item := range p.Version.Items {
		matched := false
		for _, dependency := range p.Workflows {
			if dependency.FlowFragment.ID != item.FlowFragmentID {
				continue
			}
			switch item.VersionPolicy {
			case FlowFragmentVersionFixed:
				matched = dependency.Version.ID == item.WorkflowVersionID
			case FlowFragmentVersionLatest:
				matched = dependency.ResolvedFromLatest
			}
			if matched {
				break
			}
		}
		if !matched {
			return fmt.Errorf("test task item %d has no matching workflow dependency", item.SequenceNumber)
		}
		for _, dependency := range p.Workflows {
			if dependency.FlowFragment.ID == item.FlowFragmentID && (item.VersionPolicy == FlowFragmentVersionLatest && dependency.ResolvedFromLatest || item.VersionPolicy == FlowFragmentVersionFixed && dependency.Version.ID == item.WorkflowVersionID) {
				if _, err := ResolveParameterValues(dependency.Version.Definition.Parameters, item.Parameters); err != nil {
					return fmt.Errorf("test task item %d parameters: %w", item.SequenceNumber, err)
				}
				break
			}
		}
	}
	return p.validateDependencyGraph(nodes)
}

func (p ResolvedExecutionFlow) validateDependencyGraph(nodes map[string]bool) error {
	byVersion := map[string]FlowFragmentDependencySnapshot{}
	for _, dependency := range p.Workflows {
		if _, duplicate := byVersion[dependency.Version.ID]; duplicate {
			return errors.New("duplicate workflow version dependency")
		}
		byVersion[dependency.Version.ID] = dependency
	}
	references := map[string]FlowFragmentReferenceResolution{}
	for _, resolution := range p.References {
		key := resolution.ParentFlowFragmentVersionID + "\x00" + resolution.StepID
		if resolution.ParentFlowFragmentVersionID == "" || resolution.StepID == "" ||
			resolution.FlowFragmentID == "" || resolution.WorkflowVersionID == "" {
			return errors.New("workflow reference resolution identity is incomplete")
		}
		if _, duplicate := references[key]; duplicate {
			return errors.New("duplicate workflow reference resolution")
		}
		references[key] = resolution
	}
	usedWorkflows := map[string]bool{}
	usedReferences := map[string]bool{}
	usedNodes := map[string]bool{}
	visiting := map[string]bool{}
	var visit func(string) error
	visit = func(versionID string) error {
		dependency, exists := byVersion[versionID]
		if !exists {
			return fmt.Errorf("workflow dependency %s is missing", versionID)
		}
		if visiting[versionID] {
			return fmt.Errorf("workflow dependency cycle includes %s", versionID)
		}
		if usedWorkflows[versionID] {
			return nil
		}
		visiting[versionID] = true
		defer delete(visiting, versionID)
		var walk func([]FlowFragmentStep) error
		walk = func(steps []FlowFragmentStep) error {
			for _, step := range steps {
				if step.ElementTargetID != "" {
					key := ElementTargetDependencyIdentity(step.ElementTargetID, step.ElementTargetVersionID)
					if !nodes[key] {
						return fmt.Errorf("step %s has no exact node dependency", step.ID)
					}
					usedNodes[key] = true
				}
				if step.Reference != nil {
					key := versionID + "\x00" + step.ID
					resolution, ok := references[key]
					if !ok {
						return fmt.Errorf("step %s has no workflow reference resolution", step.ID)
					}
					target, ok := byVersion[resolution.WorkflowVersionID]
					if !ok || target.FlowFragment.ID != resolution.FlowFragmentID || resolution.FlowFragmentID != step.Reference.FlowFragmentID {
						return fmt.Errorf("step %s workflow reference target is inconsistent", step.ID)
					}
					if step.Reference.LatestPublished {
						if !resolution.ResolvedFromLatest {
							return fmt.Errorf("step %s latest workflow reference was not resolved from current", step.ID)
						}
					} else if resolution.ResolvedFromLatest || step.Reference.WorkflowVersionID != target.Version.ID {
						return fmt.Errorf("step %s fixed workflow reference changed version", step.ID)
					}
					if err := validateReferenceBindings(dependency.Version.Definition.Parameters, target.Version.Definition.Parameters, step.Reference.ParameterBindings); err != nil {
						return fmt.Errorf("step %s parameter bindings: %w", step.ID, err)
					}
					usedReferences[key] = true
					if err := visit(target.Version.ID); err != nil {
						return err
					}
				}
				if err := walk(step.Children); err != nil {
					return err
				}
				if step.ValidationGroup != nil {
					for _, branch := range step.ValidationGroup.Branches {
						if err := walk(branch.Steps); err != nil {
							return err
						}
					}
				}
			}
			return nil
		}
		if err := walk(dependency.Version.Definition.Steps); err != nil {
			return err
		}
		usedWorkflows[versionID] = true
		return nil
	}
	for _, item := range p.Version.Items {
		for _, dependency := range p.Workflows {
			if dependency.FlowFragment.ID != item.FlowFragmentID {
				continue
			}
			if item.VersionPolicy == FlowFragmentVersionFixed && dependency.Version.ID != item.WorkflowVersionID {
				continue
			}
			if item.VersionPolicy == FlowFragmentVersionLatest && !dependency.ResolvedFromLatest {
				continue
			}
			if err := visit(dependency.Version.ID); err != nil {
				return err
			}
			break
		}
	}
	if len(usedWorkflows) != len(byVersion) || len(usedReferences) != len(references) || len(usedNodes) != len(nodes) {
		return errors.New("publication dependency graph contains orphan snapshots or resolutions")
	}
	return nil
}

func EnvironmentKeys(values ...string) ([]string, error) {
	set := map[string]bool{}
	for _, value := range values {
		names, err := interpolation.Names(value)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			if strings.HasPrefix(name, "env.") && len(name) > len("env.") {
				set[strings.TrimPrefix(name, "env.")] = true
			}
		}
	}
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result, nil
}

func ElementTargetDependencyIdentity(nodeID, versionID string) string {
	return nodeID + "\x00" + versionID
}
