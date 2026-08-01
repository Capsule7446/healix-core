package execution

import "testing"

func TestRunTransitionRejectsMalformedReceiverLifecycle(t *testing.T) {
	snapshot, err := SealInstanceSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	queued, err := NewRun(Run{ID: "run-1", ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", Status: Queued, CreatedAt: 10, QueuedAt: 10}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	running, err := queued.Transition(Running, 11)
	if err != nil {
		t.Fatal(err)
	}
	succeeded, _ := running.Transition(Succeeded, 12)
	failed, _ := running.Transition(Failed, 12)
	aborted, _ := running.Transition(Aborted, 12)
	canceledQueued, _ := queued.Transition(Canceled, 12)
	canceledRunning, _ := running.Transition(Canceled, 12)
	tests := []struct {
		name   string
		source Run
		to     InstanceStatus
		mutate func(*Run)
	}{
		{"queued forbidden start", queued, Running, func(run *Run) { run.StartedAt = 10 }},
		{"queued forbidden finish", queued, Canceled, func(run *Run) { run.FinishedAt = 10 }},
		{"queued before created", queued, Running, func(run *Run) { run.QueuedAt = 9 }},
		{"queued missing created", queued, Running, func(run *Run) { run.CreatedAt = 0 }},
		{"queued negative position", queued, Running, func(run *Run) { run.QueuePosition = -1 }},
		{"running missing start", running, Succeeded, func(run *Run) { run.StartedAt = 0 }},
		{"running start before queue", running, Failed, func(run *Run) { run.StartedAt = 9 }},
		{"running forbidden finish", running, Aborted, func(run *Run) { run.FinishedAt = 12 }},
		{"running queue before created", running, Canceled, func(run *Run) { run.QueuedAt = 9 }},
		{"succeeded missing finish", succeeded, Running, func(run *Run) { run.FinishedAt = 0 }},
		{"failed reversed finish", failed, Running, func(run *Run) { run.FinishedAt = 10 }},
		{"aborted missing start", aborted, Running, func(run *Run) { run.StartedAt = 0 }},
		{"queued cancellation missing finish", canceledQueued, Running, func(run *Run) { run.FinishedAt = 0 }},
		{"running cancellation reversed", canceledRunning, Running, func(run *Run) { run.FinishedAt = 10 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := test.source
			test.mutate(&source)
			if _, err := source.Transition(test.to, 20); err == nil {
				t.Fatal("transition accepted malformed receiver")
			}
		})
	}
}

func TestRunTransitionRoundTripsThroughHydration(t *testing.T) {
	snapshot, err := SealInstanceSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	queued, err := NewRun(Run{ID: "run-1", ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", Status: Queued, CreatedAt: 10, QueuedAt: 10}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	running, err := queued.Transition(Running, 11)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		source Run
		status InstanceStatus
		at     int64
	}{
		{"queued canceled", queued, Canceled, 11},
		{"running canceled", running, Canceled, 12},
		{"succeeded", running, Succeeded, 12},
		{"failed", running, Failed, 12},
		{"aborted", running, Aborted, 12},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			next, err := test.source.Transition(test.status, test.at)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := HydrateRun(next, snapshot); err != nil {
				t.Fatalf("transition produced non-hydratable run: %v", err)
			}
		})
	}
	if _, err := queued.Transition(Running, 9); err == nil {
		t.Fatal("start before queue accepted")
	}
	if _, err := running.Transition(Succeeded, 10); err == nil {
		t.Fatal("finish before start accepted")
	}
}

func TestHydrateRunEnforcesPersistedLifecycleShapes(t *testing.T) {
	snapshot, err := SealInstanceSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	base, err := NewRun(Run{ID: "run-1", ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", Status: Queued, CreatedAt: 10, QueuedAt: 10}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		status InstanceStatus
		start  int64
		finish int64
		valid  bool
	}{
		{"queued", Queued, 0, 0, true},
		{"queued started", Queued, 11, 0, false},
		{"queued finished", Queued, 0, 11, false},
		{"running", Running, 11, 0, true},
		{"running missing start", Running, 0, 0, false},
		{"running finished", Running, 11, 12, false},
		{"succeeded", Succeeded, 11, 12, true},
		{"failed", Failed, 11, 12, true},
		{"aborted", Aborted, 11, 12, true},
		{"terminal missing start", Failed, 0, 12, false},
		{"terminal missing finish", Succeeded, 11, 0, false},
		{"terminal reversed", Aborted, 12, 11, false},
		{"canceled queued", Canceled, 0, 11, true},
		{"canceled running", Canceled, 11, 12, true},
		{"canceled missing finish", Canceled, 11, 0, false},
		{"canceled start before queue", Canceled, 9, 12, false},
		{"canceled reversed", Canceled, 12, 11, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := base
			run.Status, run.StartedAt, run.FinishedAt = test.status, test.start, test.finish
			_, err := HydrateRun(run, snapshot)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v error=%v", test.valid, err)
			}
		})
	}
	for _, mutate := range []func(*Run){
		func(run *Run) { run.QueuedAt = 9 },
		func(run *Run) { run.Status, run.StartedAt = Running, 9 },
	} {
		run := base
		mutate(&run)
		if _, err := HydrateRun(run, snapshot); err == nil {
			t.Fatal("non-monotonic lifecycle accepted")
		}
	}
}

func TestRunStatusTransitionPermitsCancellationBeforeAndDuringExecution(t *testing.T) {
	for _, from := range []InstanceStatus{Queued, Running} {
		if err := ValidateRunStatusTransition(from, Canceled); err != nil {
			t.Fatalf("%s -> CANCELED: %v", from, err)
		}
	}
}

func TestRunTransitionAcceptsLifecycleProgression(t *testing.T) {
	snapshot, err := SealInstanceSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	initial, err := NewRun(Run{ID: "run-1", ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", Status: Queued, CreatedAt: 1, QueuedAt: 1}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	run, err := initial.Transition(Running, 2)
	if err != nil || run.Status != Running {
		t.Fatalf("unexpected transition: %+v, %v", run, err)
	}
	if _, err := run.Transition(Succeeded, 3); err != nil {
		t.Fatal(err)
	}
}

func TestRunTransitionRejectsReopeningTerminalRun(t *testing.T) {
	if _, err := (Run{Status: Succeeded}).Transition(Running, 2); err == nil {
		t.Fatal("expected terminal transition rejection")
	}
}

func TestRunTransitionRejectsAbortingQueuedRun(t *testing.T) {
	if _, err := (Run{Status: Queued}).Transition(Aborted, 2); err == nil {
		t.Fatal("expected queued abort rejection")
	}
}
