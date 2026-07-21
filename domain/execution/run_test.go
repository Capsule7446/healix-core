package execution

import "testing"

func TestRunTransitionAcceptsLifecycleProgression(t *testing.T) {
	run, err := (Run{ID: "run", Status: Queued}).Transition(Running)
	if err != nil || run.Status != Running {
		t.Fatalf("unexpected transition: %+v, %v", run, err)
	}
	if _, err := run.Transition(Succeeded); err != nil {
		t.Fatal(err)
	}
}

func TestRunTransitionRejectsReopeningTerminalRun(t *testing.T) {
	if _, err := (Run{Status: Succeeded}).Transition(Running); err == nil {
		t.Fatal("expected terminal transition rejection")
	}
}

func TestRunTransitionRejectsAbortingQueuedRun(t *testing.T) {
	if _, err := (Run{Status: Queued}).Transition(Aborted); err == nil {
		t.Fatal("expected queued abort rejection")
	}
}
