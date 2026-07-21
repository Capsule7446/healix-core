package automation

import (
	"errors"
	"fmt"
	"sort"
	"strings"

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

func (t TestTask) Validate() error {
	var problems []string
	if strings.TrimSpace(t.ID) == "" {
		problems = append(problems, "test task id is required")
	}
	if strings.TrimSpace(t.DisplayName) == "" {
		problems = append(problems, "test task display name is required")
	}
	if strings.TrimSpace(t.CurrentVersionID) == "" {
		problems = append(problems, "test task current version id is required")
	}
	if t.CreatedAt <= 0 || t.UpdatedAt <= 0 {
		problems = append(problems, "test task timestamps are required")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func (v TestTaskVersion) Validate() error {
	var problems []string
	if strings.TrimSpace(v.ID) == "" || strings.TrimSpace(v.TestTaskID) == "" {
		problems = append(problems, "test task version id and owner are required")
	}
	if v.VersionNumber < 1 {
		problems = append(problems, "test task version number must be >= 1")
	}
	if v.CreatedAt <= 0 {
		problems = append(problems, "test task version created timestamp is required")
	}
	if !v.FailurePolicy.IsValid() {
		problems = append(problems, fmt.Sprintf("invalid failure policy %q", v.FailurePolicy))
	}
	if len(v.Items) == 0 {
		problems = append(problems, "test task version requires at least one item")
	}
	seenIDs := map[string]bool{}
	seenSequences := map[int]bool{}
	seenEnvironmentKeys := map[string]bool{}
	for _, key := range v.RequiredEnvironmentKeys {
		if strings.TrimSpace(key) == "" || seenEnvironmentKeys[key] {
			problems = append(problems, "required environment keys must be non-empty and unique")
		}
		seenEnvironmentKeys[key] = true
	}
	for index, item := range v.Items {
		if strings.TrimSpace(item.ID) == "" {
			problems = append(problems, fmt.Sprintf("item %d id is required", index+1))
		} else if seenIDs[item.ID] {
			problems = append(problems, fmt.Sprintf("duplicate item id %s", item.ID))
		}
		seenIDs[item.ID] = true
		if item.TestTaskVersionID != v.ID {
			problems = append(problems, fmt.Sprintf("item %d belongs to another version", index+1))
		}
		if item.SequenceNumber != index+1 || seenSequences[item.SequenceNumber] {
			problems = append(problems, "item sequence numbers must be unique and contiguous from 1")
		}
		seenSequences[item.SequenceNumber] = true
		if strings.TrimSpace(item.WorkflowID) == "" {
			problems = append(problems, fmt.Sprintf("item %d workflow id is required", index+1))
		}
		if err := item.VersionPolicy.Validate(); err != nil {
			problems = append(problems, fmt.Sprintf("item %d: %v", index+1, err))
		}
		switch item.VersionPolicy {
		case WorkflowVersionFixed:
			if strings.TrimSpace(item.WorkflowVersionID) == "" {
				problems = append(problems, fmt.Sprintf("item %d fixed version id is required", index+1))
			}
		case WorkflowVersionLatest:
			if strings.TrimSpace(item.WorkflowVersionID) != "" {
				problems = append(problems, fmt.Sprintf("item %d latest policy cannot persist a version id", index+1))
			}
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func (a TestTaskAggregate) Validate() error {
	if err := a.Task.Validate(); err != nil {
		return err
	}
	if len(a.Versions) == 0 {
		return errors.New("test task requires version history")
	}
	seenIDs := map[string]bool{}
	seenNumbers := map[int]bool{}
	byID := map[string]TestTaskVersion{}
	highest := TestTaskVersion{}
	for _, version := range a.Versions {
		if err := version.Validate(); err != nil {
			return fmt.Errorf("test task version %s: %w", version.ID, err)
		}
		if version.TestTaskID != a.Task.ID {
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
	return nil
}

func (p TestTaskVersionPlan) Validate() error {
	if err := p.Task.Validate(); err != nil {
		return err
	}
	if err := p.Version.Validate(); err != nil {
		return err
	}
	if p.Version.TestTaskID != p.Task.ID || p.Task.CurrentVersionID != p.Version.ID {
		return errors.New("test task publication candidate identity is inconsistent")
	}
	if p.Version.VersionNumber == 1 {
		if p.ExpectedTaskRevision != 0 || p.Version.SourceVersionID != "" {
			return errors.New("new test task must publish version 1 without a source version")
		}
	} else if err := p.ExpectedTaskRevision.ValidatePersisted(); err != nil || strings.TrimSpace(p.Version.SourceVersionID) == "" {
		return errors.New("subsequent test task version requires source and expected revision")
	}
	workflows := map[string]WorkflowDependencySnapshot{}
	for _, dependency := range p.Workflows {
		if dependency.Workflow.ID == "" || dependency.Version.ID == "" ||
			dependency.Version.WorkflowID != dependency.Workflow.ID || dependency.Version.VersionNumber < 1 {
			return errors.New("workflow dependency snapshot identity is invalid")
		}
		key := dependency.Workflow.ID + "\x00" + dependency.Version.ID
		if _, exists := workflows[key]; exists {
			return errors.New("duplicate workflow dependency snapshot")
		}
		workflows[key] = dependency
	}
	nodes := map[string]bool{}
	for _, dependency := range p.Nodes {
		if dependency.Node.ID == "" || dependency.Version.ID == "" ||
			dependency.Version.NodeID != dependency.Node.ID || dependency.Version.VersionNumber < 1 {
			return errors.New("node dependency snapshot identity is invalid")
		}
		key := NodeDependencyIdentity(dependency.Node.ID, dependency.Version.ID)
		if nodes[key] {
			return errors.New("duplicate node dependency snapshot")
		}
		nodes[key] = true
	}
	for _, item := range p.Version.Items {
		matched := false
		for _, dependency := range p.Workflows {
			if dependency.Workflow.ID != item.WorkflowID {
				continue
			}
			switch item.VersionPolicy {
			case WorkflowVersionFixed:
				matched = dependency.Version.ID == item.WorkflowVersionID
			case WorkflowVersionLatest:
				matched = dependency.ResolvedFromLatest && dependency.Workflow.CurrentVersionID == dependency.Version.ID
			}
			if matched {
				break
			}
		}
		if !matched {
			return fmt.Errorf("test task item %d has no matching workflow dependency", item.SequenceNumber)
		}
	}
	return p.validateDependencyGraph(nodes)
}

func (p TestTaskVersionPlan) validateDependencyGraph(nodes map[string]bool) error {
	byVersion := map[string]WorkflowDependencySnapshot{}
	for _, dependency := range p.Workflows {
		if _, duplicate := byVersion[dependency.Version.ID]; duplicate {
			return errors.New("duplicate workflow version dependency")
		}
		byVersion[dependency.Version.ID] = dependency
	}
	references := map[string]WorkflowReferenceResolution{}
	for _, resolution := range p.References {
		key := resolution.ParentWorkflowVersionID + "\x00" + resolution.StepID
		if resolution.ParentWorkflowVersionID == "" || resolution.StepID == "" ||
			resolution.WorkflowID == "" || resolution.WorkflowVersionID == "" {
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
		var walk func([]WorkflowStep) error
		walk = func(steps []WorkflowStep) error {
			for _, step := range steps {
				if step.NodeID != "" {
					key := NodeDependencyIdentity(step.NodeID, step.NodeVersionID)
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
					if !ok || target.Workflow.ID != resolution.WorkflowID || resolution.WorkflowID != step.Reference.WorkflowID {
						return fmt.Errorf("step %s workflow reference target is inconsistent", step.ID)
					}
					if step.Reference.LatestPublished {
						if !resolution.ResolvedFromLatest || target.Workflow.CurrentVersionID != target.Version.ID {
							return fmt.Errorf("step %s latest workflow reference was not resolved from current", step.ID)
						}
					} else if resolution.ResolvedFromLatest || step.Reference.WorkflowVersionID != target.Version.ID {
						return fmt.Errorf("step %s fixed workflow reference changed version", step.ID)
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
			if dependency.Workflow.ID != item.WorkflowID {
				continue
			}
			if item.VersionPolicy == WorkflowVersionFixed && dependency.Version.ID != item.WorkflowVersionID {
				continue
			}
			if item.VersionPolicy == WorkflowVersionLatest && !dependency.ResolvedFromLatest {
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

func NodeDependencyIdentity(nodeID, versionID string) string {
	return nodeID + "\x00" + versionID
}
