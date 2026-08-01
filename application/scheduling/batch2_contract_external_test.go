package scheduling_test

import (
	"context"
	"testing"

	app "github.com/Capsule7446/healix-core/application/scheduling"
	domain "github.com/Capsule7446/healix-core/domain/execution"
)

func TestBatch2PublicCommandContractsCompile(t *testing.T) {
	_ = app.CancelRunCommand{CommandID: "cancel-1", RunID: mustInstanceID("run-1"), ExpectedStatus: domain.Queued, ExpectedRevision: 3, At: 10}
	_ = app.AbortRunCommand{CommandID: "abort-1", RunID: mustInstanceID("run-1"), ExpectedRevision: 3, Fence: domain.WorkerFence{RunID: mustInstanceID("run-1"), ClaimToken: "claim-1"}, At: 10}
	_ = app.ReorderQueueCommand{CommandID: "reorder-1", ScopeID: "scope-1", ExpectedRevision: 3, RunIDs: []string{"run-2", "run-1"}}
	var cancel *app.CancelRunService
	var abort *app.AbortRunService
	var reorder *app.ReorderQueueService
	_, _, _ = cancel, abort, reorder
	_, _ = app.CancelRunRequestDigest(app.CancelRunCommand{})
	_, _ = app.AbortRunRequestDigest(app.AbortRunCommand{})
	_, _ = app.ReorderQueueRequestDigest(app.ReorderQueueCommand{})
	_ = app.CodeInstanceSignalRetryable
	_ = app.CodeInstanceCommandIdentityConflict
	_ = app.CodeInstanceIdentityConflict
	_ = app.CodeInstanceRevisionConflict
	_ = app.CodeInstanceStatusConflict
	_ = app.CodeQueueRevisionConflict
	_ = app.CodeQueueMembershipConflict
	_ = app.CodeInstanceAdapterContractViolation
	_ = app.CodeCreateInstanceCommandInvalid
	_ = app.CodeCreateInstanceCommandConflict
	_ = app.CodeCreateInstanceSnapshotConflict
	_ = app.CodeCreateInstanceAdapterContractViolation
	_ = app.CodeCreateInstanceRetryable
	_ = app.CodeCreateInstanceCatalogGraphUnresolvable
	_ = app.CodeCancelInstanceCommandInvalid
	_ = app.CodeAbortInstanceCommandInvalid
	_ = app.CodeReorderQueueCommandInvalid
	_ = context.Background()
}
