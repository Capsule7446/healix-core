package execution

import (
	"fmt"
	"testing"
)

func TestValidateSnapshotIndexesTestTaskVersionItems(t *testing.T) {
	const itemCount = 256
	input := validRunSnapshotInput(t)
	input.TestTaskVersion.Items = make([]TestTaskVersionItemSnapshot, itemCount)
	input.Plan.Entries = make([]WorkflowEntry, itemCount)
	for index := 0; index < itemCount; index++ {
		itemID := fmt.Sprintf("item-%04d", index)
		input.TestTaskVersion.Items[index] = TestTaskVersionItemSnapshot{
			ID:                itemID,
			TestTaskVersionID: input.TestTaskVersionID,
			SequenceNumber:    index + 1,
			WorkflowID:        "workflow-1",
			WorkflowVersionID: "workflow-v2",
		}
		input.Plan.Entries[index] = WorkflowEntry{
			ExecutionID:       fmt.Sprintf("entry-%04d", index),
			TestTaskItemID:    itemID,
			SequenceNumber:    index + 1,
			WorkflowID:        "workflow-1",
			WorkflowVersionID: "workflow-v2",
		}
	}
	lookups := 0
	err := validateTestTaskVersionItemEntries(
		input.TestTaskVersionID,
		input.TestTaskVersion.Items,
		input.Plan.Entries,
		func() { lookups++ },
	)
	if err != nil {
		t.Fatalf("validate complete TestTaskVersion item-entry correspondence: %v", err)
	}
	if lookups != itemCount {
		t.Fatalf("TestTaskVersion item lookups = %d, want %d", lookups, itemCount)
	}
}
