package automation

import (
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// CodeDeletedAggregate 表示已删除自动化聚合错误。
const CodeDeletedAggregate fault.Code = "AUTOMATION_AGGREGATE_DELETED"

// DeletedAggregateError 构造已删除聚合的前置条件错误。
func DeletedAggregateError() error {
	err, constructionErr := fault.New(
		fault.FailedPrecondition,
		CodeDeletedAggregate,
		"automation aggregate has been deleted",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// validateTransitionTime 校验生命周期转换时间为正且不早于聚合更新时间。
// 环境、元素目标、流程片段和执行流程共用此检查，并直接返回 AUTOMATION_AGGREGATE_TRANSITION_INVALID。
func validateTransitionTime(at, updatedAt int64) error {
	if at <= 0 {
		return aggregateTransitionInvalidError(mustViolation(fault.CodeFieldInvalid, "at", "transition time must be positive"))
	}
	if at < updatedAt {
		return aggregateTransitionInvalidError(mustViolation(fault.CodeFieldInvalid, "at", "transition time cannot precede the aggregate's updated time"))
	}
	return nil
}

// NewEnvironment 初始化环境修订和变量副本，并校验创建时间与环境内容。
func NewEnvironment(value Environment) (Environment, error) {
	value.Revision = 1
	if value.CreatedAt <= 0 || value.UpdatedAt != value.CreatedAt {
		return Environment{}, aggregateTransitionInvalidError(mustViolation(fault.CodeFieldInvalid, "createdAt", "environment creation time must be positive and equal updated time"))
	}
	value.Variables = value.Variables.Clone()
	if err := value.Validate(); err != nil {
		return Environment{}, err
	}
	return value, nil
}

// UpdateMetadata 返回应用元数据和变量后的环境副本，不修改接收值；删除聚合或修订耗尽时返回错误。
func (e Environment) UpdateMetadata(displayName, baseURL string, variables EnvironmentVariables, at int64) (Environment, error) {
	if e.DeletedAt != 0 {
		return Environment{}, DeletedAggregateError()
	}
	if err := validateTransitionTime(at, e.UpdatedAt); err != nil {
		return Environment{}, err
	}
	nextRevision, err := e.Revision.Next()
	if err != nil {
		return Environment{}, err
	}
	next := e
	next.DisplayName = displayName
	next.BaseURL = baseURL
	next.Variables = variables.Clone()
	next.UpdatedAt = at
	next.Revision = nextRevision
	if err := next.Validate(); err != nil {
		return Environment{}, err
	}
	return next, nil
}

// Delete 返回标记为已删除的环境副本，并递增修订。
func (e Environment) Delete(at int64) (Environment, error) { return setEnvironmentDeleted(e, true, at) }

// Restore 返回清除删除标记的环境副本，并递增修订。
func (e Environment) Restore(at int64) (Environment, error) {
	return setEnvironmentDeleted(e, false, at)
}

// setEnvironmentDeleted 返回应用删除状态后的环境副本，并校验时间和修订。
func setEnvironmentDeleted(e Environment, deleted bool, at int64) (Environment, error) {
	if (e.DeletedAt != 0) == deleted {
		return Environment{}, aggregateTransitionInvalidError(mustViolation(fault.CodeFieldInvalid, "deletedAt", "environment lifecycle transition is a no-op"))
	}
	if err := validateTransitionTime(at, e.UpdatedAt); err != nil {
		return Environment{}, err
	}
	r, err := e.Revision.Next()
	if err != nil {
		return Environment{}, err
	}
	next := e
	next.DeletedAt = 0
	if deleted {
		next.DeletedAt = at
	}
	next.UpdatedAt = at
	next.Revision = r
	next.Variables = e.Variables.Clone()
	if err := next.Validate(); err != nil {
		return Environment{}, err
	}
	return next, nil
}

// NewElementTarget 初始化元素目标聚合及其第一个版本，并复制引用字段。
func NewElementTarget(node ElementTarget, initial ElementTargetVersion) (ElementTargetAggregate, error) {
	node.Revision = 1
	node.CurrentVersionID = initial.ID
	initial.ElementTargetID = node.ID
	initial.VersionNumber = 1
	if node.CreatedAt <= 0 || node.UpdatedAt != node.CreatedAt || initial.CreatedAt != node.CreatedAt {
		return ElementTargetAggregate{}, aggregateTransitionInvalidError(mustViolation(fault.CodeFieldInvalid, "createdAt", "element target creation timestamps must be positive and equal"))
	}
	a := ElementTargetAggregate{ElementTarget: node, Current: initial, Versions: []ElementTargetVersion{initial}}
	a = cloneNodeAggregate(a)
	if err := a.ValidateLoadedHistory(); err != nil {
		return ElementTargetAggregate{}, err
	}
	return a, nil
}

// UpdateMetadata 返回应用元素目标元数据后的聚合副本，不修改接收值。
func (a ElementTargetAggregate) UpdateMetadata(displayName, folderID string, properties Properties, at int64) (ElementTargetAggregate, error) {
	if a.ElementTarget.DeletedAt != 0 {
		return ElementTargetAggregate{}, DeletedAggregateError()
	}
	if err := validateTransitionTime(at, a.ElementTarget.UpdatedAt); err != nil {
		return ElementTargetAggregate{}, err
	}
	r, err := a.ElementTarget.Revision.Next()
	if err != nil {
		return ElementTargetAggregate{}, err
	}
	n := cloneNodeAggregate(a)
	n.ElementTarget.DisplayName = displayName
	n.ElementTarget.FolderID = folderID
	n.ElementTarget.Properties = properties.Clone()
	n.ElementTarget.UpdatedAt = at
	n.ElementTarget.Revision = r
	if err := n.ValidateLoadedHistory(); err != nil {
		return ElementTargetAggregate{}, err
	}
	return n, nil
}

// Delete 返回标记为已删除的元素目标聚合副本。
func (a ElementTargetAggregate) Delete(at int64) (ElementTargetAggregate, error) {
	return a.setDeleted(true, at)
}

// Restore 返回清除删除标记的元素目标聚合副本。
func (a ElementTargetAggregate) Restore(at int64) (ElementTargetAggregate, error) {
	return a.setDeleted(false, at)
}

// setDeleted 返回应用删除状态后的元素目标聚合副本，并校验时间和修订。
func (a ElementTargetAggregate) setDeleted(deleted bool, at int64) (ElementTargetAggregate, error) {
	if (a.ElementTarget.DeletedAt != 0) == deleted {
		return ElementTargetAggregate{}, aggregateTransitionInvalidError(mustViolation(fault.CodeFieldInvalid, "deletedAt", "element target lifecycle transition is a no-op"))
	}
	if err := validateTransitionTime(at, a.ElementTarget.UpdatedAt); err != nil {
		return ElementTargetAggregate{}, err
	}
	r, err := a.ElementTarget.Revision.Next()
	if err != nil {
		return ElementTargetAggregate{}, err
	}
	n := cloneNodeAggregate(a)
	n.ElementTarget.DeletedAt = 0
	if deleted {
		n.ElementTarget.DeletedAt = at
	}
	n.ElementTarget.UpdatedAt = at
	n.ElementTarget.Revision = r
	if err := n.ValidateLoadedHistory(); err != nil {
		return ElementTargetAggregate{}, err
	}
	return n, nil
}

// NewFlowFragment 初始化流程片段聚合及其第一个版本，并复制引用字段。
func NewFlowFragment(workflow FlowFragment, initial FlowFragmentVersion) (FlowFragmentAggregate, error) {
	workflow.Revision = 1
	workflow.CurrentVersionID = initial.ID
	initial.FlowFragmentID = workflow.ID
	initial.VersionNumber = 1
	if workflow.CreatedAt <= 0 || workflow.UpdatedAt != workflow.CreatedAt || initial.CreatedAt != workflow.CreatedAt {
		return FlowFragmentAggregate{}, aggregateTransitionInvalidError(mustViolation(fault.CodeFieldInvalid, "createdAt", "flow fragment creation timestamps must be positive and equal"))
	}
	a := cloneWorkflowAggregate(FlowFragmentAggregate{FlowFragment: workflow, Current: initial, Versions: []FlowFragmentVersion{initial}})
	if err := a.ValidateLoadedHistory(); err != nil {
		return FlowFragmentAggregate{}, err
	}
	return a, nil
}

// UpdateMetadata 返回应用流程片段元数据后的聚合副本，不修改接收值。
func (a FlowFragmentAggregate) UpdateMetadata(displayName, folderID string, properties Properties, at int64) (FlowFragmentAggregate, error) {
	if a.FlowFragment.DeletedAt != 0 {
		return FlowFragmentAggregate{}, DeletedAggregateError()
	}
	if err := validateTransitionTime(at, a.FlowFragment.UpdatedAt); err != nil {
		return FlowFragmentAggregate{}, err
	}
	r, err := a.FlowFragment.Revision.Next()
	if err != nil {
		return FlowFragmentAggregate{}, err
	}
	n := cloneWorkflowAggregate(a)
	n.FlowFragment.DisplayName = displayName
	n.FlowFragment.FolderID = folderID
	n.FlowFragment.Properties = properties.Clone()
	n.FlowFragment.UpdatedAt = at
	n.FlowFragment.Revision = r
	if err := n.ValidateLoadedHistory(); err != nil {
		return FlowFragmentAggregate{}, err
	}
	return n, nil
}

// Delete 返回标记为已删除的流程片段聚合副本。
func (a FlowFragmentAggregate) Delete(at int64) (FlowFragmentAggregate, error) {
	return a.setDeleted(true, at)
}

// Restore 返回清除删除标记的流程片段聚合副本。
func (a FlowFragmentAggregate) Restore(at int64) (FlowFragmentAggregate, error) {
	return a.setDeleted(false, at)
}

// setDeleted 返回应用删除状态后的流程片段聚合副本，并校验时间和修订。
func (a FlowFragmentAggregate) setDeleted(deleted bool, at int64) (FlowFragmentAggregate, error) {
	if (a.FlowFragment.DeletedAt != 0) == deleted {
		return FlowFragmentAggregate{}, aggregateTransitionInvalidError(mustViolation(fault.CodeFieldInvalid, "deletedAt", "flow fragment lifecycle transition is a no-op"))
	}
	if err := validateTransitionTime(at, a.FlowFragment.UpdatedAt); err != nil {
		return FlowFragmentAggregate{}, err
	}
	r, err := a.FlowFragment.Revision.Next()
	if err != nil {
		return FlowFragmentAggregate{}, err
	}
	n := cloneWorkflowAggregate(a)
	n.FlowFragment.DeletedAt = 0
	if deleted {
		n.FlowFragment.DeletedAt = at
	}
	n.FlowFragment.UpdatedAt = at
	n.FlowFragment.Revision = r
	if err := n.ValidateLoadedHistory(); err != nil {
		return FlowFragmentAggregate{}, err
	}
	return n, nil
}

// PublishVersion 校验并发布执行流程新版本，保持来源身份、单调时间和修订一致。
func (a ExecutionFlowAggregate) PublishVersion(publication ExecutionFlowVersionPublication) (ExecutionFlowAggregate, error) {
	if err := a.Validate(); err != nil {
		return ExecutionFlowAggregate{}, err
	}
	if a.Task.DeletedAt != 0 {
		return ExecutionFlowAggregate{}, DeletedAggregateError()
	}
	if strings.TrimSpace(publication.ID) == "" || publication.CreatedAt <= 0 || publication.CreatedAt < a.Task.UpdatedAt {
		return ExecutionFlowAggregate{}, aggregateTransitionInvalidError(mustViolation(fault.CodeFieldInvalid, "publication", "publication requires a new version identity and a monotonic timestamp"))
	}
	for _, existing := range a.Versions {
		if existing.ID == publication.ID {
			return ExecutionFlowAggregate{}, aggregateTransitionInvalidError(mustViolation(fault.CodeFieldDuplicate, "publication.id", "publication version id already exists"))
		}
	}
	next := cloneTestTaskAggregate(a)
	version := cloneTestTaskVersion(ExecutionFlowVersion{
		ID:                      publication.ID,
		ExecutionFlowID:         a.Task.ID,
		VersionNumber:           len(a.Versions) + 1,
		SourceVersionID:         a.Current.ID,
		Items:                   publication.Items,
		FailurePolicy:           publication.FailurePolicy,
		RequiredEnvironmentKeys: publication.RequiredEnvironmentKeys,
		CreatedAt:               publication.CreatedAt,
	})
	for index := range version.Items {
		version.Items[index].TestTaskVersionID = version.ID
		version.Items[index].SequenceNumber = index + 1
	}
	nextRevision, err := a.Task.Revision.Next()
	if err != nil {
		return ExecutionFlowAggregate{}, err
	}
	next.Task.CurrentVersionID = version.ID
	next.Task.UpdatedAt = version.CreatedAt
	next.Task.Revision = nextRevision
	next.Current = cloneTestTaskVersion(version)
	next.Versions = append(next.Versions, cloneTestTaskVersion(version))
	if err := next.Validate(); err != nil {
		return ExecutionFlowAggregate{}, err
	}
	return next, nil
}

// NewExecutionFlow 初始化执行流程聚合及其第一个版本。
func NewExecutionFlow(task ExecutionFlow, initial ExecutionFlowVersion) (ExecutionFlowAggregate, error) {
	task.Revision = 1
	task.CurrentVersionID = initial.ID
	initial.ExecutionFlowID = task.ID
	initial.VersionNumber = 1
	initial.SourceVersionID = ""
	if task.CreatedAt <= 0 || task.UpdatedAt != task.CreatedAt || initial.CreatedAt != task.CreatedAt {
		return ExecutionFlowAggregate{}, aggregateTransitionInvalidError(mustViolation(fault.CodeFieldInvalid, "createdAt", "execution flow creation timestamps must be positive and equal"))
	}
	aggregate := cloneTestTaskAggregate(ExecutionFlowAggregate{Task: task, Current: initial, Versions: []ExecutionFlowVersion{initial}})
	if err := aggregate.Validate(); err != nil {
		return ExecutionFlowAggregate{}, err
	}
	return aggregate, nil
}

// cloneTestTaskAggregate 返回执行流程聚合的独立副本。
func cloneTestTaskAggregate(aggregate ExecutionFlowAggregate) ExecutionFlowAggregate {
	cloned := aggregate
	cloned.Current = cloneTestTaskVersion(aggregate.Current)
	cloned.Versions = make([]ExecutionFlowVersion, len(aggregate.Versions))
	for index, version := range aggregate.Versions {
		cloned.Versions[index] = cloneTestTaskVersion(version)
	}
	return cloned
}

// cloneTestTaskVersion 返回执行流程版本及其条目的独立副本。
func cloneTestTaskVersion(version ExecutionFlowVersion) ExecutionFlowVersion {
	cloned := version
	cloned.RequiredEnvironmentKeys = append([]string(nil), version.RequiredEnvironmentKeys...)
	cloned.Items = make([]ExecutionFlowItem, len(version.Items))
	for index, item := range version.Items {
		cloned.Items[index] = item
		cloned.Items[index].Parameters = cloneParameterValues(item.Parameters)
	}
	return cloned
}

// cloneParameterBindings 返回参数绑定映射的独立副本；nil 输入保持 nil。
func cloneParameterBindings(values map[string]parameter.Binding) map[string]parameter.Binding {
	if values == nil {
		return nil
	}
	cloned := make(map[string]parameter.Binding, len(values))
	for key, binding := range values {
		cloned[key] = binding.Clone()
	}
	return cloned
}

// cloneParameterValues 返回参数值映射的独立副本；nil 输入保持 nil。
func cloneParameterValues(values map[string]parameter.Value) map[string]parameter.Value {
	if values == nil {
		return nil
	}
	cloned := make(map[string]parameter.Value, len(values))
	for key, value := range values {
		cloned[key] = value.Clone()
	}
	return cloned
}
