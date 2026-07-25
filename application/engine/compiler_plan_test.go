package engine

import (
	"fmt"
	"strconv"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func compilePlanForTest(plan execution.Plan) (CompiledRun, error) {
	if err := plan.Validate(); err != nil {
		return CompiledRun{}, err
	}
	return compileDraftSnapshotForTest(plan.Snapshot())
}

func compileDraftSnapshotForTest(draft execution.Draft) (CompiledRun, error) {
	snapshot, err := runSnapshotForCompilerTest(draft, map[string]string{})
	if err != nil {
		return CompiledRun{}, err
	}
	return CompileRunSnapshot(snapshot)
}

func runSnapshotForCompilerTest(draft execution.Draft, environmentProperties map[string]string) (execution.RunSnapshot, error) {
	items := make([]execution.TestTaskVersionItemSnapshot, len(draft.Entries))
	invocations := make([]execution.InvocationScopeSnapshot, 0, len(draft.Entries))
	workflows := make(map[string]execution.WorkflowSnapshot, len(draft.Workflows))
	resolutions := make(map[execution.WorkflowReferenceKey]execution.ReferenceResolution, len(draft.References))
	for _, workflow := range draft.Workflows {
		workflows[workflow.VersionID] = workflow
	}
	for _, resolution := range draft.References {
		resolutions[execution.WorkflowReferenceKey{ParentVersionID: resolution.ParentVersionID, StepID: resolution.StepID}] = resolution
	}
	var addInvocation func(string, string, string, string, string, map[string]parameter.Value, int) error
	addInvocation = func(path, parentPath, parentVersionID, stepID, versionID string, values map[string]parameter.Value, depth int) error {
		if depth > execution.MaxWorkflowReferenceDepth {
			return fmt.Errorf("test invocation depth exceeds limit")
		}
		workflow, exists := workflows[versionID]
		if !exists {
			return fmt.Errorf("workflow version %s is missing", versionID)
		}
		invocation := execution.InvocationScopeSnapshot{Path: path, ParentPath: parentPath, ParentVersionID: parentVersionID, StepID: stepID, WorkflowID: workflow.WorkflowID, WorkflowVersionID: versionID, Values: values}
		invocations = append(invocations, invocation)
		for _, step := range workflow.Steps {
			if step.Kind != execution.WorkflowReference || step.Reference == nil {
				continue
			}
			resolution, ok := resolutions[execution.WorkflowReferenceKey{ParentVersionID: versionID, StepID: step.ID}]
			if !ok {
				return fmt.Errorf("reference resolution is missing")
			}
			bindings := step.Reference.ParameterBindings
			childValues := make(map[string]parameter.Value, len(bindings))
			for name, binding := range bindings {
				value, err := binding.Resolve(values)
				if err != nil {
					return err
				}
				childValues[name] = value
			}
			child := workflows[resolution.WorkflowVersionID]
			for _, definition := range child.Parameters {
				if _, exists := childValues[definition.Name]; !exists {
					if value, present := definition.Default.Value(); present {
						childValues[definition.Name] = value
					}
				}
			}
			childPath := path + "/" + strconv.Itoa(len(step.ID)) + ":" + step.ID
			before := len(invocations)
			if err := addInvocation(childPath, path, versionID, step.ID, resolution.WorkflowVersionID, childValues, depth+1); err != nil {
				return err
			}
			invocations[before].Bindings = bindings
		}
		return nil
	}
	for index, entry := range draft.Entries {
		items[index] = execution.TestTaskVersionItemSnapshot{ID: entry.TestTaskItemID, TestTaskVersionID: "task-v1", SequenceNumber: entry.SequenceNumber, WorkflowID: entry.WorkflowID, WorkflowVersionID: entry.WorkflowVersionID}
		if err := addInvocation(entry.ExecutionID, "", "", "", entry.WorkflowVersionID, entry.Parameters.Values, 1); err != nil {
			return execution.RunSnapshot{}, err
		}
	}
	input := execution.RunSnapshotInput{
		SchemaVersion: execution.RunSnapshotSchemaV1,
		RunID:         draft.RunID, TestTaskID: "task", TestTaskVersionID: "task-v1", TestTaskVersionNumber: 1,
		TestTask:        execution.TestTaskSnapshot{ID: "task", CurrentVersionID: "task-v1"},
		TestTaskVersion: execution.TestTaskVersionSnapshot{ID: "task-v1", TestTaskID: "task", VersionNumber: 1, Items: items},
		Plan:            draft, Invocations: invocations,
		Environment:      execution.EnvironmentSnapshot{ID: "env", Revision: 1, DisplayName: "Environment", BaseURL: "https://example.test", Properties: environmentProperties},
		FailurePolicy:    draft.FailurePolicy,
		ScreenshotPolicy: execution.ScreenshotPolicySnapshot{Version: execution.ScreenshotPolicyV1},
		HealerPolicy:     execution.DefaultHealerPolicySnapshot(),
	}
	snapshot, err := execution.SealRunSnapshot(input)
	if err != nil {
		return execution.RunSnapshot{}, err
	}
	return snapshot, nil
}
