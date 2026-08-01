package execution

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestRunSnapshotEnvironmentNamesUseSharedValidation(t *testing.T) {
	tests := []struct {
		name   string
		schema RunSnapshotSchema
		key    string
	}{
		{name: "V1 malformed UTF-8", schema: RunSnapshotSchemaV1, key: string([]byte{0xff})},
		{name: "V1 control character", schema: RunSnapshotSchemaV1, key: "bad\nkey"},
		{name: "V2 oversized", schema: RunSnapshotSchemaV2, key: strings.Repeat("x", parameter.MaxNameBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validRunSnapshotInput(t)
			input.SchemaVersion = test.schema
			if test.schema == RunSnapshotSchemaV1 {
				input.Environment.Properties = map[string]string{test.key: "value"}
				input.Environment.Variables = nil
			} else {
				input.Environment.Properties = nil
				input.Environment.Variables = map[string]parameter.Value{test.key: parameter.TextValue("value")}
			}
			if _, err := SealInstanceSnapshot(input); err == nil {
				t.Fatal("invalid environment name accepted")
			}
		})
	}
}

func TestRunSnapshotInvocationOrderIsCanonicalAndDigestIndependent(t *testing.T) {
	input := snapshotWithTwoConcreteReferenceEdges(t)
	canonical, err := SealInstanceSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	for left, right := 0, len(input.Invocations)-1; left < right; left, right = left+1, right-1 {
		input.Invocations[left], input.Invocations[right] = input.Invocations[right], input.Invocations[left]
	}
	reordered, err := SealInstanceSnapshot(input)
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
	if snapshot, err := SealInstanceSnapshot(input); err == nil || snapshot.Digest() != "" {
		t.Fatalf("missing parent accepted: %#v/%v", snapshot, err)
	}
}

func TestRunSnapshotRejectsInvocationParentCycle(t *testing.T) {
	input := snapshotWithTwoConcreteReferenceEdges(t)
	firstChildPath := input.Invocations[1].Path
	secondChildPath := input.Invocations[2].Path
	input.Invocations[1].ParentPath = secondChildPath
	input.Invocations[2].ParentPath = firstChildPath
	snapshot, err := SealInstanceSnapshot(input)
	if snapshot.Digest() != "" {
		t.Fatalf("parent cycle accepted with digest %q", snapshot.Digest())
	}
	requireCreateInstanceSnapshotRejection(t, err, "cycle")
}

func validRunSnapshotInput(t *testing.T) InstanceSnapshotInput {
	t.Helper()
	number, err := parameter.NewNumberValue("1.20")
	if err != nil {
		t.Fatal(err)
	}
	return InstanceSnapshotInput{
		SchemaVersion: RunSnapshotSchemaV1,
		RunID:         "run-1", ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3",
		TestTaskVersionNumber: 3,
		ExecutionFlow:         TestTaskSnapshot{ID: "task-1"},
		ExecutionFlowVersion:  ExecutionFlowVersionSnapshot{ID: "task-v3", ExecutionFlowID: "task-1", VersionNumber: 3, Items: []ExecutionFlowVersionItemSnapshot{{ID: "item-1", TestTaskVersionID: "task-v3", SequenceNumber: 1, FlowFragmentID: "workflow-1", WorkflowVersionID: "workflow-v2"}}},
		Plan: PlanSnapshot{RunID: "run-1", FailurePolicy: FailurePolicyStopOnFailure,
			Entries:   []Entry{{ID: mustEntryID("entry-1"), TestTaskItemID: "item-1", SequenceNumber: 1, FlowFragmentID: "workflow-1", WorkflowVersionID: "workflow-v2", Parameters: ParameterSnapshot{ID: "scope-root", SchemaVersion: 1, WorkflowVersionID: "workflow-v2", Values: map[string]parameter.Value{"count": number, "regions": parameter.MultiSelectValue([]string{"north,east", "south"})}}}},
			Workflows: []WorkflowSnapshot{{ID: "workflow-1", FlowFragmentID: "workflow-1", VersionID: "workflow-v2", DisplayName: "Flow", VersionNumber: 2, Parameters: []Parameter{{Name: "count", DisplayName: "Count", Type: parameter.Number, Required: true}, {Name: "regions", DisplayName: "Regions", Type: parameter.MultiSelect, Required: true, Options: []string{"north,east", "south"}}}, Steps: []Step{{ID: "wait", DisplayName: "Wait", Kind: WaitStep, WaitKind: "sleep", WaitMS: 1}}}},
		},
		Invocations:      []InvocationScopeSnapshot{{Path: "entry-1", ParentPath: "", FlowFragmentID: "workflow-1", WorkflowVersionID: "workflow-v2", Values: map[string]parameter.Value{"count": number, "regions": parameter.MultiSelectValue([]string{"north,east", "south"})}}},
		Environment:      EnvironmentSnapshot{ID: "env-1", Revision: 7, DisplayName: "CI", BaseURL: "https://example.test", Properties: map[string]string{"password": "ordinary-property", "region": "east"}},
		FailurePolicy:    FailurePolicyStopOnFailure,
		ScreenshotPolicy: ScreenshotPolicySnapshot{Version: ScreenshotPolicyV1, Enabled: true, Destination: "artifacts"},
		HealerPolicy:     DefaultHealerPolicySnapshot(),
	}
}

func TestRunSnapshotV1DigestAndTypedHydrationRemainCompatible(t *testing.T) {
	input := validRunSnapshotInput(t)
	sealed, err := SealInstanceSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	const legacyDigest = "sha256:47b58c11e5f5c2fe7b5a8bb764ee01b1f5f04970926a7dee3f3d2ba67cfb93a3"
	if sealed.Digest() != legacyDigest {
		t.Fatalf("V1 digest changed: got %q", sealed.Digest())
	}
	hydrated, err := HydrateInstanceSnapshot(input, legacyDigest)
	if err != nil {
		t.Fatal(err)
	}
	value := hydrated.Environment().Variables["region"]
	if value.Type() != parameter.Text || value.Text() != "east" {
		t.Fatalf("V1 environment value = %#v", value)
	}
}

func TestRunSnapshotV2DigestIsStableTypeSensitiveAndOwnsMultiSelect(t *testing.T) {
	input := validRunSnapshotInput(t)
	input.SchemaVersion = RunSnapshotSchemaV2
	input.Environment.Properties = nil
	input.Environment.Variables = map[string]parameter.Value{
		"flag": parameter.TextValue("true"),
		"list": parameter.MultiSelectValue([]string{"east", "west"}),
	}
	sealed, err := SealInstanceSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	reordered := validRunSnapshotInput(t)
	reordered.SchemaVersion = RunSnapshotSchemaV2
	reordered.Environment.Properties = nil
	reordered.Environment.Variables = map[string]parameter.Value{
		"list": parameter.MultiSelectValue([]string{"east", "west"}),
		"flag": parameter.TextValue("true"),
	}
	other, err := SealInstanceSnapshot(reordered)
	if err != nil || sealed.Digest() != other.Digest() {
		t.Fatalf("V2 digest unstable: %q %q %v", sealed.Digest(), other.Digest(), err)
	}
	typed := validRunSnapshotInput(t)
	typed.SchemaVersion = RunSnapshotSchemaV2
	typed.Environment.Properties = nil
	typed.Environment.Variables = map[string]parameter.Value{
		"flag": parameter.BooleanValue(true),
		"list": parameter.MultiSelectValue([]string{"east", "west"}),
	}
	booleanSnapshot, err := SealInstanceSnapshot(typed)
	if err != nil {
		t.Fatal(err)
	}
	if booleanSnapshot.Digest() == sealed.Digest() {
		t.Fatal("TEXT and BOOLEAN environment values have equal digests")
	}
	exported := sealed.Environment()
	exported.Variables["list"] = parameter.MultiSelectValue([]string{"mutated"})
	if got := sealed.Environment().Variables["list"].MultiSelect(); !reflect.DeepEqual(got, []string{"east", "west"}) {
		t.Fatalf("snapshot aliases MULTI_SELECT: %v", got)
	}
	persisted := sealed.Input()
	persisted.Environment.Variables["list"] = parameter.MultiSelectValue([]string{"persisted mutation"})
	if got := sealed.Input().Environment.Variables["list"].MultiSelect(); !reflect.DeepEqual(got, []string{"east", "west"}) {
		t.Fatalf("Input aliases MULTI_SELECT: %v", got)
	}
	hydrated, err := HydrateInstanceSnapshot(sealed.Input(), sealed.Digest())
	if err != nil {
		t.Fatal(err)
	}
	hydratedEnvironment := hydrated.Environment()
	hydratedEnvironment.Variables["list"] = parameter.MultiSelectValue([]string{"hydrated mutation"})
	if got := hydrated.Environment().Variables["list"].MultiSelect(); !reflect.DeepEqual(got, []string{"east", "west"}) {
		t.Fatalf("hydrated snapshot aliases MULTI_SELECT: %v", got)
	}
}

func TestSealRunSnapshotOwnsDataAndHasStableCanonicalDigest(t *testing.T) {
	input := validRunSnapshotInput(t)
	sealed, err := SealInstanceSnapshot(input)
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
	other, err := SealInstanceSnapshot(reordered)
	if err != nil || sealed.Digest() != other.Digest() {
		t.Fatalf("digest unstable: %q %q %v", sealed.Digest(), other.Digest(), err)
	}
	if len(sealed.Digest()) != 71 || !strings.HasPrefix(sealed.Digest(), "sha256:") {
		t.Fatalf("digest = %q", sealed.Digest())
	}
}

func TestRunSnapshotDigestChangesForExecutionRelevantCategories(t *testing.T) {
	base, err := SealInstanceSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*InstanceSnapshotInput)
	}{
		{"schema", func(v *InstanceSnapshotInput) { v.SchemaVersion++ }},
		{"run", func(v *InstanceSnapshotInput) { v.RunID = "run-2"; v.Plan.RunID = "run-2" }},
		{"task version", func(v *InstanceSnapshotInput) {
			v.TestTaskVersionID = "task-v4"
			v.TestTaskVersionNumber = 4
			v.ExecutionFlowVersion.ID = "task-v4"
			v.ExecutionFlowVersion.VersionNumber = 4
			v.ExecutionFlowVersion.Items[0].TestTaskVersionID = "task-v4"
		}},
		{"scope", func(v *InstanceSnapshotInput) {
			number, _ := parameter.NewNumberValue("2")
			v.Invocations[0].Values["count"] = number
			v.Plan.Entries[0].Parameters.Values["count"] = number
		}},
		{"environment", func(v *InstanceSnapshotInput) { v.Environment.Properties["region"] = "west" }},
		{"failure policy", func(v *InstanceSnapshotInput) {
			v.FailurePolicy = FailurePolicyContinueOnFailure
			v.Plan.FailurePolicy = FailurePolicyContinueOnFailure
		}},
		{"screenshot", func(v *InstanceSnapshotInput) { v.ScreenshotPolicy.Destination = "other" }},
		{"healer", func(v *InstanceSnapshotInput) { v.HealerPolicy.ReviewCap = .5 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validRunSnapshotInput(t)
			test.mutate(&input)
			got, err := SealInstanceSnapshot(input)
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
		mutate func(*InstanceSnapshotInput)
	}{
		{"schema", func(v *InstanceSnapshotInput) { v.SchemaVersion = 0 }}, {"run", func(v *InstanceSnapshotInput) { v.RunID = "" }},
		{"task", func(v *InstanceSnapshotInput) { v.ExecutionFlowID = "" }}, {"task version", func(v *InstanceSnapshotInput) { v.TestTaskVersionID = "" }},
		{"task version number", func(v *InstanceSnapshotInput) { v.TestTaskVersionNumber = 0 }}, {"environment URL", func(v *InstanceSnapshotInput) { v.Environment.BaseURL = "ftp://example.test" }},
		{"property key", func(v *InstanceSnapshotInput) { v.Environment.Properties[" "] = "x" }}, {"property value", func(v *InstanceSnapshotInput) {
			v.Environment.Properties["x"] = strings.Repeat("x", MaxSnapshotStringBytes+1)
		}},
		{"failure policy", func(v *InstanceSnapshotInput) { v.FailurePolicy = "INVALID" }}, {"screenshot version", func(v *InstanceSnapshotInput) { v.ScreenshotPolicy.Version = 0 }},
		{"screenshot destination", func(v *InstanceSnapshotInput) { v.ScreenshotPolicy.Destination = "" }}, {"healer version", func(v *InstanceSnapshotInput) { v.HealerPolicy.Version = 0 }},
		{"healer NaN", func(v *InstanceSnapshotInput) { v.HealerPolicy.ReviewCap = math.NaN() }}, {"healer Inf", func(v *InstanceSnapshotInput) { v.HealerPolicy.Weights.Tag = math.Inf(1) }},
		{"zero healer", func(v *InstanceSnapshotInput) { v.HealerPolicy.Weights = HealerWeightsSnapshot{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validRunSnapshotInput(t)
			test.mutate(&input)
			if _, err := SealInstanceSnapshot(input); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
}

func TestRunSnapshotInputExportSupportsDurableHydrationWithoutAliasing(t *testing.T) {
	sealed, err := SealInstanceSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	exported := sealed.Input()
	exported.Environment.Properties["region"] = "mutated"
	exported.Plan.Entries[0].ID = mustEntryID("mutated")
	if sealed.Environment().Properties["region"] != "east" || sealed.Plan().Entries[0].ID != mustEntryID("entry-1") {
		t.Fatal("export aliases sealed snapshot")
	}
	persisted := sealed.Input()
	hydrated, err := HydrateInstanceSnapshot(persisted, sealed.Digest())
	if err != nil || hydrated.Digest() != sealed.Digest() {
		t.Fatalf("hydrate exported input: digest=%q err=%v", hydrated.Digest(), err)
	}
}

func TestHydrateRunRestoresPrivateSnapshotSeal(t *testing.T) {
	snapshot, err := SealInstanceSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	created, err := NewRun(Run{ID: "run-1", ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", Status: Queued, CreatedAt: 1, QueuedAt: 1}, snapshot)
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
	other, err := SealInstanceSnapshot(otherInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := HydrateRun(persisted, other); err == nil {
		t.Fatal("different snapshot accepted")
	}
}

func TestRunTransitionPreservesSnapshotIdentity(t *testing.T) {
	snapshot, err := SealInstanceSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	run, err := NewRun(Run{ID: "run-1", ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", Status: Queued, CreatedAt: 1, QueuedAt: 1}, snapshot)
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
