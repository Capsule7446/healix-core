package sampling

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
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
		return UnpublishedFlowFragment{}, draftIndexOutOfRangeError()
	}
	*steps = slices.Insert(*steps, index, automation.CloneFlowFragmentSteps([]automation.FlowFragmentStep{step})[0])
	return finalizeUnpublishedFlowFragment(next)
}

func UpdateUnpublishedFlowFragmentStep(workflow UnpublishedFlowFragment, step automation.FlowFragmentStep) (UnpublishedFlowFragment, error) {
	if strings.TrimSpace(step.ID) == "" {
		return UnpublishedFlowFragment{}, draftInvalidError([]fault.Violation{
			mustViolation(fault.CodeFieldRequired, "stepId", "step id is required"),
		})
	}
	next := cloneUnpublishedFlowFragment(workflow)
	found := false
	walkSamplingSteps(next.Steps, func(candidate *automation.FlowFragmentStep) {
		if candidate.ID == step.ID {
			*candidate = automation.CloneFlowFragmentSteps([]automation.FlowFragmentStep{step})[0]
			found = true
		}
	})
	if !found {
		return UnpublishedFlowFragment{}, draftStepNotFoundError()
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
		return UnpublishedFlowFragment{}, draftStepNotFoundError()
	}
	next.ValidationCapturedActionIDs = slices.DeleteFunc(next.ValidationCapturedActionIDs, func(id string) bool { return id == stepID })
	return finalizeUnpublishedFlowFragment(next)
}

func MoveUnpublishedFlowFragmentStep(workflow UnpublishedFlowFragment, stepID string, destination FlowFragmentStepContainer, index int) (UnpublishedFlowFragment, error) {
	var moved automation.FlowFragmentStep
	found := false
	walkSamplingSteps(workflow.Steps, func(step *automation.FlowFragmentStep) {
		if step.ID == stepID {
			moved = automation.CloneFlowFragmentSteps([]automation.FlowFragmentStep{*step})[0]
			found = true
		}
	})
	if !found {
		return UnpublishedFlowFragment{}, draftStepNotFoundError()
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
		return UnpublishedFlowFragment{}, draftInvalidError([]fault.Violation{
			mustViolation(fault.CodeFieldInvalid, "orderedStepIds", "reorder requires an exact permutation of the container's step ids"),
		})
	}
	reordered := make([]automation.FlowFragmentStep, len(orderedIDs))
	for index, id := range orderedIDs {
		step, exists := byID[id]
		if !exists {
			return UnpublishedFlowFragment{}, draftInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldInvalid, "orderedStepIds", "reorder requires an exact permutation of the container's step ids"),
			})
		}
		reordered[index] = step
		delete(byID, id)
	}
	if len(byID) != 0 {
		return UnpublishedFlowFragment{}, draftInvalidError([]fault.Violation{
			mustViolation(fault.CodeFieldInvalid, "orderedStepIds", "reorder requires an exact permutation of the container's step ids"),
		})
	}
	*steps = reordered
	return finalizeUnpublishedFlowFragment(next)
}

func DeleteUnpublishedElementTarget(workflow UnpublishedFlowFragment, nodeID string) (UnpublishedFlowFragment, error) {
	next := cloneUnpublishedFlowFragment(workflow)
	for _, node := range next.Nodes {
		if node.ID == nodeID && len(node.StepIDs) != 0 {
			return UnpublishedFlowFragment{}, draftElementTargetInUseError()
		}
	}
	before := len(next.Nodes)
	next.Nodes = slices.DeleteFunc(next.Nodes, func(node UnpublishedElementTarget) bool { return node.ID == nodeID })
	if len(next.Nodes) == before {
		return UnpublishedFlowFragment{}, draftElementTargetNotFoundError()
	}
	return finalizeUnpublishedFlowFragment(next)
}

func locateFlowFragmentStepContainer(workflow *UnpublishedFlowFragment, container FlowFragmentStepContainer) (*[]automation.FlowFragmentStep, error) {
	if container.ParentStepID == "" {
		if container.BranchID != "" {
			return nil, draftInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldMismatch, "container.branchId", "the root container cannot select a branch"),
			})
		}
		return &workflow.Steps, nil
	}
	var result *[]automation.FlowFragmentStep
	parentFound := false
	walkSamplingSteps(workflow.Steps, func(step *automation.FlowFragmentStep) {
		if result != nil || step.ID != container.ParentStepID {
			return
		}
		parentFound = true
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
	if result != nil {
		return result, nil
	}
	// A parent that exists but cannot hold the requested container is a different
	// failure from a parent that is absent, and it has a different fix: pass a
	// container the parent's kind actually supports, rather than create the step.
	// Reporting both as not-found sent the caller after a step that was there.
	if parentFound {
		return nil, draftInvalidError([]fault.Violation{
			mustViolation(fault.CodeFieldMismatch, "container", "the parent step cannot hold the requested container"),
		})
	}
	return nil, draftStepNotFoundError()
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

// validateUnpublishedFlowFragmentIdentity aggregates identity failures in slice
// order. Step paths are unindexed because walkSamplingSteps descends into children
// and validation branches without carrying a path, and inventing a visit ordinal
// would hand the caller a number it cannot index by.
func validateUnpublishedFlowFragmentIdentity(workflow UnpublishedFlowFragment) error {
	var violations []fault.Violation
	if strings.TrimSpace(workflow.ID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "id", "draft flow fragment id is required"))
	}
	elementTargetIDs := make(map[string]struct{}, len(workflow.Nodes))
	for index, node := range workflow.Nodes {
		field := fmt.Sprintf("elementTargets.%d.id", index)
		if strings.TrimSpace(node.ID) == "" {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, field, "draft element target id is required"))
		} else if _, exists := elementTargetIDs[node.ID]; exists {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field, "draft element target id is duplicated"))
		}
		elementTargetIDs[node.ID] = struct{}{}
	}
	stepIDs := map[string]struct{}{}
	// The walk keeps going after the first bad step. Stopping at one meant a draft
	// with two blank step ids reported a single failure, so the caller fixed it and
	// immediately hit the next — the report has to be complete to be actionable.
	// The cap is the only reason to stop early, and the walk order is the tree's
	// own depth-first order, so the kept prefix is a function of the input.
	walkSamplingSteps(workflow.Steps, func(step *automation.FlowFragmentStep) {
		if len(violations) >= fault.MaxViolations {
			return
		}
		if strings.TrimSpace(step.ID) == "" {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.id", "draft step id is required"))
			return
		}
		if _, exists := stepIDs[step.ID]; exists {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, "steps.id", "draft step id is duplicated"))
			return
		}
		stepIDs[step.ID] = struct{}{}
	})
	if len(violations) != 0 {
		return draftInvalidError(violations)
	}
	return nil
}

func cloneUnpublishedFlowFragment(workflow UnpublishedFlowFragment) UnpublishedFlowFragment {
	cloned := workflow
	cloned.Properties = workflow.Properties.Clone()
	cloned.Steps = automation.CloneFlowFragmentSteps(workflow.Steps)
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
