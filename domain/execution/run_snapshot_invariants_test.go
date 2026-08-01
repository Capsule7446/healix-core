package execution

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestSealRunSnapshotRejectsConcreteChildValuesOutsideTargetParameterContract(t *testing.T) {
	tests := []struct {
		name       string
		wantReason string
		mutate     func(*InstanceSnapshotInput, *InvocationScopeSnapshot)
	}{
		{
			name:       "missing required value",
			wantReason: `parameter "value" is missing`,
			mutate: func(_ *InstanceSnapshotInput, child *InvocationScopeSnapshot) {
				child.Bindings = map[string]parameter.Binding{}
				child.Values = map[string]parameter.Value{}
			},
		},
		{
			name:       "wrong typed literal",
			wantReason: `parameter "value": parameter value type mismatch`,
			mutate: func(_ *InstanceSnapshotInput, child *InvocationScopeSnapshot) {
				child.Bindings["value"] = parameter.LiteralBinding(parameter.TextValue("not-a-number"))
				child.Values["value"] = parameter.TextValue("not-a-number")
			},
		},
		{
			name:       "invalid select option",
			wantReason: `parameter "value": single-select value is not an option`,
			mutate: func(input *InstanceSnapshotInput, child *InvocationScopeSnapshot) {
				input.Plan.Workflows[0].Steps[0].Reference.ParameterBindings["value"] = parameter.LiteralBinding(parameter.SingleSelectValue("allowed"))
				input.Plan.Workflows[1].Parameters[0] = Parameter{
					Name: "value", DisplayName: "Value", Type: parameter.SingleSelect, Required: true, Options: []string{"allowed"},
				}
				child.Bindings["value"] = parameter.LiteralBinding(parameter.SingleSelectValue("forbidden"))
				child.Values["value"] = parameter.SingleSelectValue("forbidden")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := snapshotWithTwoConcreteReferenceEdges(t)
			child := &input.Invocations[2]
			test.mutate(&input, child)

			_, err := SealInstanceSnapshot(input)
			wantPath := fmt.Sprintf("invocation %s parameter values:", child.Path)
			requireCreateInstanceSnapshotRejection(t, err, wantPath, test.wantReason)
		})
	}
}

func TestRunSnapshotRejectsTestTaskEntryBijectionDefects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*InstanceSnapshotInput)
	}{
		{"extra entry", func(v *InstanceSnapshotInput) {
			entry := v.Plan.Entries[0]
			entry.ID = mustEntryID("entry-2")
			entry.TestTaskItemID = "extra"
			entry.SequenceNumber = 2
			v.Plan.Entries = append(v.Plan.Entries, entry)
		}},
		{"sequence mismatch", func(v *InstanceSnapshotInput) { v.ExecutionFlowVersion.Items[0].SequenceNumber = 2 }},
		{"workflow mismatch", func(v *InstanceSnapshotInput) { v.ExecutionFlowVersion.Items[0].FlowFragmentID = "other" }},
		{"workflow version mismatch", func(v *InstanceSnapshotInput) { v.ExecutionFlowVersion.Items[0].WorkflowVersionID = "other-v1" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validRunSnapshotInput(t)
			test.mutate(&input)
			if _, err := SealInstanceSnapshot(input); err == nil {
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
	if _, err := SealInstanceSnapshot(input); err == nil {
		t.Fatal("oversized environment property accepted")
	}
	input = validRunSnapshotInput(t)
	// An oversized path can no longer be constructed at all, so what the seal
	// can still be handed is an unset one. The byte bound moved to
	// ParseInvocationPath, where the coordinate tests cover it.
	input.Invocations[0].Path = InvocationPath{}
	if _, err := SealInstanceSnapshot(input); err == nil {
		t.Fatal("unset invocation path accepted")
	}
}

func TestRunSnapshotSharesAggregateStringBudgetAcrossPlanAndEnvelope(t *testing.T) {
	input := validRunSnapshotInput(t)
	chunk := strings.Repeat("x", MaxStringBytes)
	for index := 0; index < MaxAggregateStringBytes/MaxStringBytes; index++ {
		input.Environment.Properties[string(rune('a'+index%26))+strings.Repeat("k", index/26)] = chunk
	}
	if _, err := SealInstanceSnapshot(input); err == nil {
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
