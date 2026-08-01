package scheduling_test

import (
	"context"
	"testing"

	app "github.com/Capsule7446/healix-core/application/scheduling"
	domain "github.com/Capsule7446/healix-core/domain/execution"
)

func TestBatch2PublicCommandContractsCompile(t *testing.T) {
	_ = app.CancelInstanceCommand{CommandID: "cancel-1", InstanceID: mustInstanceID("run-1"), ExpectedStatus: domain.Queued, ExpectedRevision: 3, At: 10}
	_ = app.AbortInstanceCommand{CommandID: "abort-1", InstanceID: mustInstanceID("run-1"), ExpectedRevision: 3, Fence: domain.WorkerFence{InstanceID: mustInstanceID("run-1"), ClaimToken: "claim-1"}, At: 10}
	_ = app.ReorderQueueCommand{CommandID: "reorder-1", ScopeID: "scope-1", ExpectedRevision: 3, InstanceIDs: []string{"run-2", "run-1"}}
	var cancel *app.CancelInstanceService
	var abort *app.AbortInstanceService
	var reorder *app.ReorderQueueService
	_, _, _ = cancel, abort, reorder
	_, _ = app.CancelInstanceRequestDigest(app.CancelInstanceCommand{})
	_, _ = app.AbortInstanceRequestDigest(app.AbortInstanceCommand{})
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
