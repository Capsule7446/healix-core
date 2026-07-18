package workspace

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// ValidateLoadedHistory validates the detail/hydration shape. List queries are
// intentionally allowed to omit Versions and therefore do not call it.
func (a NodeAggregate) ValidateLoadedHistory() error {
	if a.Node.CurrentVersionID == "" {
		if a.Current.ID != "" {
			return errors.New("node without a current pointer cannot carry a current version")
		}
		for _, version := range a.Versions {
			if version.NodeID != a.Node.ID {
				return errors.New("node history version belongs to another node")
			}
			if version.DeletedAt == 0 {
				return errors.New("node with an available version requires a current pointer")
			}
		}
		return validateVersionIdentity(a.Node.ID, nodeVersionIdentities(a.Versions))
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if err := validateVersionIdentity(a.Node.ID, nodeVersionIdentities(a.Versions)); err != nil {
		return err
	}
	found := false
	for _, version := range a.Versions {
		if version.NodeID != a.Node.ID {
			return errors.New("node history version belongs to another node")
		}
		if version.ID == a.Current.ID {
			found = true
		}
	}
	if !found {
		return errors.New("node current version is missing from loaded history")
	}
	return nil
}

func (a WorkflowAggregate) ValidateLoadedHistory() error {
	if a.Workflow.CurrentVersionID == "" {
		if a.Current.ID != "" {
			return errors.New("workflow without a current pointer cannot carry a current version")
		}
		for _, version := range a.Versions {
			if version.WorkflowID != a.Workflow.ID {
				return errors.New("workflow history version belongs to another workflow")
			}
			if version.DeletedAt == 0 {
				return errors.New("workflow with an available version requires a current pointer")
			}
		}
		return validateVersionIdentity(a.Workflow.ID, workflowVersionIdentities(a.Versions))
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if err := validateVersionIdentity(a.Workflow.ID, workflowVersionIdentities(a.Versions)); err != nil {
		return err
	}
	found := false
	for _, version := range a.Versions {
		if version.WorkflowID != a.Workflow.ID {
			return errors.New("workflow history version belongs to another workflow")
		}
		if version.ID == a.Current.ID {
			found = true
		}
	}
	if !found {
		return errors.New("workflow current version is missing from loaded history")
	}
	return nil
}

type versionIdentity struct {
	id     string
	number int
}

func nodeVersionIdentities(versions []NodeVersion) []versionIdentity {
	result := make([]versionIdentity, len(versions))
	for i, version := range versions {
		result[i] = versionIdentity{id: version.ID, number: version.VersionNumber}
	}
	return result
}

func workflowVersionIdentities(versions []WorkflowVersion) []versionIdentity {
	result := make([]versionIdentity, len(versions))
	for i, version := range versions {
		result[i] = versionIdentity{id: version.ID, number: version.VersionNumber}
	}
	return result
}

func validateVersionIdentity(owner string, versions []versionIdentity) error {
	seenIDs := map[string]bool{}
	seenNumbers := map[int]bool{}
	numbers := make([]int, 0, len(versions))
	for _, version := range versions {
		if strings.TrimSpace(version.id) == "" || version.number < 1 {
			return fmt.Errorf("%s history contains an invalid version identity", owner)
		}
		if seenIDs[version.id] || seenNumbers[version.number] {
			return fmt.Errorf("%s history contains duplicate version identity", owner)
		}
		seenIDs[version.id] = true
		seenNumbers[version.number] = true
		numbers = append(numbers, version.number)
	}
	sort.Ints(numbers)
	for index, number := range numbers {
		if number != index+1 {
			return fmt.Errorf("%s history version numbers must be contiguous from 1", owner)
		}
	}
	return nil
}

// PublishVersion creates a new immutable NodeVersion and returns a new
// aggregate value. Existing history and the receiver are never mutated.
func (a NodeAggregate) PublishVersion(versionID, pageURL, origin string, selectors []fingerprint.Selector,
	fp fingerprint.Fingerprint, source VersionSource, at int64) (NodeAggregate, error) {
	if err := validateNodePublicationBase(a, versionID, at); err != nil {
		return NodeAggregate{}, err
	}
	next := cloneNodeAggregate(a)
	version := NodeVersion{ID: versionID, NodeID: a.Node.ID, VersionNumber: nextNodeVersion(a),
		PageURL: pageURL, Origin: origin, Selectors: append([]fingerprint.Selector(nil), selectors...),
		Fingerprint: cloneFingerprint(fp), Source: source, CreatedAt: at}
	next.Node.CurrentVersionID = version.ID
	next.Node.UpdatedAt = at
	next.Current = cloneNodeVersion(version)
	next.Versions = append(next.Versions, cloneNodeVersion(version))
	if err := next.Validate(); err != nil {
		return NodeAggregate{}, fmt.Errorf("publish node version: %w", err)
	}
	return next, nil
}

// PublishVersion creates a new immutable WorkflowVersion. The definition is
// deep-copied so caller-owned editor slices cannot mutate published history.
func (a WorkflowAggregate) PublishVersion(versionID string, definition WorkflowDefinition, at int64) (WorkflowAggregate, error) {
	if err := validateWorkflowPublicationBase(a, versionID, at); err != nil {
		return WorkflowAggregate{}, err
	}
	next := cloneWorkflowAggregate(a)
	version := WorkflowVersion{ID: versionID, WorkflowID: a.Workflow.ID,
		VersionNumber: nextWorkflowVersion(a), Definition: cloneWorkflowDefinition(definition), CreatedAt: at}
	next.Workflow.CurrentVersionID = version.ID
	next.Workflow.UpdatedAt = at
	next.Current = cloneWorkflowVersion(version)
	next.Versions = append(next.Versions, cloneWorkflowVersion(version))
	if err := next.Validate(); err != nil {
		return WorkflowAggregate{}, fmt.Errorf("publish workflow version: %w", err)
	}
	return next, nil
}

func validateNodePublicationBase(a NodeAggregate, versionID string, at int64) error {
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid current node aggregate: %w", err)
	}
	if a.Node.CurrentVersionID != a.Current.ID {
		return errors.New("node current version pointer is inconsistent")
	}
	return validateNewVersionIdentity(versionID, at, a.Current.ID, nodeVersionIDs(a.Versions))
}

func validateWorkflowPublicationBase(a WorkflowAggregate, versionID string, at int64) error {
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid current workflow aggregate: %w", err)
	}
	if a.Workflow.CurrentVersionID != a.Current.ID {
		return errors.New("workflow current version pointer is inconsistent")
	}
	return validateNewVersionIdentity(versionID, at, a.Current.ID, workflowVersionIDs(a.Versions))
}

func validateNewVersionIdentity(versionID string, at int64, currentID string, existing []string) error {
	if strings.TrimSpace(versionID) == "" {
		return errors.New("new version id is required")
	}
	if at <= 0 {
		return errors.New("publication time must be positive")
	}
	if versionID == currentID {
		return errors.New("new version id must differ from the current version")
	}
	for _, id := range existing {
		if versionID == id {
			return errors.New("new version id already exists in history")
		}
	}
	return nil
}

func nextNodeVersion(a NodeAggregate) int {
	metas := make([]VersionMeta, 0, len(a.Versions)+1)
	seenCurrent := false
	for _, version := range a.Versions {
		metas = append(metas, VersionMeta{ID: version.ID, VersionNumber: version.VersionNumber, DeletedAt: version.DeletedAt})
		seenCurrent = seenCurrent || version.ID == a.Current.ID
	}
	if !seenCurrent {
		metas = append(metas, VersionMeta{ID: a.Current.ID, VersionNumber: a.Current.VersionNumber, DeletedAt: a.Current.DeletedAt})
	}
	return NextVersionNumber(metas)
}

func nextWorkflowVersion(a WorkflowAggregate) int {
	metas := make([]VersionMeta, 0, len(a.Versions)+1)
	seenCurrent := false
	for _, version := range a.Versions {
		metas = append(metas, VersionMeta{ID: version.ID, VersionNumber: version.VersionNumber, DeletedAt: version.DeletedAt})
		seenCurrent = seenCurrent || version.ID == a.Current.ID
	}
	if !seenCurrent {
		metas = append(metas, VersionMeta{ID: a.Current.ID, VersionNumber: a.Current.VersionNumber, DeletedAt: a.Current.DeletedAt})
	}
	return NextVersionNumber(metas)
}

func nodeVersionIDs(versions []NodeVersion) []string {
	result := make([]string, len(versions))
	for index, version := range versions {
		result[index] = version.ID
	}
	return result
}

func workflowVersionIDs(versions []WorkflowVersion) []string {
	result := make([]string, len(versions))
	for index, version := range versions {
		result[index] = version.ID
	}
	return result
}

func cloneNodeAggregate(input NodeAggregate) NodeAggregate {
	result := input
	result.Node.Properties = input.Node.Properties.Clone()
	result.Current = cloneNodeVersion(input.Current)
	result.Versions = make([]NodeVersion, len(input.Versions))
	for index, version := range input.Versions {
		result.Versions[index] = cloneNodeVersion(version)
	}
	return result
}

func cloneNodeVersion(input NodeVersion) NodeVersion {
	result := input
	result.Selectors = append([]fingerprint.Selector(nil), input.Selectors...)
	result.Fingerprint = cloneFingerprint(input.Fingerprint)
	return result
}

func cloneFingerprint(input fingerprint.Fingerprint) fingerprint.Fingerprint {
	result := input
	result.Path = append([]string(nil), input.Path...)
	result.Attributes = make(map[string]string, len(input.Attributes))
	for key, value := range input.Attributes {
		result.Attributes[key] = value
	}
	return result
}

func cloneWorkflowAggregate(input WorkflowAggregate) WorkflowAggregate {
	result := input
	result.Workflow.Properties = input.Workflow.Properties.Clone()
	result.Current = cloneWorkflowVersion(input.Current)
	result.Versions = make([]WorkflowVersion, len(input.Versions))
	for index, version := range input.Versions {
		result.Versions[index] = cloneWorkflowVersion(version)
	}
	return result
}

func cloneWorkflowVersion(input WorkflowVersion) WorkflowVersion {
	result := input
	result.Definition = cloneWorkflowDefinition(input.Definition)
	return result
}

func cloneWorkflowDefinition(input WorkflowDefinition) WorkflowDefinition {
	result := WorkflowDefinition{Steps: clonePublishedWorkflowSteps(input.Steps),
		Parameters: append([]ParameterDefinition(nil), input.Parameters...)}
	for index := range result.Parameters {
		result.Parameters[index].Options = append([]string(nil), input.Parameters[index].Options...)
	}
	return result
}

func clonePublishedWorkflowSteps(input []WorkflowStep) []WorkflowStep {
	result := make([]WorkflowStep, len(input))
	for index, step := range input {
		copy := step
		copy.Values = append([]string(nil), step.Values...)
		copy.Children = clonePublishedWorkflowSteps(step.Children)
		if step.Reference != nil {
			reference := *step.Reference
			reference.ParameterBindings = make(map[string]string, len(step.Reference.ParameterBindings))
			for key, value := range step.Reference.ParameterBindings {
				reference.ParameterBindings[key] = value
			}
			copy.Reference = &reference
		}
		if step.Validation != nil {
			validation := *step.Validation
			validation.Assertion.ExpectedValues = append([]string(nil), step.Validation.Assertion.ExpectedValues...)
			validation.SupportedKinds = append([]ValidationAssertionKind(nil), step.Validation.SupportedKinds...)
			copy.Validation = &validation
		}
		if step.ValidationGroup != nil {
			group := *step.ValidationGroup
			group.Branches = make([]ValidationBranch, len(step.ValidationGroup.Branches))
			for branchIndex, branch := range step.ValidationGroup.Branches {
				group.Branches[branchIndex] = branch
				group.Branches[branchIndex].Steps = clonePublishedWorkflowSteps(branch.Steps)
			}
			copy.ValidationGroup = &group
		}
		result[index] = copy
	}
	return result
}
