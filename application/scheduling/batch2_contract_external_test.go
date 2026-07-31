package scheduling_test

import (
	"context"
	"testing"

	app "github.com/Capsule7446/healix-core/application/scheduling"
	domain "github.com/Capsule7446/healix-core/domain/execution"
)

func TestBatch2PublicCommandContractsCompile(t *testing.T) {
	_ = app.CancelRunCommand{CommandID: "cancel-1", RunID: "run-1", ExpectedStatus: domain.Queued, ExpectedRevision: 3, At: 10}
	_ = app.AbortRunCommand{CommandID: "abort-1", RunID: "run-1", ExpectedRevision: 3, Fence: domain.WorkerFence{RunID: "run-1", ClaimToken: "claim-1"}, At: 10}
	_ = app.ReorderQueueCommand{CommandID: "reorder-1", ScopeID: "scope-1", ExpectedRevision: 3, RunIDs: []string{"run-2", "run-1"}}
	var cancel *app.CancelRunService
	var abort *app.AbortRunService
	var reorder *app.ReorderQueueService
	_, _, _ = cancel, abort, reorder
	_, _ = app.CancelRunRequestDigest(app.CancelRunCommand{})
	_, _ = app.AbortRunRequestDigest(app.AbortRunCommand{})
	_, _ = app.ReorderQueueRequestDigest(app.ReorderQueueCommand{})
	_ = app.CodeRunSignalRetryable
	_ = app.CodeRunCommandIdentityConflict
	_ = app.CodeRunIdentityConflict
	_ = app.CodeRunRevisionConflict
	_ = app.CodeRunStatusConflict
	_ = app.CodeQueueRevisionConflict
	_ = app.CodeQueueMembershipConflict
	_ = app.CodeRunAdapterContractViolation
	_ = app.CodeCreateRunCommandInvalid
	_ = app.CodeCreateRunCommandConflict
	_ = app.CodeCreateRunSnapshotConflict
	_ = app.CodeCreateRunAdapterContractViolation
	_ = app.CodeCreateRunRetryable
	_ = context.Background()
}
