package evidence

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
)

func validProgressEvent(phase ProgressPhase) StepProgressEvent {
	return StepProgressEvent{ID: mustStepExecutionID("step"), ExecutionID: mustEntryID("execution"), WorkflowStepID: "workflow-step", DisplayName: "Step", Kind: "action", Phase: phase, Occurrence: 1, Timestamp: 1}
}

func TestStepProgressEventAcceptsOnlyRuntimeNonTerminalPhases(t *testing.T) {
	for _, phase := range []ProgressPhase{ProgressRunning, ProgressHealing, ProgressTransitioning, ProgressValidating} {
		if err := validProgressEvent(phase).Validate(); err != nil {
			t.Fatalf("phase %q rejected: %v", phase, err)
		}
	}
	for _, phase := range []ProgressPhase{"", "PENDING", "SUCCEEDED", "FAILED", "CANCELED", "ABORTED"} {
		if err := validProgressEvent(phase).Validate(); err == nil {
			t.Fatalf("phase %q accepted", phase)
		}
	}
}

func TestStepProgressEventRejectsMissingIdentityAndRuntimeCoordinates(t *testing.T) {
	missingIdentity := validProgressEvent(ProgressRunning)
	missingIdentity.ID = execution.StepExecutionID{}
	if err := missingIdentity.Validate(); err == nil {
		t.Fatal("missing identity accepted")
	}
	invalidOccurrence := validProgressEvent(ProgressRunning)
	invalidOccurrence.Occurrence = 0
	if err := invalidOccurrence.Validate(); err == nil {
		t.Fatal("invalid occurrence accepted")
	}
}
