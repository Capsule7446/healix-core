package automation

import (
	"errors"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

const CodeDeletedAggregate fault.Code = "AUTOMATION_AGGREGATE_DELETED"

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

func validateTransitionTime(at, updatedAt int64) error {
	if at <= 0 {
		return errors.New("transition time must be positive")
	}
	if at < updatedAt {
		return errors.New("transition time cannot precede updated time")
	}
	return nil
}

func NewEnvironment(value Environment) (Environment, error) {
	value.Revision = 1
	if value.CreatedAt <= 0 || value.UpdatedAt != value.CreatedAt {
		return Environment{}, errors.New("environment creation time must be positive and equal updated time")
	}
	value.Variables = value.Variables.Clone()
	if err := value.Validate(); err != nil {
		return Environment{}, err
	}
	return value, nil
}

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

func (e Environment) Delete(at int64) (Environment, error) { return setEnvironmentDeleted(e, true, at) }
func (e Environment) Restore(at int64) (Environment, error) {
	return setEnvironmentDeleted(e, false, at)
}
func setEnvironmentDeleted(e Environment, deleted bool, at int64) (Environment, error) {
	if (e.DeletedAt != 0) == deleted {
		return Environment{}, errors.New("environment lifecycle transition is a no-op")
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

func NewElementTarget(node ElementTarget, initial ElementTargetVersion) (ElementTargetAggregate, error) {
	node.Revision = 1
	node.CurrentVersionID = initial.ID
	initial.ElementTargetID = node.ID
	initial.VersionNumber = 1
	if node.CreatedAt <= 0 || node.UpdatedAt != node.CreatedAt || initial.CreatedAt != node.CreatedAt {
		return ElementTargetAggregate{}, errors.New("node creation timestamps must be positive and equal")
	}
	a := ElementTargetAggregate{ElementTarget: node, Current: initial, Versions: []ElementTargetVersion{initial}}
	a = cloneNodeAggregate(a)
	if err := a.ValidateLoadedHistory(); err != nil {
		return ElementTargetAggregate{}, err
	}
	return a, nil
}

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
func (a ElementTargetAggregate) Delete(at int64) (ElementTargetAggregate, error) {
	return a.setDeleted(true, at)
}
func (a ElementTargetAggregate) Restore(at int64) (ElementTargetAggregate, error) {
	return a.setDeleted(false, at)
}
func (a ElementTargetAggregate) setDeleted(deleted bool, at int64) (ElementTargetAggregate, error) {
	if (a.ElementTarget.DeletedAt != 0) == deleted {
		return ElementTargetAggregate{}, errors.New("node lifecycle transition is a no-op")
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

func NewFlowFragment(workflow FlowFragment, initial FlowFragmentVersion) (FlowFragmentAggregate, error) {
	workflow.Revision = 1
	workflow.CurrentVersionID = initial.ID
	initial.FlowFragmentID = workflow.ID
	initial.VersionNumber = 1
	if workflow.CreatedAt <= 0 || workflow.UpdatedAt != workflow.CreatedAt || initial.CreatedAt != workflow.CreatedAt {
		return FlowFragmentAggregate{}, errors.New("workflow creation timestamps must be positive and equal")
	}
	a := cloneWorkflowAggregate(FlowFragmentAggregate{FlowFragment: workflow, Current: initial, Versions: []FlowFragmentVersion{initial}})
	if err := a.ValidateLoadedHistory(); err != nil {
		return FlowFragmentAggregate{}, err
	}
	return a, nil
}
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
func (a FlowFragmentAggregate) Delete(at int64) (FlowFragmentAggregate, error) {
	return a.setDeleted(true, at)
}
func (a FlowFragmentAggregate) Restore(at int64) (FlowFragmentAggregate, error) {
	return a.setDeleted(false, at)
}
func (a FlowFragmentAggregate) setDeleted(deleted bool, at int64) (FlowFragmentAggregate, error) {
	if (a.FlowFragment.DeletedAt != 0) == deleted {
		return FlowFragmentAggregate{}, errors.New("workflow lifecycle transition is a no-op")
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

func (a ExecutionFlowAggregate) PublishVersion(publication ExecutionFlowVersionPublication) (ExecutionFlowAggregate, error) {
	if err := a.Validate(); err != nil {
		return ExecutionFlowAggregate{}, err
	}
	if a.Task.DeletedAt != 0 {
		return ExecutionFlowAggregate{}, DeletedAggregateError()
	}
	if strings.TrimSpace(publication.ID) == "" || publication.CreatedAt <= 0 || publication.CreatedAt < a.Task.UpdatedAt {
		return ExecutionFlowAggregate{}, errors.New("test task publication requires a new version identity and monotonic timestamp")
	}
	for _, existing := range a.Versions {
		if existing.ID == publication.ID {
			return ExecutionFlowAggregate{}, errors.New("test task version id already exists")
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

func NewExecutionFlow(task ExecutionFlow, initial ExecutionFlowVersion) (ExecutionFlowAggregate, error) {
	task.Revision = 1
	task.CurrentVersionID = initial.ID
	initial.ExecutionFlowID = task.ID
	initial.VersionNumber = 1
	initial.SourceVersionID = ""
	if task.CreatedAt <= 0 || task.UpdatedAt != task.CreatedAt || initial.CreatedAt != task.CreatedAt {
		return ExecutionFlowAggregate{}, errors.New("test task creation timestamps must be positive and equal")
	}
	aggregate := cloneTestTaskAggregate(ExecutionFlowAggregate{Task: task, Current: initial, Versions: []ExecutionFlowVersion{initial}})
	if err := aggregate.Validate(); err != nil {
		return ExecutionFlowAggregate{}, err
	}
	return aggregate, nil
}

func cloneTestTaskAggregate(aggregate ExecutionFlowAggregate) ExecutionFlowAggregate {
	cloned := aggregate
	cloned.Current = cloneTestTaskVersion(aggregate.Current)
	cloned.Versions = make([]ExecutionFlowVersion, len(aggregate.Versions))
	for index, version := range aggregate.Versions {
		cloned.Versions[index] = cloneTestTaskVersion(version)
	}
	return cloned
}

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
