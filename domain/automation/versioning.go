package automation

import (
	"errors"
	"fmt"
	"github.com/Capsule7446/healix-core/domain/parameter"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// ValidateLoadedHistory 验证细节/水合作用形状。列表查询故意允许省略版本，因此不调用它。
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

func (a FlowFragmentAggregate) ValidateLoadedHistory() error {
	if a.FlowFragment.CurrentVersionID == "" {
		if a.Current.ID != "" {
			return errors.New("workflow without a current pointer cannot carry a current version")
		}
		for _, version := range a.Versions {
			if version.FlowFragmentID != a.FlowFragment.ID {
				return errors.New("workflow history version belongs to another workflow")
			}
			if version.DeletedAt == 0 {
				return errors.New("workflow with an available version requires a current pointer")
			}
		}
		return validateVersionIdentity(a.FlowFragment.ID, workflowVersionIdentities(a.Versions))
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if err := validateVersionIdentity(a.FlowFragment.ID, workflowVersionIdentities(a.Versions)); err != nil {
		return err
	}
	found := false
	for _, version := range a.Versions {
		if version.FlowFragmentID != a.FlowFragment.ID {
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

func workflowVersionIdentities(versions []FlowFragmentVersion) []versionIdentity {
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

// PublishVersion 创建一个新的不可变 NodeVersion 并返回一个新的聚合值。现有的历史和接收者永远不会改变。
func (a NodeAggregate) PublishVersion(versionID, pageURL, origin string, selectors []fingerprint.Selector,
	fp fingerprint.Fingerprint, source VersionSource, at int64) (NodeAggregate, error) {
	if err := validateNodePublicationBase(a, versionID, at); err != nil {
		return NodeAggregate{}, err
	}
	next := cloneNodeAggregate(a)
	nextRevision, err := a.Node.Revision.Next()
	if err != nil {
		return NodeAggregate{}, revisionError("node", a.Node.ID, err)
	}
	versionNumber, err := nextNodeVersion(a)
	if err != nil {
		return NodeAggregate{}, fmt.Errorf("publish node version: %w", err)
	}
	version := NodeVersion{ID: versionID, NodeID: a.Node.ID, VersionNumber: versionNumber,
		PageURL: pageURL, Origin: origin, Selectors: append([]fingerprint.Selector(nil), selectors...),
		Fingerprint: cloneFingerprint(fp), Source: source, CreatedAt: at}
	next.Node.CurrentVersionID = version.ID
	next.Node.UpdatedAt = at
	next.Node.Revision = nextRevision
	next.Current = cloneNodeVersion(version)
	next.Versions = append(next.Versions, cloneNodeVersion(version))
	if err := next.Validate(); err != nil {
		return NodeAggregate{}, fmt.Errorf("publish node version: %w", err)
	}
	return next, nil
}

// PublishVersion 创建一个新的不可变 FlowFragmentVersion。该定义是深度复制的，因此调用者拥有的编辑器切片不能改变已发布的历史记录。
func (a FlowFragmentAggregate) PublishVersion(versionID string, definition FlowFragmentContent, at int64) (FlowFragmentAggregate, error) {
	if err := validateWorkflowPublicationBase(a, versionID, at); err != nil {
		return FlowFragmentAggregate{}, err
	}
	next := cloneWorkflowAggregate(a)
	nextRevision, err := a.FlowFragment.Revision.Next()
	if err != nil {
		return FlowFragmentAggregate{}, revisionError("workflow", a.FlowFragment.ID, err)
	}
	versionNumber, err := nextWorkflowVersion(a)
	if err != nil {
		return FlowFragmentAggregate{}, fmt.Errorf("publish workflow version: %w", err)
	}
	version := FlowFragmentVersion{ID: versionID, FlowFragmentID: a.FlowFragment.ID,
		VersionNumber: versionNumber, Definition: cloneWorkflowDefinition(definition), CreatedAt: at}
	next.FlowFragment.CurrentVersionID = version.ID
	next.FlowFragment.UpdatedAt = at
	next.FlowFragment.Revision = nextRevision
	next.Current = cloneWorkflowVersion(version)
	next.Versions = append(next.Versions, cloneWorkflowVersion(version))
	if err := next.Validate(); err != nil {
		return FlowFragmentAggregate{}, fmt.Errorf("publish workflow version: %w", err)
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
	if a.Node.DeletedAt != 0 {
		return ErrDeletedAggregate
	}
	if err := validateTransitionTime(at, a.Node.UpdatedAt); err != nil {
		return err
	}
	return validateNewVersionIdentity(versionID, at, a.Current.ID, nodeVersionIDs(a.Versions))
}

func validateWorkflowPublicationBase(a FlowFragmentAggregate, versionID string, at int64) error {
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid current workflow aggregate: %w", err)
	}
	if a.FlowFragment.CurrentVersionID != a.Current.ID {
		return errors.New("workflow current version pointer is inconsistent")
	}
	if a.FlowFragment.DeletedAt != 0 {
		return ErrDeletedAggregate
	}
	if err := validateTransitionTime(at, a.FlowFragment.UpdatedAt); err != nil {
		return err
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

func nextNodeVersion(a NodeAggregate) (int, error) {
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

func nextWorkflowVersion(a FlowFragmentAggregate) (int, error) {
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

func workflowVersionIDs(versions []FlowFragmentVersion) []string {
	result := make([]string, len(versions))
	for index, version := range versions {
		result[index] = version.ID
	}
	return result
}

func (a NodeAggregate) Clone() NodeAggregate {
	return cloneNodeAggregate(a)
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
	result.Framework = input.Framework.Clone()
	result.Attributes = make(map[string]string, len(input.Attributes))
	for key, value := range input.Attributes {
		result.Attributes[key] = value
	}
	return result
}

func cloneWorkflowAggregate(input FlowFragmentAggregate) FlowFragmentAggregate {
	result := input
	result.FlowFragment.Properties = input.FlowFragment.Properties.Clone()
	result.Current = cloneWorkflowVersion(input.Current)
	result.Versions = make([]FlowFragmentVersion, len(input.Versions))
	for index, version := range input.Versions {
		result.Versions[index] = cloneWorkflowVersion(version)
	}
	return result
}

func cloneWorkflowVersion(input FlowFragmentVersion) FlowFragmentVersion {
	result := input
	result.Definition = cloneWorkflowDefinition(input.Definition)
	return result
}

func cloneWorkflowDefinition(input FlowFragmentContent) FlowFragmentContent {
	result := FlowFragmentContent{Steps: clonePublishedWorkflowSteps(input.Steps),
		Parameters: append([]ParameterDefinition(nil), input.Parameters...)}
	for index := range result.Parameters {
		result.Parameters[index].Options = append([]string(nil), input.Parameters[index].Options...)
		if value, present := input.Parameters[index].Default.Value(); present {
			result.Parameters[index].Default = parameter.PresentValue(value)
		}
	}
	return result
}

func clonePublishedWorkflowSteps(input []FlowFragmentStep) []FlowFragmentStep {
	result := make([]FlowFragmentStep, len(input))
	for index, step := range input {
		copy := step
		copy.Values = append([]string(nil), step.Values...)
		copy.Children = clonePublishedWorkflowSteps(step.Children)
		if step.Reference != nil {
			reference := *step.Reference
			reference.ParameterBindings = cloneParameterBindings(step.Reference.ParameterBindings)
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
