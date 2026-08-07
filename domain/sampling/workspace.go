package sampling

import (
	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type ResolutionMode string

const (
	ResolutionModeUndecided ResolutionMode = "UNDECIDED"
	ResolutionModeCreate    ResolutionMode = "CREATE"
	ResolutionModeMerge     ResolutionMode = "MERGE"
	ResolutionModeReuse     ResolutionMode = "REUSE"
)

type ElementTargetCandidate struct {
	ElementTargetID string
	DisplayName     string
	VersionID       string
	VersionNumber   int
	Similarity      float64
	SelectorOverlap int
	Exact           bool
}

type UnpublishedElementTarget struct {
	ID             string
	DisplayName    string
	Properties     automation.Properties
	PageURL        string
	Origin         string
	Selectors      []fingerprint.Selector
	Fingerprint    fingerprint.Fingerprint
	StepIDs        []string
	ResolutionMode ResolutionMode
	Candidates     []ElementTargetCandidate
}

// UnpublishedFlowFragment 是内存中的草稿。它不复述会话生命周期：那是 Session.Status 的唯一归属。它也不携带发布状态——发布是一次原子幂等事务，"进行中"在领域侧没有观察点，而其结果是 Publish 的返回值或一个 typed fault，不是草稿上的字段。
type UnpublishedFlowFragment struct {
	ID          string
	SessionID   string
	DisplayName string
	Properties  automation.Properties
	StartedAt   int64
	// 验证插入是一个可选的暂停编辑器选择。  它只影响下一次验证捕获；普通操作始终保留根步骤，并且应用程序层清除无效/已删除的分支。
	ValidationInsertGroupID     string
	ValidationInsertBranchID    string
	ValidationCapturedActionIDs []string
	Steps                       []automation.FlowFragmentStep
	Parameters                  []automation.ParameterDefinition
	Nodes                       []UnpublishedElementTarget
}

// RebuildUnpublishedElementTargetReferences 从可编辑工作流树中派生临时 ElementTarget -> Step 投影。临时采样数据有意仅存储在内存中，因此这是任何捕获、编辑、删除或重新排序操作后的唯一事实来源。
func RebuildUnpublishedElementTargetReferences(workflow *UnpublishedFlowFragment) error {
	if workflow == nil {
		// A nil receiver is a caller code defect with no runtime remediation.
		return internalError()
	}
	stepIDsByNode := make(map[string][]string, len(workflow.Nodes))
	for _, node := range workflow.Nodes {
		stepIDsByNode[node.ID] = nil
	}
	// The walk collects every undefined reference before returning. Stopping at the
	// first meant a draft with several took one rebuild attempt per reference. Walk
	// order is the tree's own depth-first order, so the report is a function of the
	// input, and the cap is the only reason to stop early.
	var violations []fault.Violation
	var walk func([]automation.FlowFragmentStep) error
	walk = func(steps []automation.FlowFragmentStep) error {
		for _, step := range steps {
			if step.ElementTargetID != "" {
				stepIDs, ok := stepIDsByNode[step.ElementTargetID]
				if !ok {
					// Both the step id and the temporary element target id are caller
					// identities; neither may appear in the public violation.
					if len(violations) < fault.MaxViolations {
						violations = append(violations, mustViolation(fault.CodeFieldMismatch, "steps.elementTargetId", "a step references a temporary element target that the draft does not define"))
					}
				} else {
					stepIDsByNode[step.ElementTargetID] = append(stepIDs, step.ID)
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
	if err := walk(workflow.Steps); err != nil {
		return err
	}
	// The projection is only written once every reference resolved, so a rejected
	// rebuild never leaves a partially derived projection behind.
	if len(violations) != 0 {
		return workspaceInvalidError(violations)
	}
	for index := range workflow.Nodes {
		workflow.Nodes[index].StepIDs = stepIDsByNode[workflow.Nodes[index].ID]
	}
	return nil
}
