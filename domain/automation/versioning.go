package automation

import (
	"github.com/Capsule7446/healix-core/domain/parameter"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// ValidateLoadedHistory 验证细节/水合作用形状。列表查询故意允许省略版本，因此不调用它。
// It returns a single-violation AUTOMATION_ELEMENT_TARGET_HISTORY_INVALID
// envelope: the short-circuit is deterministic (the walk is over the versions
// slice in order, never a map), and no version identity reaches public text.
func (a ElementTargetAggregate) ValidateLoadedHistory() error {
	if a.ElementTarget.CurrentVersionID == "" {
		if a.Current.ID != "" {
			return elementTargetHistoryInvalidError(mustViolation(fault.CodeFieldMismatch, "currentVersionId", "an element target without a current pointer cannot carry a current version"))
		}
		for _, version := range a.Versions {
			if version.ElementTargetID != a.ElementTarget.ID {
				return elementTargetHistoryInvalidError(mustViolation(fault.CodeFieldMismatch, "versions", "a history version belongs to another element target"))
			}
			if version.DeletedAt == 0 {
				return elementTargetHistoryInvalidError(mustViolation(fault.CodeFieldRequired, "currentVersionId", "an element target with an available version requires a current pointer"))
			}
		}
		if violation, invalid := validateVersionIdentity(nodeVersionIdentities(a.Versions)); invalid {
			return elementTargetHistoryInvalidError(violation)
		}
		return nil
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if violation, invalid := validateVersionIdentity(nodeVersionIdentities(a.Versions)); invalid {
		return elementTargetHistoryInvalidError(violation)
	}
	found := false
	for _, version := range a.Versions {
		if version.ElementTargetID != a.ElementTarget.ID {
			return elementTargetHistoryInvalidError(mustViolation(fault.CodeFieldMismatch, "versions", "a history version belongs to another element target"))
		}
		if version.ID == a.Current.ID {
			found = true
		}
	}
	if !found {
		return elementTargetHistoryInvalidError(mustViolation(fault.CodeFieldMismatch, "currentVersionId", "the current version is missing from the loaded history"))
	}
	return nil
}

// ValidateLoadedHistory mirrors ElementTargetAggregate.ValidateLoadedHistory
// for the flow fragment aggregate family.
func (a FlowFragmentAggregate) ValidateLoadedHistory() error {
	if a.FlowFragment.CurrentVersionID == "" {
		if a.Current.ID != "" {
			return flowFragmentHistoryInvalidError(mustViolation(fault.CodeFieldMismatch, "currentVersionId", "a flow fragment without a current pointer cannot carry a current version"))
		}
		for _, version := range a.Versions {
			if version.FlowFragmentID != a.FlowFragment.ID {
				return flowFragmentHistoryInvalidError(mustViolation(fault.CodeFieldMismatch, "versions", "a history version belongs to another flow fragment"))
			}
			if version.DeletedAt == 0 {
				return flowFragmentHistoryInvalidError(mustViolation(fault.CodeFieldRequired, "currentVersionId", "a flow fragment with an available version requires a current pointer"))
			}
		}
		if violation, invalid := validateVersionIdentity(workflowVersionIdentities(a.Versions)); invalid {
			return flowFragmentHistoryInvalidError(violation)
		}
		return nil
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if violation, invalid := validateVersionIdentity(workflowVersionIdentities(a.Versions)); invalid {
		return flowFragmentHistoryInvalidError(violation)
	}
	found := false
	for _, version := range a.Versions {
		if version.FlowFragmentID != a.FlowFragment.ID {
			return flowFragmentHistoryInvalidError(mustViolation(fault.CodeFieldMismatch, "versions", "a history version belongs to another flow fragment"))
		}
		if version.ID == a.Current.ID {
			found = true
		}
	}
	if !found {
		return flowFragmentHistoryInvalidError(mustViolation(fault.CodeFieldMismatch, "currentVersionId", "the current version is missing from the loaded history"))
	}
	return nil
}

type versionIdentity struct {
	id     string
	number int
}

func nodeVersionIdentities(versions []ElementTargetVersion) []versionIdentity {
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

// validateVersionIdentity reports the first identity problem found by walking
// the versions slice in order — never a map — so the result is a function of
// the input alone. No version id reaches public text.
func validateVersionIdentity(versions []versionIdentity) (fault.Violation, bool) {
	seenIDs := map[string]bool{}
	seenNumbers := map[int]bool{}
	numbers := make([]int, 0, len(versions))
	for _, version := range versions {
		if strings.TrimSpace(version.id) == "" || version.number < 1 {
			return mustViolation(fault.CodeFieldInvalid, "versions", "history contains an invalid version identity"), true
		}
		if seenIDs[version.id] || seenNumbers[version.number] {
			return mustViolation(fault.CodeFieldDuplicate, "versions", "history contains a duplicate version identity"), true
		}
		seenIDs[version.id] = true
		seenNumbers[version.number] = true
		numbers = append(numbers, version.number)
	}
	sort.Ints(numbers)
	for index, number := range numbers {
		if number != index+1 {
			return mustViolation(fault.CodeFieldInvalid, "versions", "history version numbers must be contiguous from 1"), true
		}
	}
	return fault.Violation{}, false
}

// PublishVersion 创建一个新的不可变 ElementTargetVersion 并返回一个新的聚合值。现有的历史和接收者永远不会改变。
func (a ElementTargetAggregate) PublishVersion(versionID, pageURL, origin string, selectors []fingerprint.Selector,
	fp fingerprint.Fingerprint, source VersionSource, at int64) (ElementTargetAggregate, error) {
	if err := validateNodePublicationBase(a, versionID, at); err != nil {
		return ElementTargetAggregate{}, err
	}
	next := cloneNodeAggregate(a)
	nextRevision, err := a.ElementTarget.Revision.Next()
	if err != nil {
		// Revision.Next already returns AUTOMATION_REVISION_EXHAUSTED. The wrapper
		// this replaces welded the aggregate id into fresh public text on top of an
		// already-classified fault.
		return ElementTargetAggregate{}, err
	}
	versionNumber, err := nextNodeVersion(a)
	if err != nil {
		// NextVersionNumber already returns AUTOMATION_VERSION_NUMBER_EXHAUSTED.
		return ElementTargetAggregate{}, err
	}
	version := ElementTargetVersion{ID: versionID, ElementTargetID: a.ElementTarget.ID, VersionNumber: versionNumber,
		PageURL: pageURL, Origin: origin, Selectors: append([]fingerprint.Selector(nil), selectors...),
		Fingerprint: cloneFingerprint(fp), Source: source, CreatedAt: at}
	next.ElementTarget.CurrentVersionID = version.ID
	next.ElementTarget.UpdatedAt = at
	next.ElementTarget.Revision = nextRevision
	next.Current = cloneNodeVersion(version)
	next.Versions = append(next.Versions, cloneNodeVersion(version))
	if err := next.Validate(); err != nil {
		// Validate already returns AUTOMATION_ELEMENT_TARGET_INVALID.
		return ElementTargetAggregate{}, err
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
		return FlowFragmentAggregate{}, err
	}
	versionNumber, err := nextWorkflowVersion(a)
	if err != nil {
		// NextVersionNumber already returns AUTOMATION_VERSION_NUMBER_EXHAUSTED.
		return FlowFragmentAggregate{}, err
	}
	version := FlowFragmentVersion{ID: versionID, FlowFragmentID: a.FlowFragment.ID,
		VersionNumber: versionNumber, Definition: cloneWorkflowDefinition(definition), CreatedAt: at}
	next.FlowFragment.CurrentVersionID = version.ID
	next.FlowFragment.UpdatedAt = at
	next.FlowFragment.Revision = nextRevision
	next.Current = cloneWorkflowVersion(version)
	next.Versions = append(next.Versions, cloneWorkflowVersion(version))
	if err := next.Validate(); err != nil {
		// Validate already returns AUTOMATION_FLOW_FRAGMENT_INVALID.
		return FlowFragmentAggregate{}, err
	}
	return next, nil
}

// validateNodePublicationBase reports the shared publication preconditions.
// A content failure from a.Validate() and a timing failure from
// validateTransitionTime already carry their own registered code and pass
// through unwrapped; only the pointer-consistency and identity checks that are
// specific to publication mint their own AUTOMATION_ELEMENT_TARGET_HISTORY_INVALID
// violation.
func validateNodePublicationBase(a ElementTargetAggregate, versionID string, at int64) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a.ElementTarget.CurrentVersionID != a.Current.ID {
		return elementTargetHistoryInvalidError(mustViolation(fault.CodeFieldMismatch, "currentVersionId", "current version pointer is inconsistent"))
	}
	if a.ElementTarget.DeletedAt != 0 {
		return DeletedAggregateError()
	}
	if err := validateTransitionTime(at, a.ElementTarget.UpdatedAt); err != nil {
		return err
	}
	return validateNewVersionIdentity(versionID, at, a.Current.ID, nodeVersionIDs(a.Versions), elementTargetHistoryInvalidError)
}

// validateWorkflowPublicationBase mirrors validateNodePublicationBase for the
// flow fragment aggregate family.
func validateWorkflowPublicationBase(a FlowFragmentAggregate, versionID string, at int64) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a.FlowFragment.CurrentVersionID != a.Current.ID {
		return flowFragmentHistoryInvalidError(mustViolation(fault.CodeFieldMismatch, "currentVersionId", "current version pointer is inconsistent"))
	}
	if a.FlowFragment.DeletedAt != 0 {
		return DeletedAggregateError()
	}
	if err := validateTransitionTime(at, a.FlowFragment.UpdatedAt); err != nil {
		return err
	}
	return validateNewVersionIdentity(versionID, at, a.Current.ID, workflowVersionIDs(a.Versions), flowFragmentHistoryInvalidError)
}

// validateNewVersionIdentity is shared by both aggregate families; wrap builds
// the family-specific history envelope around the single violation found.
func validateNewVersionIdentity(versionID string, at int64, currentID string, existing []string, wrap func(...fault.Violation) error) error {
	if strings.TrimSpace(versionID) == "" {
		return wrap(mustViolation(fault.CodeFieldRequired, "versionId", "new version id is required"))
	}
	if at <= 0 {
		return wrap(mustViolation(fault.CodeFieldInvalid, "publishedAt", "publication time must be positive"))
	}
	if versionID == currentID {
		return wrap(mustViolation(fault.CodeFieldInvalid, "versionId", "new version id must differ from the current version"))
	}
	for _, id := range existing {
		if versionID == id {
			return wrap(mustViolation(fault.CodeFieldDuplicate, "versionId", "new version id already exists in history"))
		}
	}
	return nil
}

func nextNodeVersion(a ElementTargetAggregate) (int, error) {
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

func nodeVersionIDs(versions []ElementTargetVersion) []string {
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

func (a ElementTargetAggregate) Clone() ElementTargetAggregate {
	return cloneNodeAggregate(a)
}

func cloneNodeAggregate(input ElementTargetAggregate) ElementTargetAggregate {
	result := input
	result.ElementTarget.Properties = input.ElementTarget.Properties.Clone()
	result.Current = cloneNodeVersion(input.Current)
	result.Versions = make([]ElementTargetVersion, len(input.Versions))
	for index, version := range input.Versions {
		result.Versions[index] = cloneNodeVersion(version)
	}
	return result
}

func cloneNodeVersion(input ElementTargetVersion) ElementTargetVersion {
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
