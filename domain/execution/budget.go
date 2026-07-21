package execution

import "fmt"

type executionCost struct {
	executions int64
	waitMS     int64
}

type executionBudgetEvaluator struct {
	workflows   map[string]WorkflowSnapshot
	resolutions map[WorkflowReferenceKey]ReferenceResolution
	memo        map[string]executionCost
	visits      map[string]int
}

func validateExecutionBudget(rootVersionIDs []string, workflows map[string]WorkflowSnapshot, resolutions map[WorkflowReferenceKey]ReferenceResolution) error {
	evaluator := newExecutionBudgetEvaluator(workflows, resolutions)
	total := executionCost{}
	for _, rootVersionID := range rootVersionIDs {
		cost, err := evaluator.workflowCost(rootVersionID)
		if err != nil {
			return err
		}
		total = addCost(total, cost)
	}
	if total.executions > MaxExpandedExecutions {
		return fmt.Errorf("expanded executions exceed maximum %d", MaxExpandedExecutions)
	}
	if total.waitMS > MaxCumulativeWaitMS {
		return fmt.Errorf("cumulative wait exceeds maximum %dms", MaxCumulativeWaitMS)
	}
	return nil
}

func newExecutionBudgetEvaluator(workflows map[string]WorkflowSnapshot, resolutions map[WorkflowReferenceKey]ReferenceResolution) *executionBudgetEvaluator {
	return &executionBudgetEvaluator{
		workflows: workflows, resolutions: resolutions,
		memo:   make(map[string]executionCost, len(workflows)),
		visits: make(map[string]int, len(workflows)),
	}
}

func (e *executionBudgetEvaluator) workflowCost(versionID string) (executionCost, error) {
	if cost, exists := e.memo[versionID]; exists {
		return cost, nil
	}
	workflow, exists := e.workflows[versionID]
	if !exists {
		return executionCost{}, fmt.Errorf("workflow version %q is missing", versionID)
	}
	e.visits[versionID]++
	cost, err := e.stepsCost(versionID, workflow.Steps)
	if err != nil {
		return executionCost{}, err
	}
	e.memo[versionID] = cost
	return cost, nil
}

func (e *executionBudgetEvaluator) stepsCost(parentVersionID string, steps []Step) (executionCost, error) {
	cost := executionCost{}
	for _, step := range steps {
		stepCost := executionCost{executions: 1}
		switch step.Kind {
		case WaitStep:
			stepCost.waitMS = int64(step.WaitMS)
		case ValidationStep:
			if step.Validation != nil {
				stepCost.waitMS = int64(step.Validation.MaxWaitMS)
			}
		case ValidationGroupStep:
			if step.ValidationGroup != nil {
				stepCost.waitMS = int64(step.ValidationGroup.MaxWaitMS)
				for _, branch := range step.ValidationGroup.Branches {
					branchCost, err := e.stepsCost(parentVersionID, branch.Steps)
					if err != nil {
						return executionCost{}, err
					}
					stepCost = addCost(stepCost, branchCost)
				}
			}
		case RepeatStep:
			childCost, err := e.stepsCost(parentVersionID, step.Children)
			if err != nil {
				return executionCost{}, err
			}
			stepCost = addCost(stepCost, multiplyCost(childCost, int64(step.RepeatCount)))
		case WorkflowReference:
			resolution, exists := e.resolutions[WorkflowReferenceKey{ParentVersionID: parentVersionID, StepID: step.ID}]
			if !exists {
				return executionCost{}, fmt.Errorf("workflow reference step %q has no resolution", step.ID)
			}
			childCost, err := e.workflowCost(resolution.WorkflowVersionID)
			if err != nil {
				return executionCost{}, err
			}
			stepCost = addCost(stepCost, childCost)
		}
		cost = addCost(cost, stepCost)
		if cost.executions > MaxExpandedExecutions || cost.waitMS > MaxCumulativeWaitMS {
			return cost, nil
		}
	}
	return cost, nil
}

func workflowExecutionCost(versionID string, multiplier int64, workflows map[string]WorkflowSnapshot, resolutions map[WorkflowReferenceKey]ReferenceResolution) (executionCost, error) {
	cost, err := newExecutionBudgetEvaluator(workflows, resolutions).workflowCost(versionID)
	return multiplyCost(cost, multiplier), err
}

func executionStepsCost(parentVersionID string, steps []Step, multiplier int64, workflows map[string]WorkflowSnapshot, resolutions map[WorkflowReferenceKey]ReferenceResolution) (executionCost, error) {
	cost, err := newExecutionBudgetEvaluator(workflows, resolutions).stepsCost(parentVersionID, steps)
	return multiplyCost(cost, multiplier), err
}

func multiplyCost(cost executionCost, multiplier int64) executionCost {
	return executionCost{
		executions: cappedMultiply(cost.executions, multiplier, MaxExpandedExecutions+1),
		waitMS:     cappedMultiply(cost.waitMS, multiplier, MaxCumulativeWaitMS+1),
	}
}

func addCost(a, b executionCost) executionCost {
	return executionCost{
		executions: cappedAdd(a.executions, b.executions, MaxExpandedExecutions+1),
		waitMS:     cappedAdd(a.waitMS, b.waitMS, MaxCumulativeWaitMS+1),
	}
}

func cappedAdd(a, b, cap int64) int64 {
	if a >= cap || b >= cap-a {
		return cap
	}
	return a + b
}

func cappedMultiply(a, b, cap int64) int64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a >= cap || b > cap/a {
		return cap
	}
	return a * b
}
