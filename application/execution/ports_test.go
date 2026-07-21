package execution

import (
	"context"
	"testing"

	"github.com/Capsule7446/healix-core/domain/evidence"
)

type fakeCommitter struct{}

func (fakeCommitter) CommitStepTransition(context.Context, WorkerScope, evidence.StepTransitionCommit) (evidence.StepTransitionCommitResult, error) {
	return evidence.StepTransitionCommitResult{}, nil
}

func TestFactCommitterKeepsAtomicDomainCommitContract(t *testing.T) {
	var _ FactCommitter = fakeCommitter{}
}

type fakeProgressWriter struct{}

func (fakeProgressWriter) RecordStepProgress(context.Context, WorkerScope, evidence.StepProgressEvent) error {
	return nil
}
func (fakeProgressWriter) RecordValidationProgress(context.Context, WorkerScope, evidence.ValidationProgressObservation) error {
	return nil
}

func TestProgressWriterKeepsNonTerminalFactContract(t *testing.T) {
	var _ ProgressWriter = fakeProgressWriter{}
}
