package sampling

import (
	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// ResolutionMode 表示未发布元素目标与候选正式目标的解析方式。
type ResolutionMode string

const (
	// ResolutionModeUndecided 表示尚未选择解析方式。
	ResolutionModeUndecided ResolutionMode = "UNDECIDED"
	// ResolutionModeCreate 表示创建新的正式元素目标。
	ResolutionModeCreate ResolutionMode = "CREATE"
	// ResolutionModeMerge 表示合并到候选正式元素目标。
	ResolutionModeMerge ResolutionMode = "MERGE"
	// ResolutionModeReuse 表示复用候选正式元素目标。
	ResolutionModeReuse ResolutionMode = "REUSE"
)

// ElementTargetCandidate 描述一个可供未发布元素目标解析的正式候选。
type ElementTargetCandidate struct {
	ElementTargetID string
	DisplayName     string
	VersionID       string
	VersionNumber   int
	Similarity      float64
	SelectorOverlap int
	Exact           bool
}

// UnpublishedElementTarget 保存采样期间的临时元素目标及其解析候选。
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

// UnpublishedFlowFragment 保存未发布流程片段的草稿步骤、参数和临时元素目标。
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

// RebuildUnpublishedElementTargetReferences 递归重建临时元素目标到步骤 ID 的引用投影。
func RebuildUnpublishedElementTargetReferences(workflow *UnpublishedFlowFragment) error {
	if workflow == nil {
		return internalError()
	}
	stepIDsByNode := make(map[string][]string, len(workflow.Nodes))
	for _, node := range workflow.Nodes {
		stepIDsByNode[node.ID] = nil
	}
	var violations []fault.Violation
	var walk func([]automation.FlowFragmentStep) error
	walk = func(steps []automation.FlowFragmentStep) error {
		for _, step := range steps {
			if step.ElementTargetID != "" {
				stepIDs, ok := stepIDsByNode[step.ElementTargetID]
				if !ok {
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
	if len(violations) != 0 {
		return workspaceInvalidError(violations)
	}
	for index := range workflow.Nodes {
		workflow.Nodes[index].StepIDs = stepIDsByNode[workflow.Nodes[index].ID]
	}
	return nil
}
