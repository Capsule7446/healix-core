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
	// The reference walk joins the mapping-set violations, so one malformed
	// mapping and one unmapped reference come back together rather than as two
	// consecutive rejections.
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

// appendUnmappedReferenceViolations walks the whole tree before anything is
// rewritten. Reporting the first unmapped reference and stopping meant a
// workspace with several of them took one publish attempt per bad reference;
// validating first also keeps the rewrite from producing a half-transformed tree
// it then has to throw away. Walk order is the tree's own depth-first order, so
// the report — and the prefix kept at the cap — is a function of the input.
func appendUnmappedReferenceViolations(violations []fault.Violation, steps []automation.FlowFragmentStep, mappings map[string]automation.SamplingNodeMapping) []fault.Violation {
	for _, step := range steps {
		if len(violations) >= fault.MaxViolations {
			return violations
		}
		if step.ElementTargetID != "" {
			if _, exists := mappings[step.ElementTargetID]; !exists {
				// The step id and the temporary element target id are both caller
				// identities, so neither may appear in the public violation.
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

// rewriteSamplingSteps runs only after validation, so every reference resolves.
func rewriteSamplingSteps(steps []automation.FlowFragmentStep, mappings map[string]automation.SamplingNodeMapping, used map[string]struct{}) []automation.FlowFragmentStep {
	rewritten := cloneSamplingSteps(steps)
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
