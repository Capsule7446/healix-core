package automation

import (
	"github.com/Capsule7446/healix-core/domain/parameter"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// ValidateLoadedHistory 校验从持久化加载的元素目标历史形状；列表查询允许省略版本，因此不调用该方法。
// 失败返回单一违规的 AUTOMATION_ELEMENT_TARGET_HISTORY_INVALID 信封，遍历按版本切片顺序执行，
// 且不会把版本身份写入公共文本。
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

// ValidateLoadedHistory 对流程片段聚合执行与元素目标相同的持久化历史校验。
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

// versionIdentity 保存版本身份校验所需的 ID 和版本号。
type versionIdentity struct {
	id     string
	number int
}

// nodeVersionIdentities 提取元素目标版本的身份序列。
func nodeVersionIdentities(versions []ElementTargetVersion) []versionIdentity {
	result := make([]versionIdentity, len(versions))
	for i, version := range versions {
		result[i] = versionIdentity{id: version.ID, number: version.VersionNumber}
	}
	return result
}

// workflowVersionIdentities 提取流程片段版本的身份序列。
func workflowVersionIdentities(versions []FlowFragmentVersion) []versionIdentity {
	result := make([]versionIdentity, len(versions))
	for i, version := range versions {
		result[i] = versionIdentity{id: version.ID, number: version.VersionNumber}
	}
	return result
}

// validateVersionIdentity 按版本切片顺序返回首个身份问题；结果不依赖映射迭代顺序，版本 ID 不写入公共文本。
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
		// Revision.Next 已返回 AUTOMATION_REVISION_EXHAUSTED，此处保持原错误码和私有原因。
		return ElementTargetAggregate{}, err
	}
	versionNumber, err := nextNodeVersion(a)
	if err != nil {
		// NextVersionNumber 已返回 AUTOMATION_VERSION_NUMBER_EXHAUSTED，此处保持原错误码。
		return ElementTargetAggregate{}, err
	}
	version := ElementTargetVersion{ID: versionID, ElementTargetID: a.ElementTarget.ID, VersionNumber: versionNumber,
		PageURL: pageURL, Origin: origin, Selectors: append([]fingerprint.Selector(nil), selectors...),
		Fingerprint: fp.Clone(), Source: source, CreatedAt: at}
	next.ElementTarget.CurrentVersionID = version.ID
	next.ElementTarget.UpdatedAt = at
	next.ElementTarget.Revision = nextRevision
	next.Current = cloneNodeVersion(version)
	next.Versions = append(next.Versions, cloneNodeVersion(version))
	if err := next.Validate(); err != nil {
		// Validate 已返回 AUTOMATION_ELEMENT_TARGET_INVALID，此处保持原错误码。
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
		// NextVersionNumber 已返回 AUTOMATION_VERSION_NUMBER_EXHAUSTED，此处保持原错误码。
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
		// Validate 已返回 AUTOMATION_FLOW_FRAGMENT_INVALID，此处保持原错误码。
		return FlowFragmentAggregate{}, err
	}
	return next, nil
}

// validateNodePublicationBase 校验元素目标发布的共享前置条件。
// 内容和时间错误保持各自注册错误码；仅发布专属的当前指针一致性和新身份校验生成历史错误违规。
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

// validateWorkflowPublicationBase 对流程片段聚合执行与元素目标相同的发布前置校验。
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

// validateNewVersionIdentity 校验新版本身份、发布时间和历史唯一性；wrap 负责构造对应聚合的历史错误信封。
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

// nextNodeVersion 根据元素目标历史计算下一个版本号。
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

// nextWorkflowVersion 根据流程片段历史计算下一个版本号。
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

// nodeVersionIDs 提取元素目标版本 ID 列表。
func nodeVersionIDs(versions []ElementTargetVersion) []string {
	result := make([]string, len(versions))
	for index, version := range versions {
		result[index] = version.ID
	}
	return result
}

// workflowVersionIDs 提取流程片段版本 ID 列表。
func workflowVersionIDs(versions []FlowFragmentVersion) []string {
	result := make([]string, len(versions))
	for index, version := range versions {
		result[index] = version.ID
	}
	return result
}

// Clone 返回元素目标聚合的深复制，不与原聚合共享映射、切片或指纹引用。
func (a ElementTargetAggregate) Clone() ElementTargetAggregate {
	return cloneNodeAggregate(a)
}

// cloneNodeAggregate 返回元素目标聚合的独立副本。
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

// cloneNodeVersion 返回元素目标版本及其选择器、指纹的独立副本。
func cloneNodeVersion(input ElementTargetVersion) ElementTargetVersion {
	result := input
	result.Selectors = append([]fingerprint.Selector(nil), input.Selectors...)
	result.Fingerprint = input.Fingerprint.Clone()
	return result
}

// cloneWorkflowAggregate 返回流程片段聚合的独立副本。
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

// cloneWorkflowVersion 返回流程片段版本及其定义的独立副本。
func cloneWorkflowVersion(input FlowFragmentVersion) FlowFragmentVersion {
	result := input
	result.Definition = cloneWorkflowDefinition(input.Definition)
	return result
}

// cloneWorkflowDefinition 返回流程片段定义及其步骤、参数的独立副本。
func cloneWorkflowDefinition(input FlowFragmentContent) FlowFragmentContent {
	return FlowFragmentContent{Steps: CloneFlowFragmentSteps(input.Steps),
		Parameters: CloneParameterDefinitions(input.Parameters)}
}

// CloneParameterDefinitions 深复制参数定义内容，包括每个 Options 切片和默认值。
// 返回结果不与输入共享可变引用，发布后的不可变版本不会受编辑器切片修改影响。
func CloneParameterDefinitions(input []ParameterDefinition) []ParameterDefinition {
	if input == nil {
		return nil
	}
	result := append([]ParameterDefinition(nil), input...)
	for index := range result {
		result[index].Options = append([]string(nil), input[index].Options...)
		if value, present := input[index].Default.Value(); present {
			result[index].Default = parameter.PresentValue(value)
		}
	}
	return result
}

// CloneFlowFragmentSteps 深复制流程片段步骤内容，包括子步骤、引用绑定、断言期望值和验证组分支。
// 返回结果不与输入共享可变切片，调用方可独立编辑副本。
func CloneFlowFragmentSteps(input []FlowFragmentStep) []FlowFragmentStep {
	if input == nil {
		return nil
	}
	result := make([]FlowFragmentStep, len(input))
	for index, step := range input {
		copy := step
		copy.Values = append([]string(nil), step.Values...)
		copy.Children = CloneFlowFragmentSteps(step.Children)
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
				group.Branches[branchIndex].Steps = CloneFlowFragmentSteps(branch.Steps)
			}
			copy.ValidationGroup = &group
		}
		result[index] = copy
	}
	return result
}
