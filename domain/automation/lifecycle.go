package automation

import (
	"errors"
	"fmt"
	"github.com/Capsule7446/healix-core/domain/parameter"
	"strings"
)

var ErrDeletedAggregate = errors.New("deleted aggregate cannot be mutated")

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
		return Environment{}, ErrDeletedAggregate
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
	return next, nil
}

func NewNode(node Node, initial NodeVersion) (NodeAggregate, error) {
	node.Revision = 1
	node.CurrentVersionID = initial.ID
	initial.NodeID = node.ID
	initial.VersionNumber = 1
	if node.CreatedAt <= 0 || node.UpdatedAt != node.CreatedAt || initial.CreatedAt != node.CreatedAt {
		return NodeAggregate{}, errors.New("node creation timestamps must be positive and equal")
	}
	a := NodeAggregate{Node: node, Current: initial, Versions: []NodeVersion{initial}}
	a = cloneNodeAggregate(a)
	if err := a.ValidateLoadedHistory(); err != nil {
		return NodeAggregate{}, err
	}
	return a, nil
}

func (a NodeAggregate) UpdateMetadata(displayName, folderID string, properties Properties, at int64) (NodeAggregate, error) {
	if a.Node.DeletedAt != 0 {
		return NodeAggregate{}, ErrDeletedAggregate
	}
	if err := validateTransitionTime(at, a.Node.UpdatedAt); err != nil {
		return NodeAggregate{}, err
	}
	r, err := a.Node.Revision.Next()
	if err != nil {
		return NodeAggregate{}, err
	}
	n := cloneNodeAggregate(a)
	n.Node.DisplayName = displayName
	n.Node.FolderID = folderID
	n.Node.Properties = properties.Clone()
	n.Node.UpdatedAt = at
	n.Node.Revision = r
	if err := n.ValidateLoadedHistory(); err != nil {
		return NodeAggregate{}, err
	}
	return n, nil
}
func (a NodeAggregate) Delete(at int64) (NodeAggregate, error)  { return a.setDeleted(true, at) }
func (a NodeAggregate) Restore(at int64) (NodeAggregate, error) { return a.setDeleted(false, at) }
func (a NodeAggregate) setDeleted(deleted bool, at int64) (NodeAggregate, error) {
	if (a.Node.DeletedAt != 0) == deleted {
		return NodeAggregate{}, errors.New("node lifecycle transition is a no-op")
	}
	if err := validateTransitionTime(at, a.Node.UpdatedAt); err != nil {
		return NodeAggregate{}, err
	}
	r, err := a.Node.Revision.Next()
	if err != nil {
		return NodeAggregate{}, err
	}
	n := cloneNodeAggregate(a)
	n.Node.DeletedAt = 0
	if deleted {
		n.Node.DeletedAt = at
	}
	n.Node.UpdatedAt = at
	n.Node.Revision = r
	return n, nil
}

func NewWorkflow(workflow Workflow, initial WorkflowVersion) (WorkflowAggregate, error) {
	workflow.Revision = 1
	workflow.CurrentVersionID = initial.ID
	initial.WorkflowID = workflow.ID
	initial.VersionNumber = 1
	if workflow.CreatedAt <= 0 || workflow.UpdatedAt != workflow.CreatedAt || initial.CreatedAt != workflow.CreatedAt {
		return WorkflowAggregate{}, errors.New("workflow creation timestamps must be positive and equal")
	}
	a := cloneWorkflowAggregate(WorkflowAggregate{Workflow: workflow, Current: initial, Versions: []WorkflowVersion{initial}})
	if err := a.ValidateLoadedHistory(); err != nil {
		return WorkflowAggregate{}, err
	}
	return a, nil
}
func (a WorkflowAggregate) UpdateMetadata(displayName, folderID string, properties Properties, at int64) (WorkflowAggregate, error) {
	if a.Workflow.DeletedAt != 0 {
		return WorkflowAggregate{}, ErrDeletedAggregate
	}
	if err := validateTransitionTime(at, a.Workflow.UpdatedAt); err != nil {
		return WorkflowAggregate{}, err
	}
	r, err := a.Workflow.Revision.Next()
	if err != nil {
		return WorkflowAggregate{}, err
	}
	n := cloneWorkflowAggregate(a)
	n.Workflow.DisplayName = displayName
	n.Workflow.FolderID = folderID
	n.Workflow.Properties = properties.Clone()
	n.Workflow.UpdatedAt = at
	n.Workflow.Revision = r
	if err := n.ValidateLoadedHistory(); err != nil {
		return WorkflowAggregate{}, err
	}
	return n, nil
}
func (a WorkflowAggregate) Delete(at int64) (WorkflowAggregate, error) { return a.setDeleted(true, at) }
func (a WorkflowAggregate) Restore(at int64) (WorkflowAggregate, error) {
	return a.setDeleted(false, at)
}
func (a WorkflowAggregate) setDeleted(deleted bool, at int64) (WorkflowAggregate, error) {
	if (a.Workflow.DeletedAt != 0) == deleted {
		return WorkflowAggregate{}, errors.New("workflow lifecycle transition is a no-op")
	}
	if err := validateTransitionTime(at, a.Workflow.UpdatedAt); err != nil {
		return WorkflowAggregate{}, err
	}
	r, err := a.Workflow.Revision.Next()
	if err != nil {
		return WorkflowAggregate{}, err
	}
	n := cloneWorkflowAggregate(a)
	n.Workflow.DeletedAt = 0
	if deleted {
		n.Workflow.DeletedAt = at
	}
	n.Workflow.UpdatedAt = at
	n.Workflow.Revision = r
	return n, nil
}

func (a TestTaskAggregate) PublishVersion(publication TestTaskVersionPublication) (TestTaskAggregate, error) {
	if err := a.Validate(); err != nil {
		return TestTaskAggregate{}, err
	}
	if a.Task.DeletedAt != 0 {
		return TestTaskAggregate{}, ErrDeletedAggregate
	}
	if strings.TrimSpace(publication.ID) == "" || publication.CreatedAt <= 0 || publication.CreatedAt < a.Task.UpdatedAt {
		return TestTaskAggregate{}, errors.New("test task publication requires a new version identity and monotonic timestamp")
	}
	for _, existing := range a.Versions {
		if existing.ID == publication.ID {
			return TestTaskAggregate{}, errors.New("test task version id already exists")
		}
	}
	next := cloneTestTaskAggregate(a)
	version := cloneTestTaskVersion(TestTaskVersion{
		ID:                      publication.ID,
		TestTaskID:              a.Task.ID,
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
		return TestTaskAggregate{}, err
	}
	next.Task.CurrentVersionID = version.ID
	next.Task.UpdatedAt = version.CreatedAt
	next.Task.Revision = nextRevision
	next.Current = cloneTestTaskVersion(version)
	next.Versions = append(next.Versions, cloneTestTaskVersion(version))
	if err := next.Validate(); err != nil {
		return TestTaskAggregate{}, err
	}
	return next, nil
}

func NewTestTask(task TestTask, initial TestTaskVersion) (TestTaskAggregate, error) {
	task.Revision = 1
	task.CurrentVersionID = initial.ID
	initial.TestTaskID = task.ID
	initial.VersionNumber = 1
	initial.SourceVersionID = ""
	if task.CreatedAt <= 0 || task.UpdatedAt != task.CreatedAt || initial.CreatedAt != task.CreatedAt {
		return TestTaskAggregate{}, errors.New("test task creation timestamps must be positive and equal")
	}
	aggregate := cloneTestTaskAggregate(TestTaskAggregate{Task: task, Current: initial, Versions: []TestTaskVersion{initial}})
	if err := aggregate.Validate(); err != nil {
		return TestTaskAggregate{}, err
	}
	return aggregate, nil
}

func revisionError(kind, id string, err error) error {
	return fmt.Errorf("%s %s revision: %w", kind, id, err)
}

func cloneTestTaskAggregate(aggregate TestTaskAggregate) TestTaskAggregate {
	cloned := aggregate
	cloned.Current = cloneTestTaskVersion(aggregate.Current)
	cloned.Versions = make([]TestTaskVersion, len(aggregate.Versions))
	for index, version := range aggregate.Versions {
		cloned.Versions[index] = cloneTestTaskVersion(version)
	}
	return cloned
}

func cloneTestTaskVersion(version TestTaskVersion) TestTaskVersion {
	cloned := version
	cloned.RequiredEnvironmentKeys = append([]string(nil), version.RequiredEnvironmentKeys...)
	cloned.Items = make([]TestTaskItem, len(version.Items))
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
