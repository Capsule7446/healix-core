package sampling

import (
	"fmt"
	"github.com/Capsule7446/healix-core/domain/parameter"
	"strings"

	"github.com/Capsule7446/healix-core/domain/automation"
)

func RewriteTemporaryNodeReferences(steps []automation.WorkflowStep, mappings []automation.SamplingNodeMapping) ([]automation.WorkflowStep, error) {
	mappingByTemporaryID := make(map[string]automation.SamplingNodeMapping, len(mappings))
	for _, mapping := range mappings {
		if strings.TrimSpace(mapping.TemporaryNodeID) == "" || strings.TrimSpace(mapping.NodeID) == "" || strings.TrimSpace(mapping.NodeVersionID) == "" {
			return nil, fmt.Errorf("sampling node mapping requires temporary and formal identity")
		}
		if _, exists := mappingByTemporaryID[mapping.TemporaryNodeID]; exists {
			return nil, fmt.Errorf("duplicate sampling node mapping %q", mapping.TemporaryNodeID)
		}
		mappingByTemporaryID[mapping.TemporaryNodeID] = mapping
	}
	used := make(map[string]struct{}, len(mappingByTemporaryID))
	rewritten, err := rewriteSamplingSteps(steps, mappingByTemporaryID, used)
	if err != nil {
		return nil, err
	}
	if len(used) != len(mappingByTemporaryID) {
		return nil, fmt.Errorf("sampling node mappings must exactly match referenced temporary nodes")
	}
	return rewritten, nil
}

func rewriteSamplingSteps(steps []automation.WorkflowStep, mappings map[string]automation.SamplingNodeMapping, used map[string]struct{}) ([]automation.WorkflowStep, error) {
	rewritten := cloneSamplingSteps(steps)
	for index := range rewritten {
		step := &rewritten[index]
		if step.NodeID != "" {
			mapping, exists := mappings[step.NodeID]
			if !exists {
				return nil, fmt.Errorf("sampling step %q references unmapped temporary node %q", step.ID, step.NodeID)
			}
			used[step.NodeID] = struct{}{}
			step.NodeID = mapping.NodeID
			step.NodeVersionID = mapping.NodeVersionID
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

func cloneSamplingSteps(steps []automation.WorkflowStep) []automation.WorkflowStep {
	if steps == nil {
		return nil
	}
	cloned := make([]automation.WorkflowStep, len(steps))
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
