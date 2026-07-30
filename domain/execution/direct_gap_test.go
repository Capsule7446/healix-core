package execution

import (
	"errors"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestFailurePolicyAndPlanFailurePolicyDirect(t *testing.T) {
	for _, tt := range []struct {
		policy FailurePolicy
		valid  bool
	}{{FailurePolicyStopOnFailure, true}, {FailurePolicyContinueOnFailure, true}, {FailurePolicy(""), false}, {FailurePolicy("CONTINUE"), false}} {
		if got := tt.policy.IsValid(); got != tt.valid {
			t.Errorf("%q.IsValid() = %v", tt.policy, got)
		}
	}
	for _, policy := range []FailurePolicy{FailurePolicyStopOnFailure, FailurePolicyContinueOnFailure} {
		input := validRunSnapshotInput(t)
		input.Plan.FailurePolicy = policy
		plan, err := Seal(input.Plan)
		if err != nil {
			t.Fatal(err)
		}
		if got := plan.FailurePolicy(); got != policy {
			t.Errorf("FailurePolicy() = %q, want %q", got, policy)
		}
	}
}

func TestValidateRunDirect(t *testing.T) {
	snapshot, err := SealRunSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	valid, err := NewRun(Run{ID: "run-1", ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", EnvironmentID: "env-1", Status: Queued, CreatedAt: 1, QueuedAt: 1}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		mutate  func(Run) Run
		wantErr bool
	}{
		{"valid", func(r Run) Run { return r }, false},
		{"missing identity", func(r Run) Run { r.EnvironmentID = ""; return r }, true},
		{"broken digest", func(r Run) Run { r.SnapshotDigest = "sha256:bad"; return r }, true},
		{"invalid lifecycle", func(r Run) Run { r.Status = Running; return r }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateRun(tt.mutate(valid)); (got != nil) != tt.wantErr {
				t.Fatalf("error=%v wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

func TestRunSnapshotNamedAccessorsAndInvocationIsolationDirect(t *testing.T) {
	snapshot, err := SealRunSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion() != RunSnapshotSchemaV1 || snapshot.ExecutionFlowID() != "task-1" || snapshot.TestTaskVersionID() != "task-v3" {
		t.Fatalf("accessors returned wrong identity")
	}
	invocation, ok := snapshot.Invocation("entry-1")
	if !ok {
		t.Fatal("Invocation did not find entry-1")
	}
	invocation.Values["count"] = parameter.TextValue("mutated")
	invocation.Bindings = map[string]parameter.Binding{}
	again, ok := snapshot.Invocation("entry-1")
	if !ok {
		t.Fatal("second Invocation did not find entry-1")
	}
	if again.Values["count"].Type() != parameter.Number {
		t.Fatal("returned Values map aliases snapshot")
	}
	if _, ok := snapshot.Invocation("missing"); ok {
		t.Fatal("unknown invocation found")
	}
}

func TestExecutionStatusDirect(t *testing.T) {
	for _, tt := range []struct {
		status   ExecutionStatus
		terminal bool
	}{{ExecutionPending, false}, {ExecutionRunning, false}, {ExecutionSucceeded, true}, {ExecutionFailed, true}, {ExecutionCanceled, true}, {ExecutionAborted, true}, {ExecutionSkipped, true}, {ExecutionStatus(""), false}} {
		if got := IsTerminalExecutionStatus(tt.status); got != tt.terminal {
			t.Errorf("terminal(%q)=%v", tt.status, got)
		}
	}
	for _, tt := range []struct {
		name     string
		from, to ExecutionStatus
		allowed  bool
	}{{"pending running", ExecutionPending, ExecutionRunning, true}, {"pending failed", ExecutionPending, ExecutionFailed, true}, {"running succeeded", ExecutionRunning, ExecutionSucceeded, true}, {"terminal cannot move", ExecutionSucceeded, ExecutionRunning, false}, {"same", ExecutionRunning, ExecutionRunning, false}, {"unknown", ExecutionStatus(""), ExecutionRunning, false}} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.from.CanTransitionTo(tt.to)
			if (err == nil) != tt.allowed {
				t.Fatalf("error=%v", err)
			}
			if !tt.allowed && !errors.Is(err, ErrInvalidExecutionStatusTransition) {
				t.Fatalf("wrong error: %v", err)
			}
		})
	}
}
