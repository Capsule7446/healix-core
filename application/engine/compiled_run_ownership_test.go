package engine

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
)

func TestCompiledRunAccessorsReturnIndependentEntries(t *testing.T) {
	draft := minimalCompilerPlan()
	draft.Workflows[0].Steps = []execution.Step{{
		ID: "click", DisplayName: "Click", Kind: execution.ActionStep, Action: "click",
		ElementTargetID: compilerNodeID, ElementTargetVersionID: compilerNodeV1,
	}}
	draft.Nodes = []execution.NodeSnapshot{compilerNodeSnapshot(compilerNodeV1, "submit")}
	snapshot, err := runSnapshotForCompilerTest(draft, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	entries := compiled.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	metadataID := ""
	for id, metadata := range entries[0].Metadata {
		if metadata.FlowFragmentStepID == "click" {
			metadataID = id
			break
		}
	}
	if metadataID == "" {
		t.Fatal("compiled metadata is empty")
	}
	entries[0].EntryID = mustEntryID("mutated")
	entries[0].Metadata[metadataID] = StepMetadata{DisplayName: "mutated"}
	entries[0].RuntimeNodes[compilerNodeV1] = RuntimeNodeIdentity{ElementTargetID: "mutated"}
	entries = append(entries, CompiledEntry{})

	again, ok := compiled.Entry(mustEntryID("execution-entry"))
	if !ok {
		t.Fatal("execution-entry is missing after caller mutation")
	}
	if again.EntryID != mustEntryID("execution-entry") {
		t.Fatalf("execution id = %q, want execution-entry", again.EntryID)
	}
	if got := again.Metadata[metadataID]; got.DisplayName != "Click" {
		t.Fatalf("metadata aliases caller-owned map: %#v", got)
	}
	if got := again.RuntimeNodes[compilerNodeV1]; got.ElementTargetID != compilerNodeID {
		t.Fatalf("runtime node aliases caller-owned map: %#v", got)
	}
	if got := len(compiled.Entries()); got != 1 {
		t.Fatalf("internal entries changed through returned slice: %d", got)
	}

	again.Metadata[metadataID] = StepMetadata{DisplayName: "mutated again"}
	again.RuntimeNodes[compilerNodeV1] = RuntimeNodeIdentity{ElementTargetID: "mutated again"}
	third, ok := compiled.Entry(mustEntryID("execution-entry"))
	if !ok || third.Metadata[metadataID].DisplayName != "Click" || third.RuntimeNodes[compilerNodeV1].ElementTargetID != compilerNodeID {
		t.Fatalf("Entry exposed internal map ownership: %#v, ok=%t", third, ok)
	}
}

func TestCompiledRunEntryRejectsCorruptedIndexedIdentity(t *testing.T) {
	snapshot, err := runSnapshotForCompilerTest(minimalCompilerPlan(), map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	index := compiled.byID[mustEntryID("execution-entry")]
	original := compiled.entries[index]
	tests := []struct {
		name   string
		mutate func(*CompiledEntry)
	}{
		{name: "public run", mutate: func(entry *CompiledEntry) { entry.InstanceID = mustInstanceID("other") }},
		{name: "sealed run", mutate: func(entry *CompiledEntry) { entry.identity.instanceID = mustInstanceID("other") }},
		{name: "public digest", mutate: func(entry *CompiledEntry) { entry.SnapshotDigest = "other" }},
		{name: "sealed digest", mutate: func(entry *CompiledEntry) { entry.identity.snapshotDigest = "other" }},
		{name: "public execution", mutate: func(entry *CompiledEntry) { entry.EntryID = mustEntryID("other") }},
		{name: "sealed execution", mutate: func(entry *CompiledEntry) { entry.identity.entryID = mustEntryID("other") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled.entries[index] = original
			test.mutate(&compiled.entries[index])
			if _, ok := compiled.Entry(mustEntryID("execution-entry")); ok {
				t.Fatal("corrupted indexed entry was returned")
			}
		})
	}
}
