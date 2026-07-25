package execution

import (
	"math"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestRunSnapshotInvocationOrderIsCanonicalAndDigestIndependent(t *testing.T) {
	input := snapshotWithTwoConcreteReferenceEdges(t)
	canonical, err := SealRunSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	for left, right := 0, len(input.Invocations)-1; left < right; left, right = left+1, right-1 {
		input.Invocations[left], input.Invocations[right] = input.Invocations[right], input.Invocations[left]
	}
	reordered, err := SealRunSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	if reordered.Digest() != canonical.Digest() {
		t.Fatalf("invocation order changed digest: %s/%s", reordered.Digest(), canonical.Digest())
	}
	ordered := reordered.Invocations()
	for index := 1; index < len(ordered); index++ {
		if ordered[index-1].Path > ordered[index].Path {
			t.Fatal("sealed invocation order is not canonical")
		}
	}
}

func TestRunSnapshotRejectsInvocationWithMissingParentIndependentOfOrder(t *testing.T) {
	input := snapshotWithTwoConcreteReferenceEdges(t)
	input.Invocations[1].ParentPath = "missing"
	input.Invocations[0], input.Invocations[1] = input.Invocations[1], input.Invocations[0]
	if snapshot, err := SealRunSnapshot(input); err == nil || snapshot.Digest() != "" {
		t.Fatalf("missing parent accepted: %#v/%v", snapshot, err)
	}
}

func TestRunSnapshotRejectsInvocationParentCycle(t *testing.T) {
	input := snapshotWithTwoConcreteReferenceEdges(t)
	firstChildPath := input.Invocations[1].Path
	secondChildPath := input.Invocations[2].Path
	input.Invocations[1].ParentPath = secondChildPath
	input.Invocations[2].ParentPath = firstChildPath
	if snapshot, err := SealRunSnapshot(input); err == nil || !strings.Contains(err.Error(), "cycle") || snapshot.Digest() != "" {
		t.Fatalf("parent cycle accepted: %#v/%v", snapshot, err)
	}
}

func validRunSnapshotInput(t *testing.T) RunSnapshotInput {
	t.Helper()
	number, err := parameter.NewNumberValue("1.20")
	if err != nil {
		t.Fatal(err)
	}
	return RunSnapshotInput{
		SchemaVersion: RunSnapshotSchemaV1,
		RunID:         "run-1", TestTaskID: "task-1", TestTaskVersionID: "task-v3",
		TestTaskVersionNumber: 3,
		TestTask:              TestTaskSnapshot{ID: "task-1", CurrentVersionID: "task-v3"},
		TestTaskVersion:       TestTaskVersionSnapshot{ID: "task-v3", TestTaskID: "task-1", VersionNumber: 3, Items: []TestTaskVersionItemSnapshot{{ID: "item-1", TestTaskVersionID: "task-v3", SequenceNumber: 1, WorkflowID: "workflow-1", WorkflowVersionID: "workflow-v2"}}},
		Plan: Draft{RunID: "run-1", FailurePolicy: FailurePolicyStopOnFailure,
			Entries:   []WorkflowEntry{{ExecutionID: "entry-1", TestTaskItemID: "item-1", SequenceNumber: 1, WorkflowID: "workflow-1", WorkflowVersionID: "workflow-v2", Parameters: ParameterSnapshot{ID: "scope-root", SchemaVersion: 1, WorkflowVersionID: "workflow-v2", Values: map[string]parameter.Value{"count": number, "regions": parameter.MultiSelectValue([]string{"north,east", "south"})}}}},
			Workflows: []WorkflowSnapshot{{ID: "workflow-1", WorkflowID: "workflow-1", VersionID: "workflow-v2", DisplayName: "Flow", VersionNumber: 2, Parameters: []Parameter{{Name: "count", DisplayName: "Count", Type: parameter.Number, Required: true}, {Name: "regions", DisplayName: "Regions", Type: parameter.MultiSelect, Required: true, Options: []string{"north,east", "south"}}}, Steps: []Step{{ID: "wait", DisplayName: "Wait", Kind: WaitStep, WaitKind: "sleep", WaitMS: 1}}}},
		},
		Invocations:      []InvocationScopeSnapshot{{Path: "entry-1", ParentPath: "", WorkflowID: "workflow-1", WorkflowVersionID: "workflow-v2", Values: map[string]parameter.Value{"count": number, "regions": parameter.MultiSelectValue([]string{"north,east", "south"})}}},
		Environment:      EnvironmentSnapshot{ID: "env-1", Revision: 7, DisplayName: "CI", BaseURL: "https://example.test", Properties: map[string]string{"password": "ordinary-property", "region": "east"}},
		FailurePolicy:    FailurePolicyStopOnFailure,
		ScreenshotPolicy: ScreenshotPolicySnapshot{Version: ScreenshotPolicyV1, Enabled: true, Destination: "artifacts"},
		HealerPolicy:     DefaultHealerPolicySnapshot(),
	}
}

func TestSealRunSnapshotOwnsDataAndHasStableCanonicalDigest(t *testing.T) {
	input := validRunSnapshotInput(t)
	sealed, err := SealRunSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Environment.Properties["region"] = "mutated"
	input.Plan.Entries[0].Parameters.Values["regions"] = parameter.MultiSelectValue([]string{"mutated"})
	if sealed.Environment().Properties["region"] != "east" || sealed.Plan().Entries[0].Parameters.Values["regions"].MultiSelect()[0] != "north,east" {
		t.Fatal("snapshot aliases input")
	}
	copy := sealed.Environment()
	copy.Properties["region"] = "changed"
	if sealed.Environment().Properties["region"] != "east" {
		t.Fatal("snapshot getter aliases state")
	}
	reordered := validRunSnapshotInput(t)
	reordered.Environment.Properties = map[string]string{"region": "east", "password": "ordinary-property"}
	other, err := SealRunSnapshot(reordered)
	if err != nil || sealed.Digest() != other.Digest() {
		t.Fatalf("digest unstable: %q %q %v", sealed.Digest(), other.Digest(), err)
	}
	if len(sealed.Digest()) != 71 || !strings.HasPrefix(sealed.Digest(), "sha256:") {
		t.Fatalf("digest = %q", sealed.Digest())
	}
}

func TestRunSnapshotDigestChangesForExecutionRelevantCategories(t *testing.T) {
	base, err := SealRunSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*RunSnapshotInput)
	}{
		{"schema", func(v *RunSnapshotInput) { v.SchemaVersion++ }},
		{"run", func(v *RunSnapshotInput) { v.RunID = "run-2"; v.Plan.RunID = "run-2" }},
		{"task version", func(v *RunSnapshotInput) {
			v.TestTaskVersionID = "task-v4"
			v.TestTaskVersionNumber = 4
			v.TestTask.CurrentVersionID = "task-v4"
			v.TestTaskVersion.ID = "task-v4"
			v.TestTaskVersion.VersionNumber = 4
			v.TestTaskVersion.Items[0].TestTaskVersionID = "task-v4"
		}},
		{"scope", func(v *RunSnapshotInput) {
			number, _ := parameter.NewNumberValue("2")
			v.Invocations[0].Values["count"] = number
			v.Plan.Entries[0].Parameters.Values["count"] = number
		}},
		{"environment", func(v *RunSnapshotInput) { v.Environment.Properties["region"] = "west" }},
		{"failure policy", func(v *RunSnapshotInput) {
			v.FailurePolicy = FailurePolicyContinueOnFailure
			v.Plan.FailurePolicy = FailurePolicyContinueOnFailure
		}},
		{"screenshot", func(v *RunSnapshotInput) { v.ScreenshotPolicy.Destination = "other" }},
		{"healer", func(v *RunSnapshotInput) { v.HealerPolicy.ReviewCap = .5 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validRunSnapshotInput(t)
			test.mutate(&input)
			got, err := SealRunSnapshot(input)
			if test.name == "schema" {
				if err == nil {
					return
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Digest() == base.Digest() {
				t.Fatal("digest unchanged")
			}
		})
	}
}

func TestSealRunSnapshotRejectsInvalidIdentityEnvironmentAndPolicies(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RunSnapshotInput)
	}{
		{"schema", func(v *RunSnapshotInput) { v.SchemaVersion = 0 }}, {"run", func(v *RunSnapshotInput) { v.RunID = "" }},
		{"task", func(v *RunSnapshotInput) { v.TestTaskID = "" }}, {"task version", func(v *RunSnapshotInput) { v.TestTaskVersionID = "" }},
		{"task version number", func(v *RunSnapshotInput) { v.TestTaskVersionNumber = 0 }}, {"environment URL", func(v *RunSnapshotInput) { v.Environment.BaseURL = "ftp://example.test" }},
		{"property key", func(v *RunSnapshotInput) { v.Environment.Properties[" "] = "x" }}, {"property value", func(v *RunSnapshotInput) {
			v.Environment.Properties["x"] = strings.Repeat("x", MaxSnapshotStringBytes+1)
		}},
		{"failure policy", func(v *RunSnapshotInput) { v.FailurePolicy = "INVALID" }}, {"screenshot version", func(v *RunSnapshotInput) { v.ScreenshotPolicy.Version = 0 }},
		{"screenshot destination", func(v *RunSnapshotInput) { v.ScreenshotPolicy.Destination = "" }}, {"healer version", func(v *RunSnapshotInput) { v.HealerPolicy.Version = 0 }},
		{"healer NaN", func(v *RunSnapshotInput) { v.HealerPolicy.ReviewCap = math.NaN() }}, {"healer Inf", func(v *RunSnapshotInput) { v.HealerPolicy.Weights.Tag = math.Inf(1) }},
		{"zero healer", func(v *RunSnapshotInput) { v.HealerPolicy.Weights = HealerWeightsSnapshot{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validRunSnapshotInput(t)
			test.mutate(&input)
			if _, err := SealRunSnapshot(input); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
}

func TestRunSnapshotInputExportSupportsDurableHydrationWithoutAliasing(t *testing.T) {
	sealed, err := SealRunSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	exported := sealed.Input()
	exported.Environment.Properties["region"] = "mutated"
	exported.Plan.Entries[0].ExecutionID = "mutated"
	if sealed.Environment().Properties["region"] != "east" || sealed.Plan().Entries[0].ExecutionID != "entry-1" {
		t.Fatal("export aliases sealed snapshot")
	}
	persisted := sealed.Input()
	hydrated, err := HydrateRunSnapshot(persisted, sealed.Digest())
	if err != nil || hydrated.Digest() != sealed.Digest() {
		t.Fatalf("hydrate exported input: digest=%q err=%v", hydrated.Digest(), err)
	}
}

func TestHydrateRunRestoresPrivateSnapshotSeal(t *testing.T) {
	snapshot, err := SealRunSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	created, err := NewRun(Run{ID: "run-1", TestTaskID: "task-1", TestTaskVersionID: "task-v3", Status: Queued, CreatedAt: 1, QueuedAt: 1}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	persisted := created
	persisted.sealedSnapshotDigest = ""
	hydrated, err := HydrateRun(persisted, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hydrated.Transition(Running, 11); err != nil {
		t.Fatalf("transition hydrated run: %v", err)
	}
	tampered := persisted
	tampered.SnapshotDigest = "sha256:" + strings.Repeat("0", 64)
	if _, err := HydrateRun(tampered, snapshot); err == nil {
		t.Fatal("tampered persisted digest accepted")
	}
	otherInput := validRunSnapshotInput(t)
	otherInput.Environment.Properties["region"] = "west"
	other, err := SealRunSnapshot(otherInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := HydrateRun(persisted, other); err == nil {
		t.Fatal("different snapshot accepted")
	}
}

func TestRunTransitionPreservesSnapshotIdentity(t *testing.T) {
	snapshot, err := SealRunSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	run, err := NewRun(Run{ID: "run-1", TestTaskID: "task-1", TestTaskVersionID: "task-v3", Status: Queued, CreatedAt: 1, QueuedAt: 1}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	next, err := run.Transition(Running, 11)
	if err != nil {
		t.Fatal(err)
	}
	if next.SnapshotSchemaVersion != RunSnapshotSchemaV1 || next.SnapshotDigest != snapshot.Digest() || next.TestTaskVersionID != "task-v3" {
		t.Fatal("transition lost snapshot identity")
	}
	invalid := run
	invalid.SnapshotDigest = "changed"
	if _, err := invalid.Transition(Running, 11); err == nil {
		t.Fatal("tampered identity accepted")
	}
}
