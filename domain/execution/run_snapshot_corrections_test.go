package execution

import (
	"math"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestSealRunSnapshotRejectsBindingsOnRootInvocation(t *testing.T) {
	input := validRunSnapshotInput(t)
	input.Invocations[0].Bindings = map[string]parameter.Binding{
		"count": parameter.LiteralBinding(input.Invocations[0].Values["count"]),
	}

	snapshot, err := SealRunSnapshot(input)

	if err == nil || !strings.Contains(err.Error(), "root invocation cannot have bindings") {
		t.Fatalf("root invocation binding accepted with digest %q: %v", snapshot.Digest(), err)
	}
	if snapshot.Digest() != "" {
		t.Fatalf("rejected snapshot has digest %q", snapshot.Digest())
	}
}

func TestRunSnapshotFreezesCompleteExecutionPlanAndInvocationScopes(t *testing.T) {
	input := validRunSnapshotInput(t)
	input.Plan.Workflows[0].Steps[0].DisplayName = "original"
	sealed, err := SealRunSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	firstDigest := sealed.Digest()
	input.Plan.Workflows[0].Steps[0].DisplayName = "mutated"
	input.Invocations[0].Values["count"] = input.Plan.Entries[0].Parameters.Values["regions"]
	if sealed.Plan().Workflows[0].Steps[0].DisplayName != "original" {
		t.Fatal("plan aliases input")
	}
	copy := sealed.Plan()
	copy.Workflows[0].Steps[0].DisplayName = "getter mutation"
	if sealed.Plan().Workflows[0].Steps[0].DisplayName != "original" {
		t.Fatal("plan getter aliases snapshot")
	}
	changed := validRunSnapshotInput(t)
	changed.Plan.Workflows[0].Steps[0].DisplayName = "changed"
	other, err := SealRunSnapshot(changed)
	if err != nil || other.Digest() == firstDigest {
		t.Fatalf("plan behavior absent from digest: %v", err)
	}
}

func TestRunSnapshotDigestsEnvironmentRevisionAndReferenceProvenance(t *testing.T) {
	base, err := SealRunSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RunSnapshotInput){
		func(v *RunSnapshotInput) { v.Environment.Revision++ },
		func(v *RunSnapshotInput) {
			v.Plan.References = []ReferenceResolution{{ParentVersionID: "workflow-v2", StepID: "call", WorkflowID: "child", WorkflowVersionID: "child-v1", ResolvedFromLatest: true}}
			v.Plan.Workflows[0].Steps = []Step{{ID: "call", DisplayName: "Call", Kind: WorkflowReference, Reference: &Reference{WorkflowID: "child", WorkflowVersionID: "child-v1"}}}
			v.Plan.Workflows = append(v.Plan.Workflows, WorkflowSnapshot{ID: "child", WorkflowID: "child", VersionID: "child-v1", DisplayName: "Child", VersionNumber: 1, Steps: []Step{{ID: "wait-child", DisplayName: "Wait", Kind: WaitStep, WaitKind: "sleep", WaitMS: 1}}})
			v.Invocations = append(v.Invocations, InvocationScopeSnapshot{Path: "entry-1/call", ParentPath: "entry-1", ParentVersionID: "workflow-v2", StepID: "call", WorkflowID: "child", WorkflowVersionID: "child-v1", ResolvedFromLatest: true, Values: map[string]parameter.Value{}, Bindings: map[string]parameter.Binding{}})
		},
	} {
		input := validRunSnapshotInput(t)
		mutate(&input)
		got, err := SealRunSnapshot(input)
		if err != nil {
			t.Fatal(err)
		}
		if got.Digest() == base.Digest() {
			t.Fatal("digest unchanged")
		}
	}
}

func TestHydrateRunSnapshotVerifiesStoredDigest(t *testing.T) {
	input := validRunSnapshotInput(t)
	sealed, err := SealRunSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := HydrateRunSnapshot(input, sealed.Digest()); err != nil {
		t.Fatal(err)
	}
	if _, err := HydrateRunSnapshot(input, "sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Fatal("digest mismatch accepted")
	}
}

func TestPolicyNegativeZeroHasCanonicalDigest(t *testing.T) {
	fields := []func(*HealerPolicySnapshot){
		func(p *HealerPolicySnapshot) { p.ReviewCap = math.Copysign(0, -1) },
		func(p *HealerPolicySnapshot) { p.Weights.Tag = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.ID = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.RoleName = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.Class = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.Attrs = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.Text = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.Index = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.Neighbor = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.LabelText = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.Container = math.Copysign(0, -1) },
	}
	for i, set := range fields {
		negative := validRunSnapshotInput(t)
		positive := validRunSnapshotInput(t)
		set(&negative.HealerPolicy)
		set(&positive.HealerPolicy)
		normalizePolicyPositiveZero(&positive.HealerPolicy)
		a, ea := SealRunSnapshot(negative)
		b, eb := SealRunSnapshot(positive)
		if ea != nil || eb != nil || a.Digest() != b.Digest() {
			t.Fatalf("field %d: %v %v", i, ea, eb)
		}
	}
}
func normalizePolicyPositiveZero(p *HealerPolicySnapshot) {
	if p.ReviewCap == 0 {
		p.ReviewCap = 0
	}
	if p.AppliedCap == 0 {
		p.AppliedCap = 0
	}
	values := []*float64{&p.Weights.Tag, &p.Weights.ID, &p.Weights.RoleName, &p.Weights.Class, &p.Weights.Attrs, &p.Weights.Text, &p.Weights.Index, &p.Weights.Neighbor, &p.Weights.LabelText, &p.Weights.Container}
	for _, v := range values {
		if *v == 0 {
			*v = 0
		}
	}
}

func TestNewRunRejectsInvalidInitialLifecycleShape(t *testing.T) {
	snapshot, err := SealRunSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []Run{{ID: "run-1", TestTaskID: "task-1", TestTaskVersionID: "task-v3", Status: Running, CreatedAt: 1, QueuedAt: 1}, {ID: "run-1", TestTaskID: "task-1", TestTaskVersionID: "task-v3", Status: Succeeded, CreatedAt: 1, QueuedAt: 1}, {ID: "run-1", TestTaskID: "task-1", TestTaskVersionID: "task-v3", Status: "UNKNOWN", CreatedAt: 1, QueuedAt: 1}, {ID: "run-1", TestTaskID: "task-1", TestTaskVersionID: "task-v3", Status: Queued, CreatedAt: 0, QueuedAt: 0}, {ID: "run-1", TestTaskID: "task-1", TestTaskVersionID: "task-v3", Status: Queued, CreatedAt: 2, QueuedAt: 1}}
	for _, run := range tests {
		if _, err := NewRun(run, snapshot); err == nil {
			t.Fatalf("accepted %#v", run)
		}
	}
}
