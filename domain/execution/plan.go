package execution

import (
	"sort"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// planSeal 是仅包内可创建的计划封印标记。
type planSeal struct {
	marker byte
}

var sealedPlanToken = &planSeal{marker: 1}

// StepKind 标识执行步骤的受约束种类。
type StepKind string

const (
	// ActionStep 表示执行宿主动作的步骤。
	ActionStep StepKind = "ACTION"
	// WaitStep 表示等待指定时间或条件的步骤。
	WaitStep StepKind = "WAIT"
	// RepeatStep 表示按次数重复子步骤的步骤。
	RepeatStep StepKind = "REPEAT"
	// FlowFragmentReference 表示调用另一个工作流版本的引用步骤。
	FlowFragmentReference StepKind = "WORKFLOW_REF"
	// ValidationStep 表示执行单项终态校验的步骤。
	ValidationStep StepKind = "VALIDATION"
	// ValidationGroupStep 表示执行分支校验组的步骤。
	ValidationGroupStep StepKind = "VALIDATION_GROUP"
)

// Step 描述计划中的一个动作、等待、重复、引用或校验节点。
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

// Reference 描述工作流引用的目标版本和参数绑定。
type Reference struct {
	FlowFragmentID    string
	WorkflowVersionID string
	ParameterBindings map[string]parameter.Binding
}

// Validation 描述单项断言及其最大等待和稳定窗口。
type Validation struct {
	Kind           string
	Expected       string
	ExpectedValues []string
	Attribute      string
	IgnoreCase     bool
	MaxWaitMS      int
	StabilityMS    int
}

// ValidationGroup 描述并行校验分支及其共享等待窗口。
type ValidationGroup struct {
	Branches    []ValidationBranch
	MaxWaitMS   int
	StabilityMS int
}

// ValidationBranch 描述校验组中的一个具名步骤分支。
type ValidationBranch struct {
	ID    string
	Name  string
	Steps []Step
}

// Parameter 保存工作流参数定义的执行快照。
type Parameter struct {
	Name        string
	DisplayName string
	Description string
	Type        parameter.Type
	Required    bool
	Default     parameter.OptionalValue
	Options     []string
}

// ParameterSnapshot 保存入口绑定到工作流版本的已解析参数值。
type ParameterSnapshot struct {
	ID                string
	SchemaVersion     int
	WorkflowVersionID string
	Values            map[string]parameter.Value
}

// WorkflowSnapshot 保存工作流版本、参数和步骤树的不可变计划数据。
type WorkflowSnapshot struct {
	ID             string
	VersionID      string
	FlowFragmentID string
	DisplayName    string
	VersionNumber  int
	Parameters     []Parameter
	Steps          []Step
}

// NodeSnapshot 保存元素目标版本、定位器和指纹的计划数据。
type NodeSnapshot struct {
	ElementTargetID string
	VersionID       string
	DisplayName     string
	PageURL         string
	Origin          string
	Selectors       []fingerprint.Selector
	Fingerprint     fingerprint.Fingerprint
}

// NodeDependencyKey 以元素目标和版本 ID 标识节点依赖。
type NodeDependencyKey struct {
	ElementTargetID string
	VersionID       string
}

// WorkflowReferenceKey 以父工作流版本和步骤 ID 标识引用边。
type WorkflowReferenceKey struct {
	ParentVersionID string
	StepID          string
}

// ReferenceResolution 保存工作流引用边的具体目标版本及其解析来源。
type ReferenceResolution struct {
	ParentVersionID    string
	StepID             string
	FlowFragmentID     string
	WorkflowVersionID  string
	ResolvedFromLatest bool
}

// FailurePolicy 决定入口失败后是否继续执行后续入口。
type FailurePolicy string

const (
	// FailurePolicyStopOnFailure 表示首个失败后停止后续入口。
	FailurePolicyStopOnFailure FailurePolicy = "STOP_ON_FAILURE"
	// FailurePolicyContinueOnFailure 表示入口失败后继续执行后续入口。
	FailurePolicyContinueOnFailure FailurePolicy = "CONTINUE_ON_FAILURE"
)

// IsValid 判断失败策略是否属于支持集合。
func (p FailurePolicy) IsValid() bool {
	return p == FailurePolicyStopOnFailure || p == FailurePolicyContinueOnFailure
}

// Entry 描述计划中按序执行的测试任务入口及其参数快照。
type Entry struct {
	ID                EntryID
	TestTaskItemID    string
	SequenceNumber    int
	FlowFragmentID    string
	WorkflowVersionID string
	Parameters        ParameterSnapshot
}

// PlanSnapshot 是尚未封印或从封印计划复制出的完整计划数据。
type PlanSnapshot struct {
	InstanceID    InstanceID
	FailurePolicy FailurePolicy
	Entries       []Entry
	Workflows     []WorkflowSnapshot
	Nodes         []NodeSnapshot
	References    []ReferenceResolution
}

// Plan 保存经校验、规范排序且带包内封印的执行计划。
type Plan struct {
	draft PlanSnapshot
	seal  *planSeal
}

// Seal 校验计划草稿、复制所有引用数据并按入口序号和 ID 规范排序后返回封印计划。
func Seal(draft PlanSnapshot) (Plan, error) {
	if err := draft.Validate(); err != nil {
		return Plan{}, err
	}
	canonical := cloneDraft(draft)
	// 入口 ID 作为序号相同时的确定性次序，使规范排序本身保持全序，不依赖其他校验步骤。
	sort.Slice(canonical.Entries, func(i, j int) bool {
		if canonical.Entries[i].SequenceNumber != canonical.Entries[j].SequenceNumber {
			return canonical.Entries[i].SequenceNumber < canonical.Entries[j].SequenceNumber
		}
		return canonical.Entries[i].ID.String() < canonical.Entries[j].ID.String()
	})
	return Plan{draft: canonical, seal: sealedPlanToken}, nil
}

// IsSealed 判断计划是否带有 Seal 创建的包内封印。
func (p Plan) IsSealed() bool { return p.seal == sealedPlanToken }

// Validate 校验计划是否满足封印不变量。
func (p Plan) Validate() error {
	if !p.IsSealed() {
		return mustExecutionFault(fault.FailedPrecondition, CodePlanUnsealed, "execution plan must be sealed")
	}
	return nil
}

// Snapshot 返回计划草稿的深拷贝，调用方可安全修改结果。
func (p Plan) Snapshot() PlanSnapshot { return cloneDraft(p.draft) }

// InstanceID 返回计划所属实例 ID。
func (p Plan) InstanceID() InstanceID { return p.draft.InstanceID }

// FailurePolicy 返回计划的入口失败策略。
func (p Plan) FailurePolicy() FailurePolicy { return p.draft.FailurePolicy }

// Entries 返回入口及其参数快照的深拷贝切片。
func (p Plan) Entries() []Entry {
	entries := append([]Entry(nil), p.draft.Entries...)
	for i := range entries {
		entries[i].Parameters = cloneParameterSnapshot(entries[i].Parameters)
	}
	return entries
}

// Workflows 返回工作流快照的深拷贝切片。
func (p Plan) Workflows() []WorkflowSnapshot { return cloneWorkflows(p.draft.Workflows) }

// Nodes 返回节点快照的深拷贝切片。
func (p Plan) Nodes() []NodeSnapshot { return cloneNodes(p.draft.Nodes) }

// References 返回引用解析快照切片的副本。
func (p Plan) References() []ReferenceResolution {
	return append([]ReferenceResolution(nil), p.draft.References...)
}

// cloneDraft 深复制计划快照中的入口、工作流、节点和引用切片，隔离调用方所有权。
func cloneDraft(draft PlanSnapshot) PlanSnapshot {
	entries := append([]Entry(nil), draft.Entries...)
	for i := range entries {
		entries[i].Parameters = cloneParameterSnapshot(entries[i].Parameters)
	}
	return PlanSnapshot{
		InstanceID: draft.InstanceID, FailurePolicy: draft.FailurePolicy,
		Entries:   entries,
		Workflows: cloneWorkflows(draft.Workflows), Nodes: cloneNodes(draft.Nodes),
		References: append([]ReferenceResolution(nil), draft.References...),
	}
}

// cloneWorkflows 深复制工作流参数、步骤树及其嵌套引用和校验结构。
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

// cloneSteps 递归深复制步骤切片、子步骤、引用、断言和校验分支。
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

// cloneNodes 深复制节点选择器切片和指纹数据，隔离源计划的所有权。
func cloneNodes(nodes []NodeSnapshot) []NodeSnapshot {
	result := make([]NodeSnapshot, len(nodes))
	for i, snapshot := range nodes {
		result[i] = snapshot
		result[i].Selectors = append([]fingerprint.Selector(nil), snapshot.Selectors...)
		result[i].Fingerprint = snapshot.Fingerprint.Clone()
	}
	return result
}

// cloneBindings 深复制参数绑定映射；nil 输入保持 nil，结果映射由调用方独占。
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

// cloneParameterValues 深复制参数值映射；nil 输入保持 nil，避免快照共享可变值。
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

// cloneParameterSnapshot 复制参数快照并深复制其值映射。
func cloneParameterSnapshot(snapshot ParameterSnapshot) ParameterSnapshot {
	snapshot.Values = cloneParameterValues(snapshot.Values)
	return snapshot
}
