package execution

import (
	"fmt"
	"testing"
)

func TestBuildSnapshotValidationIndexesIncludesEveryWorkflowReference(t *testing.T) {
	const childCount = 128
	input := validRunSnapshotInput(t)
	root := &input.Plan.Workflows[0]
	root.Steps = make([]Step, childCount)
	input.Plan.References = make([]ReferenceResolution, childCount)
	for index := 0; index < childCount; index++ {
		stepID := fmt.Sprintf("call-%04d", index)
		childID := fmt.Sprintf("child-%04d", index)
		childVersionID := fmt.Sprintf("child-v%04d", index)
		step := Step{ID: stepID, DisplayName: stepID, Kind: FlowFragmentReference, Reference: &Reference{FlowFragmentID: childID, WorkflowVersionID: childVersionID}}
		resolution := ReferenceResolution{ParentVersionID: root.VersionID, StepID: stepID, FlowFragmentID: childID, WorkflowVersionID: childVersionID}
		workflow := WorkflowSnapshot{ID: childID, FlowFragmentID: childID, VersionID: childVersionID, DisplayName: childID, VersionNumber: 1, Steps: []Step{{ID: "wait", DisplayName: "Wait", Kind: WaitStep, WaitKind: "sleep", WaitMS: 1}}}
		root.Steps[index] = step
		input.Plan.References[index] = resolution
		input.Plan.Workflows = append(input.Plan.Workflows, workflow)
	}

	indexes, err := buildSnapshotValidationIndexes(input.Plan)
	if err != nil {
		t.Fatalf("build snapshot validation indexes: %v", err)
	}
	if len(indexes.entriesByID) != len(input.Plan.Entries) || len(indexes.workflows) != len(input.Plan.Workflows) || len(indexes.referenceByEdge) != childCount || len(indexes.referenceSteps) != childCount || len(indexes.stepsByWorkflowID) != len(input.Plan.Workflows) {
		t.Fatalf("index cardinalities = entries %d, workflows %d, resolutions %d, reference steps %d, workflow step lists %d", len(indexes.entriesByID), len(indexes.workflows), len(indexes.referenceByEdge), len(indexes.referenceSteps), len(indexes.stepsByWorkflowID))
	}
	for _, entry := range input.Plan.Entries {
		if got, exists := indexes.entriesByID[entry.ID]; !exists || got.ID != entry.ID {
			t.Fatalf("entry index missing %q", entry.ID)
		}
	}
	for _, workflow := range input.Plan.Workflows {
		if got, exists := indexes.workflows[workflow.VersionID]; !exists || got.VersionID != workflow.VersionID {
			t.Fatalf("workflow index missing %q", workflow.VersionID)
		}
		gotSteps, exists := indexes.stepsByWorkflowID[workflow.VersionID]
		wantSteps := workflowReferenceSteps(workflow.Steps)
		if !exists || len(gotSteps) != len(wantSteps) {
			t.Fatalf("workflow reference steps for %q = %d, want %d", workflow.VersionID, len(gotSteps), len(wantSteps))
		}
		for index, want := range wantSteps {
			if gotSteps[index].ID != want.ID {
				t.Fatalf("workflow reference step %d for %q = %q, want %q", index, workflow.VersionID, gotSteps[index].ID, want.ID)
			}
		}
	}
	for _, resolution := range input.Plan.References {
		key := referenceEdgeKey{ParentVersionID: resolution.ParentVersionID, StepID: resolution.StepID}
		if got, exists := indexes.referenceByEdge[key]; !exists || got != resolution {
			t.Fatalf("reference resolution index missing %+v", key)
		}
		if got, exists := indexes.referenceSteps[key]; !exists || got.ID != resolution.StepID || got.Reference == nil || got.Reference.WorkflowVersionID != resolution.WorkflowVersionID {
			t.Fatalf("reference step index missing %+v", key)
		}
	}
}

func TestBuildSnapshotValidationIndexesIncludesEveryEntry(t *testing.T) {
	const entryCount = 128
	input := validRunSnapshotInput(t)
	parameters := input.Plan.Entries[0].Parameters
	input.ExecutionFlowVersion.Items = make([]ExecutionFlowVersionItemSnapshot, entryCount)
	input.Plan.Entries = make([]Entry, entryCount)
	for index := 0; index < entryCount; index++ {
		itemID := fmt.Sprintf("item-%04d", index)
		executionID := fmt.Sprintf("entry-%04d", index)
		input.ExecutionFlowVersion.Items[index] = ExecutionFlowVersionItemSnapshot{ID: itemID, TestTaskVersionID: input.TestTaskVersionID, SequenceNumber: index + 1, FlowFragmentID: "workflow-1", WorkflowVersionID: "workflow-v2"}
		input.Plan.Entries[index] = Entry{ID: mustEntryID(executionID), TestTaskItemID: itemID, SequenceNumber: index + 1, FlowFragmentID: "workflow-1", WorkflowVersionID: "workflow-v2", Parameters: parameters}
	}

	indexes, err := buildSnapshotValidationIndexes(input.Plan)
	if err != nil {
		t.Fatalf("build snapshot validation indexes: %v", err)
	}
	if len(indexes.entriesByID) != entryCount || len(indexes.workflows) != len(input.Plan.Workflows) || len(indexes.referenceByEdge) != len(input.Plan.References) || len(indexes.referenceSteps) != len(input.Plan.References) || len(indexes.stepsByWorkflowID) != len(input.Plan.Workflows) {
		t.Fatalf("index cardinalities = entries %d, workflows %d, resolutions %d, reference steps %d, workflow step lists %d", len(indexes.entriesByID), len(indexes.workflows), len(indexes.referenceByEdge), len(indexes.referenceSteps), len(indexes.stepsByWorkflowID))
	}
	for _, entry := range input.Plan.Entries {
		got, exists := indexes.entriesByID[entry.ID]
		if !exists || got.ID != entry.ID || got.TestTaskItemID != entry.TestTaskItemID || got.SequenceNumber != entry.SequenceNumber || got.WorkflowVersionID != entry.WorkflowVersionID {
			t.Fatalf("entry index incomplete for %q", entry.ID)
		}
	}
	for _, workflow := range input.Plan.Workflows {
		if got, exists := indexes.workflows[workflow.VersionID]; !exists || got.VersionID != workflow.VersionID {
			t.Fatalf("workflow index missing %q", workflow.VersionID)
		}
		gotSteps, exists := indexes.stepsByWorkflowID[workflow.VersionID]
		wantSteps := workflowReferenceSteps(workflow.Steps)
		if !exists || len(gotSteps) != len(wantSteps) {
			t.Fatalf("workflow reference steps index incomplete for %q", workflow.VersionID)
		}
		for index, want := range wantSteps {
			if gotSteps[index].ID != want.ID {
				t.Fatalf("workflow reference step %d for %q = %q, want %q", index, workflow.VersionID, gotSteps[index].ID, want.ID)
			}
		}
	}
	for _, resolution := range input.Plan.References {
		key := referenceEdgeKey{ParentVersionID: resolution.ParentVersionID, StepID: resolution.StepID}
		if got, exists := indexes.referenceByEdge[key]; !exists || got != resolution {
			t.Fatalf("reference resolution index missing %+v", key)
		}
		if got, exists := indexes.referenceSteps[key]; !exists || got.ID != resolution.StepID || got.Reference == nil || got.Reference.WorkflowVersionID != resolution.WorkflowVersionID {
			t.Fatalf("reference step index missing %+v", key)
		}
	}
}

func TestValidateSnapshotIndexesTestTaskVersionItems(t *testing.T) {
	const itemCount = 256
	input := validRunSnapshotInput(t)
	input.ExecutionFlowVersion.Items = make([]ExecutionFlowVersionItemSnapshot, itemCount)
	input.Plan.Entries = make([]Entry, itemCount)
	for index := 0; index < itemCount; index++ {
		itemID := fmt.Sprintf("item-%04d", index)
		input.ExecutionFlowVersion.Items[index] = ExecutionFlowVersionItemSnapshot{
			ID:                itemID,
			TestTaskVersionID: input.TestTaskVersionID,
			SequenceNumber:    index + 1,
			FlowFragmentID:    "workflow-1",
			WorkflowVersionID: "workflow-v2",
		}
		input.Plan.Entries[index] = Entry{
			ID:                mustEntryID(fmt.Sprintf("entry-%04d", index)),
			TestTaskItemID:    itemID,
			SequenceNumber:    index + 1,
			FlowFragmentID:    "workflow-1",
			WorkflowVersionID: "workflow-v2",
		}
	}
	err := validateTestTaskVersionItemEntries(
		input.TestTaskVersionID,
		input.ExecutionFlowVersion.Items,
		input.Plan.Entries,
	)
	if err != nil {
		t.Fatalf("validate complete ExecutionFlowVersion item-entry correspondence: %v", err)
	}
}
