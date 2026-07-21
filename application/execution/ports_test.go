package execution

import (
	"context"
	"testing"

	"github.com/Capsule7446/healix-core/domain/workspace"
)

type fakeCommitter struct{}

func (fakeCommitter) CommitStepTransition(context.Context, workspace.StepTransitionCommit) (workspace.StepTransitionCommitResult, error) {
	return workspace.StepTransitionCommitResult{}, nil
}

func TestFactCommitterKeepsAtomicDomainCommitContract(t *testing.T) {
	var _ FactCommitter = fakeCommitter{}
}

type fakeProgressWriter struct{}

func (fakeProgressWriter) RecordStepProgress(context.Context, workspace.StepPhaseEvent) error {
	return nil
}
func (fakeProgressWriter) RecordValidationProgress(context.Context, workspace.ValidationObservation) error {
	return nil
}
func (fakeProgressWriter) AttachTerminalStepError(context.Context, workspace.StepPhaseEvent) error {
	return nil
}

func TestProgressWriterKeepsNonTerminalFactContract(t *testing.T) {
	var _ ProgressWriter = fakeProgressWriter{}
}
