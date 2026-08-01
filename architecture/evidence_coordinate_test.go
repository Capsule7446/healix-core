package architecture_test

import (
	"go/ast"
	"go/token"
	"testing"

	coreengine "github.com/Capsule7446/healix-core/application/engine"
	"github.com/Capsule7446/healix-core/domain/evidence"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/node"
)

func w2TestOccurrenceFields(t *testing.T) {
	t.Helper()
	root := repositoryRoot(t)
	evidenceDir := root + "/domain/evidence"
	violations := 0
	err := walkAllGo(evidenceDir, func(path string, file *ast.File, _ *token.FileSet) {
		if violations > 10 {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			if !ts.Name.IsExported() {
				return true
			}
			hasEntryID := false
			hasOccurrence := false
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 {
					continue
				}
				name := field.Names[0].Name
				if name == "EntryID" {
					hasEntryID = true
				}
				if name == "Occurrence" {
					hasOccurrence = true
				}
			}
			if hasEntryID && !hasOccurrence {
				t.Errorf("evidence struct %s has EntryID but no Occurrence field", ts.Name.Name)
				violations++
			}
			return true
		})
	})
	if err != nil {
		t.Fatalf("walkAllGo: %v", err)
	}
}

func w2TestObservationCoordinateFields(t *testing.T) {
	t.Helper()
	_ = node.OperationObservation{
		InstanceID: mustParseInstanceID(t, "test-instance"),
		EntryID:    mustParseEntryID(t, "test-entry"),
		Occurrence: 1,
		NodeID:     "test-node",
		Operation:  "test",
	}
}

func w2TestStepMetadataHasInvocationPath(t *testing.T) {
	t.Helper()
	_ = coreengine.StepMetadata{
		FlowFragmentStepID: "test",
		DisplayName:        "test",
		Kind:               "STEP",
		HierarchyPath:      "test",
		InvocationPath:     mustParseInvocationPath(t, "test-entry"),
	}
}

func w2TestExistingStepProgressEventAndStepPhaseEventHaveInvocationPath(t *testing.T) {
	t.Helper()
	_ = evidence.StepProgressEvent{
		ID:             mustParseStepExecutionID(t, "test-id"),
		EntryID:        mustParseEntryID(t, "test-entry"),
		InvocationPath: mustParseInvocationPath(t, "test-entry"),
		Occurrence:     1,
		Timestamp:      1,
	}
	_ = evidence.StepPhaseEvent{
		ID:             mustParseStepExecutionID(t, "test-id"),
		EntryID:        mustParseEntryID(t, "test-entry"),
		InvocationPath: mustParseInvocationPath(t, "test-entry"),
		Occurrence:     1,
		Timestamp:      1,
	}
}

func w2TestEvidenceTypesHaveOccurrence(t *testing.T) {
	t.Helper()
	_ = evidence.HealObservation{
		EntryID:    mustParseEntryID(t, "test-entry"),
		Occurrence: 1,
	}
	_ = evidence.ValidationObservation{
		EntryID:    mustParseEntryID(t, "test-entry"),
		Occurrence: 1,
	}
	_ = evidence.ValidationProgressObservation{
		EntryID:    mustParseEntryID(t, "test-entry"),
		Occurrence: 1,
	}
	_ = evidence.ValidationGroupTerminalObservation{
		EntryID:    mustParseEntryID(t, "test-entry"),
		Occurrence: 1,
	}
	_ = evidence.HealCandidateReset{
		EntryID:    mustParseEntryID(t, "test-entry"),
		Occurrence: 1,
	}
	_ = evidence.StepFact{
		EntryID:    mustParseEntryID(t, "test-entry"),
		Occurrence: 1,
	}
}

func mustParseInstanceID(t *testing.T, value string) execution.InstanceID {
	t.Helper()
	id, err := execution.NewInstanceID(value)
	if err != nil {
		t.Fatalf("NewInstanceID: %v", err)
	}
	return id
}

func mustParseEntryID(t *testing.T, value string) execution.EntryID {
	t.Helper()
	id, err := execution.NewEntryID(value)
	if err != nil {
		t.Fatalf("NewEntryID: %v", err)
	}
	return id
}

func mustParseStepExecutionID(t *testing.T, value string) execution.StepExecutionID {
	t.Helper()
	id, err := execution.NewStepExecutionID(value)
	if err != nil {
		t.Fatalf("NewStepExecutionID: %v", err)
	}
	return id
}

func mustParseInvocationPath(t *testing.T, value string) execution.InvocationPath {
	t.Helper()
	p, err := execution.ParseInvocationPath(value)
	if err != nil {
		t.Fatalf("ParseInvocationPath: %v", err)
	}
	return p
}

func TestEvidenceCoordinateW2(t *testing.T) {
	t.Run("evidence_occurrence_fields", w2TestOccurrenceFields)
	t.Run("observation_coordinate_fields", w2TestObservationCoordinateFields)
	t.Run("step_metadata_has_invocation_path", w2TestStepMetadataHasInvocationPath)
	t.Run("event_types_have_invocation_path", w2TestExistingStepProgressEventAndStepPhaseEventHaveInvocationPath)
	t.Run("evidence_types_have_occurrence", w2TestEvidenceTypesHaveOccurrence)
}
