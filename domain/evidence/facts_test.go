package evidence

import "testing"

func TestStepFactRequiresTerminalIdentity(t *testing.T) {
	fact := StepFact{ID: "fact", RunID: mustInstanceID("run"), ExecutionID: mustEntryID("execution"), StepExecution: mustStepExecutionID("step"), Phase: PhaseSucceeded, ObservedAt: 1}
	if err := fact.Validate(); err != nil {
		t.Fatal(err)
	}
	fact.Phase = "RUNNING"
	if err := fact.Validate(); err == nil {
		t.Fatal("expected non-terminal fact rejection")
	}
}
