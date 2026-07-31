package execution

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestRunStatusTransitionMatrix(t *testing.T) {
	type transition struct {
		from RunStatus
		to   RunStatus
	}
	allowed := map[transition]struct{}{
		{Queued, Running}:    {},
		{Queued, Canceled}:   {},
		{Running, Succeeded}: {},
		{Running, Failed}:    {},
		{Running, Canceled}:  {},
		{Running, Aborted}:   {},
	}
	statuses := []RunStatus{Queued, Running, Succeeded, Failed, Canceled, Aborted, "UNKNOWN"}

	for _, from := range statuses {
		for _, to := range statuses {
			name := string(from) + "_to_" + string(to)
			t.Run(name, func(t *testing.T) {
				_, wantAllowed := allowed[transition{from: from, to: to}]
				err := ValidateRunStatusTransition(from, to)
				if wantAllowed && err != nil {
					t.Fatalf("legal transition rejected: %v", err)
				}
				if !wantAllowed && !fault.IsCode(err, CodeRunStatusTransitionInvalid) {
					t.Fatalf("illegal transition error = %v, want %v", err, CodeRunStatusTransitionInvalid)
				}
			})
		}
	}
}

func TestExecutionStatusTransitionAndTerminalMatrices(t *testing.T) {
	type transition struct {
		from ExecutionStatus
		to   ExecutionStatus
	}
	allowed := map[transition]struct{}{
		{ExecutionPending, ExecutionRunning}:   {},
		{ExecutionPending, ExecutionFailed}:    {},
		{ExecutionPending, ExecutionCanceled}:  {},
		{ExecutionPending, ExecutionSkipped}:   {},
		{ExecutionRunning, ExecutionSucceeded}: {},
		{ExecutionRunning, ExecutionFailed}:    {},
		{ExecutionRunning, ExecutionCanceled}:  {},
		{ExecutionRunning, ExecutionAborted}:   {},
	}
	terminal := map[ExecutionStatus]bool{
		ExecutionPending:   false,
		ExecutionRunning:   false,
		ExecutionSucceeded: true,
		ExecutionFailed:    true,
		ExecutionCanceled:  true,
		ExecutionAborted:   true,
		ExecutionSkipped:   true,
		"UNKNOWN":          false,
	}
	statuses := []ExecutionStatus{
		ExecutionPending,
		ExecutionRunning,
		ExecutionSucceeded,
		ExecutionFailed,
		ExecutionCanceled,
		ExecutionAborted,
		ExecutionSkipped,
		"UNKNOWN",
	}

	for _, status := range statuses {
		if got := IsTerminalExecutionStatus(status); got != terminal[status] {
			t.Errorf("IsTerminalExecutionStatus(%q) = %v, want %v", status, got, terminal[status])
		}
		for _, to := range statuses {
			name := string(status) + "_to_" + string(to)
			t.Run(name, func(t *testing.T) {
				_, wantAllowed := allowed[transition{from: status, to: to}]
				err := ValidateExecutionStatusTransition(status, to)
				if wantAllowed && err != nil {
					t.Fatalf("legal transition rejected: %v", err)
				}
				if !wantAllowed && !fault.IsCode(err, CodeStatusTransitionInvalid) {
					t.Fatalf("illegal transition error = %v, want %v", err, CodeStatusTransitionInvalid)
				}
			})
		}
	}
}

func TestWorkerFenceBoundaryAndStaleErrorContract(t *testing.T) {
	tests := []struct {
		name      string
		fence     WorkerFence
		wantError bool
	}{
		{name: "missing both", wantError: true},
		{name: "missing run", fence: WorkerFence{ClaimToken: "claim"}, wantError: true},
		{name: "missing claim", fence: WorkerFence{RunID: "run"}, wantError: true},
		{name: "minimal opaque identities", fence: WorkerFence{RunID: "r", ClaimToken: "c"}},
		{name: "unicode opaque identities", fence: WorkerFence{RunID: "执行-一", ClaimToken: "领取-一"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.fence.Validate()
			if (err != nil) != test.wantError {
				t.Fatalf("WorkerFence.Validate() error = %v, wantError = %v", err, test.wantError)
			}
		})
	}

	fence := WorkerFence{RunID: "run-sensitive", ClaimToken: "secret-claim-token"}
	err := NewStaleWorkerFenceError()
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Code() != CodeWorkerFenceStale || descriptor.Kind() != fault.Conflict || descriptor.Message() != "worker execution authority is stale" {
		t.Fatalf("stale fence descriptor = %#v, %v", descriptor, ok)
	}
	if strings.Contains(err.Error(), fence.RunID) || strings.Contains(err.Error(), fence.ClaimToken) {
		t.Fatalf("stale fence error exposes identity: %q", err)
	}
}

func TestValidateRunAcceptsEveryLegalLifecycleShape(t *testing.T) {
	snapshot, err := SealRunSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	queued, err := NewRun(Run{
		ID: "run-1", ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", EnvironmentID: "env-1",
		Status: Queued, QueuePosition: 0, CreatedAt: 10, QueuedAt: 10,
	}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	running, err := queued.Transition(Running, 10)
	if err != nil {
		t.Fatal(err)
	}
	canceledQueued, _ := queued.Transition(Canceled, 10)
	canceledRunning, _ := running.Transition(Canceled, 10)
	succeeded, _ := running.Transition(Succeeded, 10)
	failed, _ := running.Transition(Failed, 10)
	aborted, _ := running.Transition(Aborted, 10)

	for _, test := range []struct {
		name string
		run  Run
	}{
		{name: "queued", run: queued},
		{name: "running at queue boundary", run: running},
		{name: "queued cancellation at boundary", run: canceledQueued},
		{name: "running cancellation at boundary", run: canceledRunning},
		{name: "succeeded at boundary", run: succeeded},
		{name: "failed at boundary", run: failed},
		{name: "aborted at boundary", run: aborted},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateRun(test.run); err != nil {
				t.Fatalf("legal lifecycle rejected: %v", err)
			}
		})
	}
}

func TestValidateRunRejectsSingleFactorBoundaryViolations(t *testing.T) {
	snapshot, err := SealRunSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	base, err := NewRun(Run{
		ID: "run-1", ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", EnvironmentID: "env-1",
		Status: Queued, QueuePosition: 0, CreatedAt: 10, QueuedAt: 10,
	}, snapshot)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*Run)
	}{
		{name: "blank run id", mutate: func(run *Run) { run.ID = " \t" }},
		{name: "blank task id", mutate: func(run *Run) { run.ExecutionFlowID = "\n" }},
		{name: "blank task version id", mutate: func(run *Run) { run.TestTaskVersionID = " " }},
		{name: "blank environment id", mutate: func(run *Run) { run.EnvironmentID = " " }},
		{name: "schema below boundary", mutate: func(run *Run) { run.SnapshotSchemaVersion = RunSnapshotSchemaV1 - 1 }},
		{name: "digest missing prefix", mutate: func(run *Run) { run.SnapshotDigest = strings.Repeat("0", 71) }},
		{name: "digest differs from seal", mutate: func(run *Run) { run.SnapshotDigest = "sha256:" + strings.Repeat("0", 64) }},
		{name: "private seal missing", mutate: func(run *Run) { run.sealedSnapshotDigest = "" }},
		{name: "queue position below boundary", mutate: func(run *Run) { run.QueuePosition = -1 }},
		{name: "created timestamp below boundary", mutate: func(run *Run) { run.CreatedAt = 0 }},
		{name: "queue timestamp before creation", mutate: func(run *Run) { run.QueuedAt = run.CreatedAt - 1 }},
		{name: "queued run has start", mutate: func(run *Run) { run.StartedAt = run.QueuedAt }},
		{name: "unknown status", mutate: func(run *Run) { run.Status = "UNKNOWN" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := base
			test.mutate(&run)
			if err := ValidateRun(run); err == nil {
				t.Fatal("invalid run accepted")
			}
		})
	}
}
