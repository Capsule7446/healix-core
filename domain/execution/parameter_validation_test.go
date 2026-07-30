package execution

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func directParameterDraft(definitions []Parameter, values map[string]parameter.Value) Draft {
	return Draft{RunID: "run", FailurePolicy: FailurePolicyStopOnFailure,
		Entries:   []WorkflowEntry{{ExecutionID: "entry", TestTaskItemID: "item", SequenceNumber: 1, FlowFragmentID: "workflow", WorkflowVersionID: "v1", Parameters: ParameterSnapshot{ID: "snapshot", SchemaVersion: 1, WorkflowVersionID: "v1", Values: values}}},
		Workflows: []WorkflowSnapshot{{ID: "workflow", FlowFragmentID: "workflow", VersionID: "v1", DisplayName: "FlowFragment", VersionNumber: 1, Parameters: definitions, Steps: []Step{{ID: "wait", DisplayName: "Wait", Kind: WaitStep, WaitKind: "sleep", WaitMS: 1}}}},
	}
}

func TestSealIndependentlyRejectsInvalidParameterDefinitionsAndValues(t *testing.T) {
	tests := []struct {
		name        string
		definitions []Parameter
		values      map[string]parameter.Value
	}{
		{"duplicate names", []Parameter{{Name: "x", DisplayName: "X", Type: parameter.Text, Required: true}, {Name: "x", DisplayName: "X2", Type: parameter.Text, Required: true}}, map[string]parameter.Value{"x": parameter.TextValue("v")}},
		{"invalid type", []Parameter{{Name: "x", DisplayName: "X", Type: parameter.Type("DATE"), Required: true}}, map[string]parameter.Value{"x": parameter.TextValue("v")}},
		{"optional missing default", []Parameter{{Name: "x", DisplayName: "X", Type: parameter.Text}}, map[string]parameter.Value{"x": parameter.TextValue("v")}},
		{"required default", []Parameter{{Name: "x", DisplayName: "X", Type: parameter.Text, Required: true, Default: parameter.PresentValue(parameter.TextValue("v"))}}, map[string]parameter.Value{"x": parameter.TextValue("v")}},
		{"blank option", []Parameter{{Name: "x", DisplayName: "X", Type: parameter.SingleSelect, Required: true, Options: []string{" "}}}, map[string]parameter.Value{"x": parameter.SingleSelectValue(" ")}},
		{"duplicate option", []Parameter{{Name: "x", DisplayName: "X", Type: parameter.MultiSelect, Required: true, Options: []string{"a", "a"}}}, map[string]parameter.Value{"x": parameter.MultiSelectValue([]string{"a"})}},
		{"unknown single option", []Parameter{{Name: "x", DisplayName: "X", Type: parameter.SingleSelect, Required: true, Options: []string{"a"}}}, map[string]parameter.Value{"x": parameter.SingleSelectValue("b")}},
		{"unknown multi option", []Parameter{{Name: "x", DisplayName: "X", Type: parameter.MultiSelect, Required: true, Options: []string{"a"}}}, map[string]parameter.Value{"x": parameter.MultiSelectValue([]string{"b"})}},
		{"required nil multi-select", []Parameter{{Name: "x", DisplayName: "X", Type: parameter.MultiSelect, Required: true, Options: []string{"a"}}}, map[string]parameter.Value{"x": parameter.MultiSelectValue(nil)}},
		{"required empty multi-select", []Parameter{{Name: "x", DisplayName: "X", Type: parameter.MultiSelect, Required: true, Options: []string{"a"}}}, map[string]parameter.Value{"x": parameter.MultiSelectValue([]string{})}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Seal(directParameterDraft(test.definitions, test.values)); err == nil {
				t.Fatal("invalid parameters accepted")
			}
		})
	}
}

func TestSealRejectsInvalidSnapshotForParameterlessWorkflow(t *testing.T) {
	tests := []struct {
		name     string
		snapshot ParameterSnapshot
	}{
		{"identity", ParameterSnapshot{ID: "snapshot"}},
		{"schema version", ParameterSnapshot{SchemaVersion: 1}},
		{"workflow version", ParameterSnapshot{WorkflowVersionID: "v1"}},
		{"unknown value", ParameterSnapshot{Values: map[string]parameter.Value{"unknown": parameter.TextValue("x")}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			draft := directParameterDraft(nil, nil)
			draft.Entries[0].Parameters = test.snapshot
			if _, err := Seal(draft); err == nil {
				t.Fatal("invalid parameterless snapshot accepted")
			}
		})
	}
	empty := directParameterDraft(nil, nil)
	empty.Entries[0].Parameters = ParameterSnapshot{}
	if _, err := Seal(empty); err != nil {
		t.Fatalf("empty parameterless snapshot rejected: %v", err)
	}
}

func TestSealRejectsMismatchedParameterizedSnapshotIdentity(t *testing.T) {
	definition := []Parameter{{Name: "x", DisplayName: "X", Type: parameter.Text, Required: true}}
	values := map[string]parameter.Value{"x": parameter.TextValue("value")}
	tests := []func(*ParameterSnapshot){
		func(snapshot *ParameterSnapshot) { snapshot.ID = "" },
		func(snapshot *ParameterSnapshot) { snapshot.SchemaVersion = 0 },
		func(snapshot *ParameterSnapshot) { snapshot.WorkflowVersionID = "other" },
	}
	for _, edit := range tests {
		draft := directParameterDraft(definition, values)
		edit(&draft.Entries[0].Parameters)
		if _, err := Seal(draft); err == nil {
			t.Fatal("invalid parameterized snapshot identity accepted")
		}
	}
}

func TestSealAccountsForParameterOptionCollectionElements(t *testing.T) {
	options := make([]string, MaxAggregateCollectionElements+1)
	for i := range options {
		options[i] = fmt.Sprintf("option-%d", i)
	}
	draft := directParameterDraft([]Parameter{{Name: "choice", DisplayName: "Choice", Type: parameter.SingleSelect, Required: true, Options: options}}, map[string]parameter.Value{"choice": parameter.SingleSelectValue(options[0])})
	if _, err := Seal(draft); err == nil || !strings.Contains(err.Error(), "collection elements") {
		t.Fatalf("error = %v", err)
	}
}

func TestSealAccountsForTypedSnapshotAndMultiSelectResources(t *testing.T) {
	large := strings.Repeat("x", MaxStringBytes+1)
	draft := directParameterDraft([]Parameter{{Name: "regions", DisplayName: "Regions", Type: parameter.MultiSelect, Required: true, Options: []string{large}}}, map[string]parameter.Value{"regions": parameter.MultiSelectValue([]string{large})})
	if _, err := Seal(draft); err == nil || !strings.Contains(err.Error(), "string exceeds") {
		t.Fatalf("error = %v", err)
	}
}
