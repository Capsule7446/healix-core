package contract_test

import (
	"context"
	"testing"

	coreengine "github.com/Capsule7446/healix-core/application/engine"
	appexecution "github.com/Capsule7446/healix-core/application/execution"
	"github.com/Capsule7446/healix-core/application/scheduling"
	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/interpolation"
	"github.com/Capsule7446/healix-core/domain/node"
	"github.com/Capsule7446/healix-core/domain/parameter"
	"github.com/Capsule7446/healix-core/domain/sampling"
)

type consumerResolver map[string]string

func (r consumerResolver) Variable(name string) (string, bool) {
	value, ok := r[name]
	return value, ok
}

type consumerDriver struct{}

type consumerAuthorityVerifier struct {
	want coreengine.ExecutionAuthority
}

func (v consumerAuthorityVerifier) VerifyExecutionAuthority(_ context.Context, authority coreengine.ExecutionAuthority) error {
	if authority != v.want {
		return execution.NewStaleWorkerFenceError()
	}
	return nil
}

func (consumerDriver) Navigate(context.Context, string) error { return nil }
func (consumerDriver) Press(context.Context, string) error    { return nil }
func (consumerDriver) Locate(context.Context, fingerprint.ElementTargetSpec) (node.Element, error) {
	return nil, node.NewElementNotFoundError()
}
func (consumerDriver) Snapshot(context.Context) (heal.DOMSnapshot, error) {
	return consumerSnapshot{}, nil
}
func (consumerDriver) WaitNetworkIdle(context.Context) error { return nil }

type consumerSnapshot struct{}

func (consumerSnapshot) Candidates(context.Context) ([]heal.SnapshotCandidate, error) {
	return nil, nil
}

type consumerCreateInstanceStore struct {
	resolved scheduling.ResolvedCreateInstance
	input    execution.InstanceSnapshotInput
	digest   string
}

func (s *consumerCreateInstanceStore) InTransaction(ctx context.Context, callback func(scheduling.CreateInstanceTx) error) error {
	return callback(s)
}
func (s *consumerCreateInstanceStore) FindCommand(context.Context, string) (scheduling.StoredCreateInstanceCommand, bool, error) {
	return scheduling.StoredCreateInstanceCommand{}, false, nil
}
func (s *consumerCreateInstanceStore) ResolveCreateInstance(context.Context, scheduling.CreateInstanceCommand) (scheduling.ResolvedCreateInstance, error) {
	return s.resolved, nil
}
func (s *consumerCreateInstanceStore) InsertCreateInstance(_ context.Context, intent scheduling.CreateInstanceIntent) (scheduling.InsertCreateInstanceOutcome, error) {
	s.input = intent.Snapshot.Input()
	s.digest = intent.Snapshot.Digest()
	hydrated, err := execution.HydrateInstanceSnapshot(s.input, s.digest)
	if err != nil {
		return scheduling.InsertCreateInstanceOutcome{}, err
	}
	entryIDs := make([]execution.EntryID, len(intent.Entries))
	for index, entry := range intent.Entries {
		entryIDs[index] = entry.ID
	}
	return scheduling.InsertCreateInstanceOutcome{Status: scheduling.InsertCreateInstanceApplied, CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: scheduling.StoredCreateInstanceResult{Run: intent.Run, Snapshot: hydrated, SnapshotDigest: hydrated.Digest(), EntryIDs: entryIDs}}, nil
}

// consumerEntryCompletionStore is a host-shaped EntryCompletionTransaction: it
// validates the intent before writing, records the receipt keyed by digest, and
// replays that receipt instead of applying a second time.
type consumerEntryCompletionStore struct {
	records map[string]appexecution.CompleteEntryOutcome
	applies int
}

func (s *consumerEntryCompletionStore) LookupEntryCompletion(_ context.Context, entryID execution.EntryID, requestDigest string) (appexecution.CompleteEntryOutcome, bool, error) {
	recorded, ok := s.records[entryID.String()+"\x00"+requestDigest]
	return recorded, ok, nil
}

func (s *consumerEntryCompletionStore) CompleteEntry(_ context.Context, intent appexecution.CompleteEntryIntent) (appexecution.CompleteEntryOutcome, error) {
	if err := appexecution.ValidateCompleteEntryIntentDigest(intent); err != nil {
		return appexecution.CompleteEntryOutcome{}, err
	}
	s.applies++
	applied := appexecution.CompleteEntryOutcome{Status: appexecution.CompleteEntryApplied, EntryID: intent.EntryID, RequestDigest: intent.RequestDigest, Decision: intent.Decision}
	if s.records == nil {
		s.records = map[string]appexecution.CompleteEntryOutcome{}
	}
	replayed := applied
	replayed.Status = appexecution.CompleteEntryReplayed
	s.records[intent.EntryID.String()+"\x00"+intent.RequestDigest] = replayed
	return applied, nil
}

func TestExternalConsumerCanImplementEntryCompletionPorts(t *testing.T) {
	store := &consumerEntryCompletionStore{}
	command := appexecution.CompleteEntryCommand{
		EntryID: mustEntryID("3:run4:item"),
		Fence:   execution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "claim"},
		State: appexecution.EntryCompletionState{
			EntryStatus:            execution.EntryRunning,
			TerminalIntent:         appexecution.TerminalIntentCancel,
			TerminalIntentRevision: 4,
			CancellationGeneration: 2,
		},
		Outcome:               appexecution.EngineOutcome{Result: coreengine.EntryResult{ExecutionOutcome: coreengine.OutcomeSucceeded, RecordingOutcome: coreengine.RecordingDisabled, TimelineOutcome: coreengine.TimelineDisabled}},
		AbortPendingCommandID: "abort-command",
	}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	digest, err := appexecution.CompleteEntryRequestDigest(command)
	if err != nil || digest == "" {
		t.Fatalf("external completion digest contract: digest=%q err=%v", digest, err)
	}
	decision, err := appexecution.DecideEntryCompletion(command.State, command.Outcome)
	if err != nil {
		t.Fatal(err)
	}
	// 裁决一：a finished engine outranks a pending cancel, and the intent still
	// travels intact into the counters the host writes verbatim.
	if decision.EntryStatus != execution.EntrySucceeded || decision.NextIntent != appexecution.TerminalIntentCancel ||
		decision.NextIntentRevision != 5 || decision.NextCancellationGeneration != 2 {
		t.Fatalf("external terminal decision contract: %+v", decision)
	}

	service := appexecution.NewEntryCompletionService(store)
	applied, err := service.Complete(context.Background(), command)
	if err != nil || applied.Status != appexecution.CompleteEntryApplied || applied.RequestDigest != digest || applied.Decision != decision {
		t.Fatalf("external complete contract: outcome=%+v err=%v", applied, err)
	}
	replayed, err := service.Complete(context.Background(), command)
	if err != nil || replayed.Status != appexecution.CompleteEntryReplayed || replayed.Decision != decision || store.applies != 1 {
		t.Fatalf("external replay contract: outcome=%+v applies=%d err=%v", replayed, store.applies, err)
	}
	if _, err := appexecution.NewEntryCompletionService(nil).Complete(context.Background(), command); !fault.IsCode(err, appexecution.CodeCompleteEntryUnavailable) {
		t.Fatalf("external unavailable contract: err=%v, want %s", err, appexecution.CodeCompleteEntryUnavailable)
	}
	forged := appexecution.CompleteEntryIntent{EntryID: command.EntryID, RequestDigest: digest, Command: command, Decision: appexecution.EntryCompletionDecision{}}
	if err := appexecution.ValidateCompleteEntryIntentDigest(forged); err == nil {
		t.Fatal("external intent contract: a host-invented decision was accepted")
	}
}

func TestExternalConsumerCanImplementCreateInstancePorts(t *testing.T) {
	workflow := automation.FlowFragmentDependencySnapshot{FlowFragment: automation.FlowFragment{ID: "workflow", DisplayName: "FlowFragment", CurrentVersionID: "workflow-v1", Properties: automation.Properties{}, CreatedAt: 1, UpdatedAt: 1}, Version: automation.FlowFragmentVersion{ID: "workflow-v1", FlowFragmentID: "workflow", VersionNumber: 1, Definition: automation.FlowFragmentContent{Steps: []automation.FlowFragmentStep{{ID: "wait", DisplayName: "Wait", Kind: automation.StepWait, WaitKind: "sleep", WaitMS: 1}}}, CreatedAt: 1}, ResolvedFromLatest: true}
	plan := automation.ResolvedExecutionFlow{Task: automation.ExecutionFlow{ID: "task", DisplayName: "Task", CurrentVersionID: "task-v1", CreatedAt: 1, UpdatedAt: 1}, Version: automation.ExecutionFlowVersion{ID: "task-v1", ExecutionFlowID: "task", VersionNumber: 1, FailurePolicy: automation.FailurePolicyStopOnFailure, CreatedAt: 1, Items: []automation.ExecutionFlowItem{{ID: "item", TestTaskVersionID: "task-v1", SequenceNumber: 1, FlowFragmentID: "workflow", VersionPolicy: automation.FlowFragmentVersionLatest}}}, Workflows: []automation.FlowFragmentDependencySnapshot{workflow}}
	path := "3:run4:item"
	store := &consumerCreateInstanceStore{resolved: scheduling.ResolvedCreateInstance{Plan: plan, Environment: automation.Environment{ID: "env", DisplayName: "Environment", BaseURL: "https://example.test", Revision: 1, Variables: automation.EnvironmentVariables{}}, Invocations: []execution.InvocationScopeSnapshot{{Path: mustInvocationPath(path), FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1", Values: map[string]parameter.Value{}}}}}
	command := scheduling.CreateInstanceCommand{CommandID: "command", InstanceID: mustInstanceID("run"), ExecutionFlowID: "task", TestTaskVersionID: "task-v1", EnvironmentID: "env", Entries: map[string]map[string]parameter.Value{"item": {}}, FailurePolicy: execution.FailurePolicyStopOnFailure, CreatedAt: 1, ScreenshotPolicy: execution.ScreenshotPolicySnapshot{Version: execution.ScreenshotPolicyV1, Enabled: true, Destination: "artifacts"}, HealerPolicy: execution.DefaultHealerPolicySnapshot()}
	service, err := scheduling.NewCreateInstanceService(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.CreateInstance(context.Background(), command)
	if err != nil || !result.WasApplied || store.digest == "" || store.input.InstanceID != mustInstanceID("run") {
		t.Fatalf("external CreateInstance contract: result=%#v digest=%q err=%v", result, store.digest, err)
	}
	snapshot, err := execution.HydrateInstanceSnapshot(store.input, store.digest)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := coreengine.CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := compiled.Entry(mustEntryID(path))
	if !ok {
		t.Fatalf("compiled execution %q is missing", path)
	}
	authority := coreengine.ExecutionAuthority{
		InstanceID: entry.InstanceID, SnapshotDigest: entry.SnapshotDigest, EntryID: entry.EntryID, ClaimToken: "claim",
	}
	runResult, err := coreengine.RunProgram(context.Background(), entry, coreengine.Config{
		InstanceID: entry.InstanceID, SnapshotDigest: entry.SnapshotDigest, EntryID: entry.EntryID,
		ClaimToken: "claim", AuthorityVerifier: consumerAuthorityVerifier{want: authority},
		Driver: consumerDriver{},
	})
	if err != nil || runResult.ExecutionOutcome != coreengine.OutcomeSucceeded {
		t.Fatalf("external compile/run contract: result=%+v err=%v", runResult, err)
	}
}

func TestPublicConsumerCanUseCoreContracts(t *testing.T) {
	value, err := interpolation.Expand("${tenant}", consumerResolver{"tenant": "north"})
	if err != nil || value != "north" {
		t.Fatalf("public interpolation contract = %q, %v", value, err)
	}
	_ = coreengine.Config{Driver: consumerDriver{}, Healer: heal.NewDefaultHealer()}
	_ = coreengine.CompiledPlan{}
	_ = sampling.MatchProfile{}
	_ = execution.PlanSnapshot{}
	_ = execution.Seal
	notStarted := appexecution.NotStartedEngineOutcome()
	if err := notStarted.Validate(); err != nil {
		t.Fatalf("public not-started outcome contract: %v", err)
	}
	if err := appexecution.TerminalIntentAbort.Validate(); err != nil {
		t.Fatalf("public terminal intent contract: %v", err)
	}
	state := appexecution.EntryCompletionState{EntryStatus: execution.EntryRunning, TerminalIntent: appexecution.TerminalIntentNone}
	if err := state.Validate(); err != nil {
		t.Fatalf("public completion state contract: %v", err)
	}
	if !fault.IsCode(appexecution.CompleteEntryUnavailableError(), appexecution.CodeCompleteEntryUnavailable) ||
		!fault.IsCode(appexecution.CompleteEntryIdentityConflictError(), appexecution.CodeCompleteEntryIdentityConflict) {
		t.Fatal("public completion fault constructors do not carry their published codes")
	}
}

var _ interpolation.Resolver = consumerResolver{}
var _ node.Driver = consumerDriver{}
var _ appexecution.EntryCompletionTransaction = (*consumerEntryCompletionStore)(nil)
