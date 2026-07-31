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
	var commandConflict *app.CommandConflictError
	var identityConflict *app.RunIdentityConflictError
	var revisionConflict *app.RunRevisionConflictError
	var statusConflict *app.RunStatusConflictError
	var queueRevisionConflict *app.QueueRevisionConflictError
	var queueMembershipConflict *app.QueueMembershipConflictError
	var adapterContract *app.RunAdapterContractError
	_, _, _, _, _, _, _ = commandConflict, identityConflict, revisionConflict, statusConflict, queueRevisionConflict, queueMembershipConflict, adapterContract
	_ = app.CodeRunSignalRetryable
	_ = context.Background()
}
