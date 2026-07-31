package sampling

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type FlowFragmentStepContainer struct {
	ParentStepID string
	BranchID     string
}

func InsertUnpublishedFlowFragmentStep(workflow UnpublishedFlowFragment, container FlowFragmentStepContainer, index int, step automation.FlowFragmentStep) (UnpublishedFlowFragment, error) {
	next := cloneUnpublishedFlowFragment(workflow)
	steps, err := locateFlowFragmentStepContainer(&next, container)
	if err != nil {
		return UnpublishedFlowFragment{}, err
	}
	if index < 0 || index > len(*steps) {
		return UnpublishedFlowFragment{}, fmt.Errorf("sampling insert index %d is out of range", index)
	}
	*steps = slices.Insert(*steps, index, cloneSamplingSteps([]automation.FlowFragmentStep{step})[0])
	return finalizeUnpublishedFlowFragment(next)
}

func UpdateUnpublishedFlowFragmentStep(workflow UnpublishedFlowFragment, step automation.FlowFragmentStep) (UnpublishedFlowFragment, error) {
	if strings.TrimSpace(step.ID) == "" {
		return UnpublishedFlowFragment{}, fmt.Errorf("sampling step id is required")
	}
	next := cloneUnpublishedFlowFragment(workflow)
	found := false
	walkSamplingSteps(next.Steps, func(candidate *automation.FlowFragmentStep) {
		if candidate.ID == step.ID {
			*candidate = cloneSamplingSteps([]automation.FlowFragmentStep{step})[0]
			found = true
		}
	})
	if !found {
		return UnpublishedFlowFragment{}, fmt.Errorf("sampling step %q was not found", step.ID)
	}
	return finalizeUnpublishedFlowFragment(next)
}

func DeleteUnpublishedFlowFragmentStep(workflow UnpublishedFlowFragment, stepID string) (UnpublishedFlowFragment, error) {
	next := cloneUnpublishedFlowFragment(workflow)
	deleted := false
	var remove func(*[]automation.FlowFragmentStep)
	remove = func(steps *[]automation.FlowFragmentStep) {
		for index := 0; index < len(*steps); index++ {
			step := &(*steps)[index]
			if step.ID == stepID {
				*steps = slices.Delete(*steps, index, index+1)
				deleted = true
				return
			}
			remove(&step.Children)
			if deleted {
				return
			}
			if step.ValidationGroup != nil {
				for branchIndex := range step.ValidationGroup.Branches {
					remove(&step.ValidationGroup.Branches[branchIndex].Steps)
					if deleted {
						return
					}
				}
			}
		}
	}
	remove(&next.Steps)
	if !deleted {
		return UnpublishedFlowFragment{}, fmt.Errorf("sampling step %q was not found", stepID)
	}
	next.ValidationCapturedActionIDs = slices.DeleteFunc(next.ValidationCapturedActionIDs, func(id string) bool { return id == stepID })
	return finalizeUnpublishedFlowFragment(next)
}

func MoveUnpublishedFlowFragmentStep(workflow UnpublishedFlowFragment, stepID string, destination FlowFragmentStepContainer, index int) (UnpublishedFlowFragment, error) {
	var moved automation.FlowFragmentStep
	found := false
	walkSamplingSteps(workflow.Steps, func(step *automation.FlowFragmentStep) {
		if step.ID == stepID {
			moved = cloneSamplingSteps([]automation.FlowFragmentStep{*step})[0]
			found = true
		}
	})
	if !found {
		return UnpublishedFlowFragment{}, fmt.Errorf("sampling step %q was not found", stepID)
	}
	without, err := DeleteUnpublishedFlowFragmentStep(workflow, stepID)
	if err != nil {
		return UnpublishedFlowFragment{}, err
	}
	return InsertUnpublishedFlowFragmentStep(without, destination, index, moved)
}

func ReorderUnpublishedFlowFragmentSteps(workflow UnpublishedFlowFragment, container FlowFragmentStepContainer, orderedIDs []string) (UnpublishedFlowFragment, error) {
	next := cloneUnpublishedFlowFragment(workflow)
	steps, err := locateFlowFragmentStepContainer(&next, container)
	if err != nil {
		return UnpublishedFlowFragment{}, err
	}
	byID := make(map[string]automation.FlowFragmentStep, len(*steps))
	for _, step := range *steps {
		byID[step.ID] = step
	}
	if len(orderedIDs) != len(byID) {
		return UnpublishedFlowFragment{}, fmt.Errorf("sampling reorder requires an exact step permutation")
	}
	reordered := make([]automation.FlowFragmentStep, len(orderedIDs))
	for index, id := range orderedIDs {
		step, exists := byID[id]
		if !exists {
			return UnpublishedFlowFragment{}, fmt.Errorf("sampling reorder requires an exact step permutation")
		}
		reordered[index] = step
		delete(byID, id)
	}
	if len(byID) != 0 {
		return UnpublishedFlowFragment{}, fmt.Errorf("sampling reorder requires an exact step permutation")
	}
	*steps = reordered
	return finalizeUnpublishedFlowFragment(next)
}

func DeleteUnpublishedElementTarget(workflow UnpublishedFlowFragment, nodeID string) (UnpublishedFlowFragment, error) {
	next := cloneUnpublishedFlowFragment(workflow)
	for _, node := range next.Nodes {
		if node.ID == nodeID && len(node.StepIDs) != 0 {
			return UnpublishedFlowFragment{}, fmt.Errorf("sampling node %q is still referenced", nodeID)
		}
	}
	before := len(next.Nodes)
	next.Nodes = slices.DeleteFunc(next.Nodes, func(node UnpublishedElementTarget) bool { return node.ID == nodeID })
	if len(next.Nodes) == before {
		return UnpublishedFlowFragment{}, fmt.Errorf("sampling node %q was not found", nodeID)
	}
	return finalizeUnpublishedFlowFragment(next)
}

func locateFlowFragmentStepContainer(workflow *UnpublishedFlowFragment, container FlowFragmentStepContainer) (*[]automation.FlowFragmentStep, error) {
	if container.ParentStepID == "" {
		if container.BranchID != "" {
			return nil, fmt.Errorf("sampling root container cannot select a branch")
		}
		return &workflow.Steps, nil
	}
	var result *[]automation.FlowFragmentStep
	walkSamplingSteps(workflow.Steps, func(step *automation.FlowFragmentStep) {
		if result != nil || step.ID != container.ParentStepID {
			return
		}
		if container.BranchID == "" {
			if step.Kind != automation.StepRepeat {
				return
			}
			result = &step.Children
			return
		}
		if step.Kind != automation.StepValidationGroup || step.ValidationGroup == nil {
			return
		}
		for index := range step.ValidationGroup.Branches {
			if step.ValidationGroup.Branches[index].ID == container.BranchID {
				result = &step.ValidationGroup.Branches[index].Steps
				return
			}
		}
	})
	if result == nil {
		return nil, fmt.Errorf("sampling step container was not found")
	}
	return result, nil
}

func walkSamplingSteps(steps []automation.FlowFragmentStep, visit func(*automation.FlowFragmentStep)) {
	for index := range steps {
		step := &steps[index]
		visit(step)
		walkSamplingSteps(step.Children, visit)
		if step.ValidationGroup != nil {
			for branchIndex := range step.ValidationGroup.Branches {
				walkSamplingSteps(step.ValidationGroup.Branches[branchIndex].Steps, visit)
			}
		}
	}
}

func finalizeUnpublishedFlowFragment(workflow UnpublishedFlowFragment) (UnpublishedFlowFragment, error) {
	if err := validateUnpublishedFlowFragmentIdentity(workflow); err != nil {
		return UnpublishedFlowFragment{}, err
	}
	if err := RebuildUnpublishedElementTargetReferences(&workflow); err != nil {
		return UnpublishedFlowFragment{}, err
	}
	return workflow, nil
}

func validateUnpublishedFlowFragmentIdentity(workflow UnpublishedFlowFragment) error {
	if strings.TrimSpace(workflow.ID) == "" {
		return fmt.Errorf("temporary sampling workflow id is required")
	}
	nodeIDs := make(map[string]struct{}, len(workflow.Nodes))
	for _, node := range workflow.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			return fmt.Errorf("temporary sampling node id is required")
		}
		if _, exists := nodeIDs[node.ID]; exists {
			return fmt.Errorf("duplicate temporary sampling node %q", node.ID)
		}
		nodeIDs[node.ID] = struct{}{}
	}
	stepIDs := map[string]struct{}{}
	var identityErr error
	walkSamplingSteps(workflow.Steps, func(step *automation.FlowFragmentStep) {
		if identityErr != nil {
			return
		}
		if strings.TrimSpace(step.ID) == "" {
			identityErr = fmt.Errorf("temporary sampling step id is required")
			return
		}
		if _, exists := stepIDs[step.ID]; exists {
			identityErr = fmt.Errorf("duplicate temporary sampling step %q", step.ID)
			return
		}
		stepIDs[step.ID] = struct{}{}
	})
	return identityErr
}

func cloneUnpublishedFlowFragment(workflow UnpublishedFlowFragment) UnpublishedFlowFragment {
	cloned := workflow
	cloned.Properties = workflow.Properties.Clone()
	cloned.Steps = cloneSamplingSteps(workflow.Steps)
	cloned.Parameters = append([]automation.ParameterDefinition(nil), workflow.Parameters...)
	cloned.Nodes = make([]UnpublishedElementTarget, len(workflow.Nodes))
	for index, node := range workflow.Nodes {
		cloned.Nodes[index] = node
		cloned.Nodes[index].Properties = node.Properties.Clone()
		cloned.Nodes[index].Selectors = append([]fingerprint.Selector(nil), node.Selectors...)
		cloned.Nodes[index].StepIDs = append([]string(nil), node.StepIDs...)
		cloned.Nodes[index].Candidates = append([]ElementTargetCandidate(nil), node.Candidates...)
	}
	cloned.ValidationCapturedActionIDs = append([]string(nil), workflow.ValidationCapturedActionIDs...)
	return cloned
}
