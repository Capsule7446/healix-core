package execution

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestRunSnapshotRejectsTestTaskEntryBijectionDefects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RunSnapshotInput)
	}{
		{"extra entry", func(v *RunSnapshotInput) {
			entry := v.Plan.Entries[0]
			entry.ExecutionID = "entry-2"
			entry.TestTaskItemID = "extra"
			entry.SequenceNumber = 2
			v.Plan.Entries = append(v.Plan.Entries, entry)
		}},
		{"sequence mismatch", func(v *RunSnapshotInput) { v.TestTaskVersion.Items[0].SequenceNumber = 2 }},
		{"workflow mismatch", func(v *RunSnapshotInput) { v.TestTaskVersion.Items[0].WorkflowID = "other" }},
		{"workflow version mismatch", func(v *RunSnapshotInput) { v.TestTaskVersion.Items[0].WorkflowVersionID = "other-v1" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validRunSnapshotInput(t)
			test.mutate(&input)
			if _, err := SealRunSnapshot(input); err == nil {
				t.Fatal("invalid correspondence accepted")
			}
		})
	}
}

func TestValidateBindingsRejectsUnknownLiteralAndParentNames(t *testing.T) {
	parent := []Parameter{{Name: "source", DisplayName: "Source", Type: parameter.Text, Required: true}}
	child := []Parameter{{Name: "known", DisplayName: "Known", Type: parameter.Text, Required: true}}
	for name, binding := range map[string]parameter.Binding{"literal": parameter.LiteralBinding(parameter.TextValue("x")), "parent": parameter.ParentReferenceBinding("source")} {
		t.Run(name, func(t *testing.T) {
			err := validateBindings(parent, child, map[string]parameter.Binding{"unknown": binding, "known": parameter.LiteralBinding(parameter.TextValue("ok"))})
			if err == nil || !strings.Contains(err.Error(), "unknown") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestRunSnapshotAggregateStringBytesIncludesEnvironmentAndInvocation(t *testing.T) {
	input := validRunSnapshotInput(t)
	input.Environment.Properties["large"] = strings.Repeat("x", MaxStringBytes+1)
	if _, err := SealRunSnapshot(input); err == nil {
		t.Fatal("oversized environment property accepted")
	}
	input = validRunSnapshotInput(t)
	input.Invocations[0].Path = strings.Repeat("x", MaxStringBytes+1)
	if _, err := SealRunSnapshot(input); err == nil {
		t.Fatal("oversized invocation path accepted")
	}
}

func TestRunSnapshotSharesAggregateStringBudgetAcrossPlanAndEnvelope(t *testing.T) {
	input := validRunSnapshotInput(t)
	chunk := strings.Repeat("x", MaxStringBytes)
	for index := 0; index < MaxAggregateStringBytes/MaxStringBytes; index++ {
		input.Environment.Properties[string(rune('a'+index%26))+strings.Repeat("k", index/26)] = chunk
	}
	if _, err := SealRunSnapshot(input); err == nil {
		t.Fatal("plan plus envelope exceeding the shared string budget was accepted")
	}
}

func TestConsumeElementsRejectsOverflowShapedCountWithoutAllocation(t *testing.T) {
	remaining := 1
	if err := consumeElements(int(^uint(0)>>1), &remaining); err == nil {
		t.Fatal("overflow-shaped element count accepted")
	}
	if remaining != 1 {
		t.Fatalf("remaining budget changed after rejection: %d", remaining)
	}
}
