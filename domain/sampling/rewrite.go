package sampling

import (
	"fmt"
	"strings"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// RewriteUnpublishedElementTargetReferences 校验映射集合并递归重写步骤引用，失败时不返回部分结果；目标身份不写入公开错误文本。
func RewriteUnpublishedElementTargetReferences(steps []automation.FlowFragmentStep, mappings []automation.SamplingNodeMapping) ([]automation.FlowFragmentStep, error) {
	mappingByTemporaryID := make(map[string]automation.SamplingNodeMapping, len(mappings))
	var violations []fault.Violation
	for index, mapping := range mappings {
		field := func(name string) string { return fmt.Sprintf("mappings.%d.%s", index, name) }
		if strings.TrimSpace(mapping.TemporaryElementTargetID) == "" {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, field("temporaryElementTargetId"), "temporary element target id is required"))
		} else if _, exists := mappingByTemporaryID[mapping.TemporaryElementTargetID]; exists {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field("temporaryElementTargetId"), "temporary element target id is mapped more than once"))
		}
		if strings.TrimSpace(mapping.ElementTargetID) == "" {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, field("elementTargetId"), "formal element target id is required"))
		}
		if strings.TrimSpace(mapping.ElementTargetVersionID) == "" {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, field("elementTargetVersionId"), "formal element target version id is required"))
		}
		mappingByTemporaryID[mapping.TemporaryElementTargetID] = mapping
	}
	violations = appendUnmappedReferenceViolations(violations, steps, mappingByTemporaryID)
	if len(violations) != 0 {
		return nil, publicationMappingInvalidError(violations)
	}
	used := make(map[string]struct{}, len(mappingByTemporaryID))
	rewritten := rewriteSamplingSteps(steps, mappingByTemporaryID, used)
	if len(used) != len(mappingByTemporaryID) {
		return nil, publicationMappingInvalidError([]fault.Violation{
			mustViolation(fault.CodeFieldMismatch, "mappings", "mappings must exactly match the referenced temporary element targets"),
		})
	}
	return rewritten, nil
}

// appendUnmappedReferenceViolations 深度优先遍历步骤树，聚合未映射临时目标引用且不暴露调用方身份。
func appendUnmappedReferenceViolations(violations []fault.Violation, steps []automation.FlowFragmentStep, mappings map[string]automation.SamplingNodeMapping) []fault.Violation {
	for _, step := range steps {
		if len(violations) >= fault.MaxViolations {
			return violations
		}
		if step.ElementTargetID != "" {
			if _, exists := mappings[step.ElementTargetID]; !exists {
				violations = append(violations, mustViolation(fault.CodeFieldMismatch, "steps.elementTargetId", "a step references a temporary element target that has no mapping"))
			}
		}
		violations = appendUnmappedReferenceViolations(violations, step.Children, mappings)
		if step.ValidationGroup != nil {
			for _, branch := range step.ValidationGroup.Branches {
				violations = appendUnmappedReferenceViolations(violations, branch.Steps, mappings)
			}
		}
	}
	return violations
}

// rewriteSamplingSteps 深拷贝并递归替换步骤及验证分支中的元素目标引用。
func rewriteSamplingSteps(steps []automation.FlowFragmentStep, mappings map[string]automation.SamplingNodeMapping, used map[string]struct{}) []automation.FlowFragmentStep {
	rewritten := automation.CloneFlowFragmentSteps(steps)
	for index := range rewritten {
		step := &rewritten[index]
		if step.ElementTargetID != "" {
			mapping := mappings[step.ElementTargetID]
			used[step.ElementTargetID] = struct{}{}
			step.ElementTargetID = mapping.ElementTargetID
			step.ElementTargetVersionID = mapping.ElementTargetVersionID
		}
		step.Children = rewriteSamplingSteps(step.Children, mappings, used)
		if step.ValidationGroup != nil {
			for branchIndex := range step.ValidationGroup.Branches {
				step.ValidationGroup.Branches[branchIndex].Steps = rewriteSamplingSteps(step.ValidationGroup.Branches[branchIndex].Steps, mappings, used)
			}
		}
	}
	return rewritten
}
