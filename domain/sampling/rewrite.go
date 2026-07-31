package sampling

import (
	"fmt"
	"strings"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// RewriteUnpublishedElementTargetReferences validates the mapping set as an
// aggregate in slice order. Temporary and formal element target identities are the
// caller's own and never reach public text.
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
	if len(violations) != 0 {
		return nil, publicationMappingInvalidError(violations)
	}
	used := make(map[string]struct{}, len(mappingByTemporaryID))
	rewritten, err := rewriteSamplingSteps(steps, mappingByTemporaryID, used)
	if err != nil {
		return nil, err
	}
	if len(used) != len(mappingByTemporaryID) {
		return nil, publicationMappingInvalidError([]fault.Violation{
			mustViolation(fault.CodeFieldMismatch, "mappings", "mappings must exactly match the referenced temporary element targets"),
		})
	}
	return rewritten, nil
}

func rewriteSamplingSteps(steps []automation.FlowFragmentStep, mappings map[string]automation.SamplingNodeMapping, used map[string]struct{}) ([]automation.FlowFragmentStep, error) {
	rewritten := cloneSamplingSteps(steps)
	for index := range rewritten {
		step := &rewritten[index]
		if step.ElementTargetID != "" {
			mapping, exists := mappings[step.ElementTargetID]
			if !exists {
				// The step id and the temporary element target id are both caller
				// identities, so neither may appear in the public violation.
				return nil, publicationMappingInvalidError([]fault.Violation{
					mustViolation(fault.CodeFieldMismatch, "steps.elementTargetId", "a step references a temporary element target that has no mapping"),
				})
			}
			used[step.ElementTargetID] = struct{}{}
			step.ElementTargetID = mapping.ElementTargetID
			step.ElementTargetVersionID = mapping.ElementTargetVersionID
		}
		children, err := rewriteSamplingSteps(step.Children, mappings, used)
		if err != nil {
			return nil, err
		}
		step.Children = children
		if step.ValidationGroup != nil {
			for branchIndex := range step.ValidationGroup.Branches {
				branchSteps, err := rewriteSamplingSteps(step.ValidationGroup.Branches[branchIndex].Steps, mappings, used)
				if err != nil {
					return nil, err
				}
				step.ValidationGroup.Branches[branchIndex].Steps = branchSteps
			}
		}
	}
	return rewritten, nil
}

func cloneSamplingSteps(steps []automation.FlowFragmentStep) []automation.FlowFragmentStep {
	if steps == nil {
		return nil
	}
	cloned := make([]automation.FlowFragmentStep, len(steps))
	for index, step := range steps {
		cloned[index] = step
		cloned[index].Values = append([]string(nil), step.Values...)
		cloned[index].Children = cloneSamplingSteps(step.Children)
		if step.Reference != nil {
			reference := *step.Reference
			reference.ParameterBindings = make(map[string]parameter.Binding, len(step.Reference.ParameterBindings))
			for name, binding := range step.Reference.ParameterBindings {
				reference.ParameterBindings[name] = binding.Clone()
			}
			cloned[index].Reference = &reference
		}
		if step.Validation != nil {
			validation := *step.Validation
			validation.SupportedKinds = append([]automation.ValidationAssertionKind(nil), step.Validation.SupportedKinds...)
			cloned[index].Validation = &validation
		}
		if step.ValidationGroup != nil {
			group := *step.ValidationGroup
			group.Branches = make([]automation.ValidationBranch, len(step.ValidationGroup.Branches))
			for branchIndex, branch := range step.ValidationGroup.Branches {
				group.Branches[branchIndex] = branch
				group.Branches[branchIndex].Steps = cloneSamplingSteps(branch.Steps)
			}
			cloned[index].ValidationGroup = &group
		}
	}
	return cloned
}
