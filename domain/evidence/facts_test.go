package evidence

import "testing"

func TestStepFactRequiresTerminalIdentity(t *testing.T) {
	fact := StepFact{ID: "fact", InstanceID: mustInstanceID("run"), EntryID: mustEntryID("execution"), StepExecutionID: mustStepExecutionID("step"), Occurrence: 1, Phase: PhaseSucceeded, ObservedAt: 1}
	if err := fact.Validate(); err != nil {
		t.Fatal(err)
	}
	fact.Phase = "RUNNING"
	if err := fact.Validate(); err == nil {
		t.Fatal("expected non-terminal fact rejection")
	}
}
