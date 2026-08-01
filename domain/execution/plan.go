package execution

import (
	"sort"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

type planSeal struct {
	marker byte
}

var sealedPlanToken = &planSeal{marker: 1}

type StepKind string

const (
	ActionStep            StepKind = "ACTION"
	WaitStep              StepKind = "WAIT"
	RepeatStep            StepKind = "REPEAT"
	FlowFragmentReference StepKind = "WORKFLOW_REF"
	ValidationStep        StepKind = "VALIDATION"
	ValidationGroupStep   StepKind = "VALIDATION_GROUP"
)

type Step struct {
	ID                     string
	DisplayName            string
	Kind                   StepKind
	CaptureScreenshot      bool
	Action                 string
	ElementTargetID        string
	ElementTargetVersionID string
	Value                  string
	Values                 []string
	WaitKind               string
	WaitMS                 int
	RepeatCount            int
	Optional               bool
	Children               []Step
	Reference              *Reference
	Validation             *Validation
	ValidationGroup        *ValidationGroup
}

type Reference struct {
	FlowFragmentID    string
	WorkflowVersionID string
	ParameterBindings map[string]parameter.Binding
}

type Validation struct {
	Kind           string
	Expected       string
	ExpectedValues []string
	Attribute      string
	IgnoreCase     bool
	MaxWaitMS      int
	StabilityMS    int
}

type ValidationGroup struct {
	Branches    []ValidationBranch
	MaxWaitMS   int
	StabilityMS int
}

type ValidationBranch struct {
	ID    string
	Name  string
	Steps []Step
}

type Parameter struct {
	Name        string
	DisplayName string
	Description string
	Type        parameter.Type
	Required    bool
	Default     parameter.OptionalValue
	Options     []string
}

type ParameterSnapshot struct {
	ID                string
	SchemaVersion     int
	WorkflowVersionID string
	Values            map[string]parameter.Value
}

type WorkflowSnapshot struct {
	ID             string
	VersionID      string
	FlowFragmentID string
	DisplayName    string
	VersionNumber  int
	Parameters     []Parameter
	Steps          []Step
}

type NodeSnapshot struct {
	ElementTargetID string
	VersionID       string
	DisplayName     string
	PageURL         string
	Origin          string
	Selectors       []fingerprint.Selector
	Fingerprint     fingerprint.Fingerprint
}

type NodeDependencyKey struct {
	ElementTargetID string
	VersionID       string
}

type WorkflowReferenceKey struct {
	ParentVersionID string
	StepID          string
}

type ReferenceResolution struct {
	ParentVersionID    string
	StepID             string
	FlowFragmentID     string
	WorkflowVersionID  string
	ResolvedFromLatest bool
}

type FailurePolicy string

const (
	FailurePolicyStopOnFailure     FailurePolicy = "STOP_ON_FAILURE"
	FailurePolicyContinueOnFailure FailurePolicy = "CONTINUE_ON_FAILURE"
)

func (p FailurePolicy) IsValid() bool {
	return p == FailurePolicyStopOnFailure || p == FailurePolicyContinueOnFailure
}

type Entry struct {
	ID                EntryID
	TestTaskItemID    string
	SequenceNumber    int
	FlowFragmentID    string
	WorkflowVersionID string
	Parameters        ParameterSnapshot
}

type PlanSnapshot struct {
	RunID         string
	FailurePolicy FailurePolicy
	Entries       []Entry
	Workflows     []WorkflowSnapshot
	Nodes         []NodeSnapshot
	References    []ReferenceResolution
}

type Plan struct {
	draft PlanSnapshot
	seal  *planSeal
}

func Seal(draft PlanSnapshot) (Plan, error) {
	if err := draft.Validate(); err != nil {
		return Plan{}, err
	}
	canonical := cloneDraft(draft)
	sort.Slice(canonical.Entries, func(i, j int) bool {
		return canonical.Entries[i].SequenceNumber < canonical.Entries[j].SequenceNumber
	})
	return Plan{draft: canonical, seal: sealedPlanToken}, nil
}

// IsSealed reports whether the plan was successfully created by Seal.
func (p Plan) IsSealed() bool { return p.seal == sealedPlanToken }

// Validate checks that the plan carries the Seal invariant.
func (p Plan) Validate() error {
	if !p.IsSealed() {
		return mustExecutionFault(fault.FailedPrecondition, CodePlanUnsealed, "execution plan must be sealed")
	}
	return nil
}

func (p Plan) Snapshot() PlanSnapshot { return cloneDraft(p.draft) }

func (p Plan) RunID() string { return p.draft.RunID }

func (p Plan) FailurePolicy() FailurePolicy { return p.draft.FailurePolicy }

func (p Plan) Entries() []Entry {
	entries := append([]Entry(nil), p.draft.Entries...)
	for i := range entries {
		entries[i].Parameters = cloneParameterSnapshot(entries[i].Parameters)
	}
	return entries
}

func (p Plan) Workflows() []WorkflowSnapshot { return cloneWorkflows(p.draft.Workflows) }

func (p Plan) Nodes() []NodeSnapshot { return cloneNodes(p.draft.Nodes) }

func (p Plan) References() []ReferenceResolution {
	return append([]ReferenceResolution(nil), p.draft.References...)
}

func cloneDraft(draft PlanSnapshot) PlanSnapshot {
	entries := append([]Entry(nil), draft.Entries...)
	for i := range entries {
		entries[i].Parameters = cloneParameterSnapshot(entries[i].Parameters)
	}
	return PlanSnapshot{
		RunID: draft.RunID, FailurePolicy: draft.FailurePolicy,
		Entries:   entries,
		Workflows: cloneWorkflows(draft.Workflows), Nodes: cloneNodes(draft.Nodes),
		References: append([]ReferenceResolution(nil), draft.References...),
	}
}

func cloneWorkflows(workflows []WorkflowSnapshot) []WorkflowSnapshot {
	result := make([]WorkflowSnapshot, len(workflows))
	for i, workflow := range workflows {
		result[i] = workflow
		result[i].Parameters = append([]Parameter(nil), workflow.Parameters...)
		for j := range result[i].Parameters {
			result[i].Parameters[j].Options = append([]string(nil), workflow.Parameters[j].Options...)
			if value, present := workflow.Parameters[j].Default.Value(); present {
				result[i].Parameters[j].Default = parameter.PresentValue(value)
			}
		}
		result[i].Steps = cloneSteps(workflow.Steps)
	}
	return result
}

func cloneSteps(steps []Step) []Step {
	result := make([]Step, len(steps))
	for i, step := range steps {
		result[i] = step
		result[i].Values = append([]string(nil), step.Values...)
		result[i].Children = cloneSteps(step.Children)
		if step.Reference != nil {
			copy := *step.Reference
			copy.ParameterBindings = cloneBindings(step.Reference.ParameterBindings)
			result[i].Reference = &copy
		}
		if step.Validation != nil {
			copy := *step.Validation
			copy.ExpectedValues = append([]string(nil), step.Validation.ExpectedValues...)
			result[i].Validation = &copy
		}
		if step.ValidationGroup != nil {
			copy := *step.ValidationGroup
			copy.Branches = make([]ValidationBranch, len(step.ValidationGroup.Branches))
			for j, branch := range step.ValidationGroup.Branches {
				copy.Branches[j] = branch
				copy.Branches[j].Steps = cloneSteps(branch.Steps)
			}
			result[i].ValidationGroup = &copy
		}
	}
	return result
}

func cloneNodes(nodes []NodeSnapshot) []NodeSnapshot {
	result := make([]NodeSnapshot, len(nodes))
	for i, snapshot := range nodes {
		result[i] = snapshot
		result[i].Selectors = append([]fingerprint.Selector(nil), snapshot.Selectors...)
		result[i].Fingerprint = cloneFingerprint(snapshot.Fingerprint)
	}
	return result
}

func cloneFingerprint(value fingerprint.Fingerprint) fingerprint.Fingerprint {
	value.Attributes = cloneMap(value.Attributes)
	value.Path = append([]string(nil), value.Path...)
	value.Framework = value.Framework.Clone()
	return value
}

func cloneBindings(source map[string]parameter.Binding) map[string]parameter.Binding {
	if source == nil {
		return nil
	}
	result := make(map[string]parameter.Binding, len(source))
	for name, binding := range source {
		result[name] = binding.Clone()
	}
	return result
}
func cloneParameterValues(source map[string]parameter.Value) map[string]parameter.Value {
	if source == nil {
		return nil
	}
	result := make(map[string]parameter.Value, len(source))
	for key, value := range source {
		result[key] = value.Clone()
	}
	return result
}

func cloneParameterSnapshot(snapshot ParameterSnapshot) ParameterSnapshot {
	snapshot.Values = cloneParameterValues(snapshot.Values)
	return snapshot
}

func cloneMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
