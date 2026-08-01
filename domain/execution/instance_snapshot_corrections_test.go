package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestSealInstanceSnapshotRejectsBindingsOnRootInvocation(t *testing.T) {
	input := validInstanceSnapshotInput(t)
	input.Invocations[0].Bindings = map[string]parameter.Binding{
		"count": parameter.LiteralBinding(input.Invocations[0].Values["count"]),
	}

	snapshot, err := SealInstanceSnapshot(input)

	requireCreateInstanceSnapshotRejection(t, err, "root invocation cannot have bindings")
	if snapshot.Digest() != "" {
		t.Fatalf("rejected snapshot has digest %q", snapshot.Digest())
	}
}

func TestInstanceSnapshotFreezesCompleteExecutionPlanAndInvocationScopes(t *testing.T) {
	input := validInstanceSnapshotInput(t)
	input.Plan.Workflows[0].Steps[0].DisplayName = "original"
	sealed, err := SealInstanceSnapshot(input)
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
	changed := validInstanceSnapshotInput(t)
	changed.Plan.Workflows[0].Steps[0].DisplayName = "changed"
	other, err := SealInstanceSnapshot(changed)
	if err != nil || other.Digest() == firstDigest {
		t.Fatalf("plan behavior absent from digest: %v", err)
	}
}

func TestInstanceSnapshotDigestsEnvironmentRevisionAndReferenceProvenance(t *testing.T) {
	base, err := SealInstanceSnapshot(validInstanceSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*InstanceSnapshotInput){
		func(v *InstanceSnapshotInput) { v.Environment.Revision++ },
		func(v *InstanceSnapshotInput) {
			v.Plan.References = []ReferenceResolution{{ParentVersionID: "workflow-v2", StepID: "call", FlowFragmentID: "child", WorkflowVersionID: "child-v1", ResolvedFromLatest: true}}
			v.Plan.Workflows[0].Steps = []Step{{ID: "call", DisplayName: "Call", Kind: FlowFragmentReference, Reference: &Reference{FlowFragmentID: "child", WorkflowVersionID: "child-v1"}}}
			v.Plan.Workflows = append(v.Plan.Workflows, WorkflowSnapshot{ID: "child", FlowFragmentID: "child", VersionID: "child-v1", DisplayName: "Child", VersionNumber: 1, Steps: []Step{{ID: "wait-child", DisplayName: "Wait", Kind: WaitStep, WaitKind: "sleep", WaitMS: 1}}})
			v.Invocations = append(v.Invocations, InvocationScopeSnapshot{Path: mustInvocationPath("entry-1/4:call"), ParentPath: mustInvocationPath("entry-1"), ParentVersionID: "workflow-v2", StepID: "call", FlowFragmentID: "child", WorkflowVersionID: "child-v1", ResolvedFromLatest: true, Values: map[string]parameter.Value{}, Bindings: map[string]parameter.Binding{}})
		},
	} {
		input := validInstanceSnapshotInput(t)
		mutate(&input)
		got, err := SealInstanceSnapshot(input)
		if err != nil {
			t.Fatal(err)
		}
		if got.Digest() == base.Digest() {
			t.Fatal("digest unchanged")
		}
	}
}

func TestSealInstanceSnapshotRejectsZeroEnvironmentRevision(t *testing.T) {
	input := validInstanceSnapshotInput(t)
	input.Environment.Revision = 0

	snapshot, err := SealInstanceSnapshot(input)

	if err == nil {
		t.Fatal("zero environment revision accepted")
	}
	if !reflect.DeepEqual(snapshot, InstanceSnapshot{}) {
		t.Fatalf("rejected snapshot is not zero value: %#v", snapshot)
	}
	if snapshot.Digest() != "" {
		t.Fatalf("rejected snapshot has digest %q", snapshot.Digest())
	}
}

func TestHydrateInstanceSnapshotRejectsZeroEnvironmentRevisionBeforeStoredDigest(t *testing.T) {
	input := validInstanceSnapshotInput(t)
	input.Environment.Revision = 0
	canonicalInput := cloneSnapshotInput(input)
	sort.Slice(canonicalInput.Invocations, func(i, j int) bool {
		return canonicalInput.Invocations[i].Path.String() < canonicalInput.Invocations[j].Path.String()
	})
	normalizeHealerZeros(&canonicalInput.HealerPolicy)
	digester := sha256.New()
	encoder := canonicalEncoder{writer: digester}
	encodeSnapshot(&encoder, canonicalInput)
	storedDigest := "sha256:" + hex.EncodeToString(digester.Sum(nil))

	snapshot, err := HydrateInstanceSnapshot(input, storedDigest)

	if err == nil {
		t.Fatal("zero environment revision accepted during hydration")
	}
	if !reflect.DeepEqual(snapshot, InstanceSnapshot{}) {
		t.Fatalf("rejected hydrated snapshot is not zero value: %#v", snapshot)
	}
	if snapshot.Digest() != "" {
		t.Fatalf("rejected hydrated snapshot has digest %q", snapshot.Digest())
	}
}

func TestHydrateInstanceSnapshotVerifiesStoredDigest(t *testing.T) {
	input := validInstanceSnapshotInput(t)
	sealed, err := SealInstanceSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := HydrateInstanceSnapshot(input, sealed.Digest()); err != nil {
		t.Fatal(err)
	}
	if _, err := HydrateInstanceSnapshot(input, "sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Fatal("digest mismatch accepted")
	}
}

func TestPolicyNegativeZeroHasCanonicalDigest(t *testing.T) {
	fields := []func(*HealerPolicySnapshot){
		func(p *HealerPolicySnapshot) { p.ReviewCap = math.Copysign(0, -1) },
		func(p *HealerPolicySnapshot) { p.Weights.Tag = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.ID = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.RoleName = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.Class = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.Attrs = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.Text = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.Index = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.Neighbor = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.LabelText = math.Copysign(0, -1) }, func(p *HealerPolicySnapshot) { p.Weights.Container = math.Copysign(0, -1) },
	}
	for i, set := range fields {
		negative := validInstanceSnapshotInput(t)
		positive := validInstanceSnapshotInput(t)
		set(&negative.HealerPolicy)
		set(&positive.HealerPolicy)
		normalizePolicyPositiveZero(&positive.HealerPolicy)
		a, ea := SealInstanceSnapshot(negative)
		b, eb := SealInstanceSnapshot(positive)
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

func TestNewInstanceRejectsInvalidInitialLifecycleShape(t *testing.T) {
	snapshot, err := SealInstanceSnapshot(validInstanceSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []Instance{{ID: mustInstanceID("run-1"), ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", Status: Running, CreatedAt: 1, QueuedAt: 1}, {ID: mustInstanceID("run-1"), ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", Status: Succeeded, CreatedAt: 1, QueuedAt: 1}, {ID: mustInstanceID("run-1"), ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", Status: "UNKNOWN", CreatedAt: 1, QueuedAt: 1}, {ID: mustInstanceID("run-1"), ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", Status: Queued, CreatedAt: 0, QueuedAt: 0}, {ID: mustInstanceID("run-1"), ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", Status: Queued, CreatedAt: 2, QueuedAt: 1}}
	for _, run := range tests {
		if _, err := NewInstance(run, snapshot); err == nil {
			t.Fatalf("accepted %#v", run)
		}
	}
}
