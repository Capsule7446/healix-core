package contract_test

import (
	"context"
	"testing"

	coreengine "github.com/Capsule7446/healix-core/application/engine"
	"github.com/Capsule7446/healix-core/application/scheduling"
	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
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
		return execution.ErrStaleWorkerFence
	}
	return nil
}

func (consumerDriver) Navigate(context.Context, string) error { return nil }
func (consumerDriver) Press(context.Context, string) error    { return nil }
func (consumerDriver) Locate(context.Context, fingerprint.ElementTargetSpec) (node.Element, error) {
	return nil, node.ErrElementNotFound
}
func (consumerDriver) Snapshot(context.Context) (heal.DOMSnapshot, error) {
	return consumerSnapshot{}, nil
}
func (consumerDriver) WaitNetworkIdle(context.Context) error { return nil }

type consumerSnapshot struct{}

func (consumerSnapshot) Candidates(context.Context) ([]heal.SnapshotCandidate, error) {
	return nil, nil
}

type consumerCreateRunStore struct {
	resolved scheduling.ResolvedCreateRun
	input    execution.RunSnapshotInput
	digest   string
}

func (s *consumerCreateRunStore) InTransaction(ctx context.Context, callback func(scheduling.CreateRunTx) error) error {
	return callback(s)
}
func (s *consumerCreateRunStore) FindCommand(context.Context, string) (scheduling.StoredCreateRunCommand, bool, error) {
	return scheduling.StoredCreateRunCommand{}, false, nil
}
func (s *consumerCreateRunStore) ResolveCreateRun(context.Context, scheduling.CreateRunCommand) (scheduling.ResolvedCreateRun, error) {
	return s.resolved, nil
}
func (s *consumerCreateRunStore) InsertCreateRun(_ context.Context, intent scheduling.CreateRunIntent) (scheduling.InsertCreateRunOutcome, error) {
	s.input = intent.Snapshot.Input()
	s.digest = intent.Snapshot.Digest()
	hydrated, err := execution.HydrateRunSnapshot(s.input, s.digest)
	if err != nil {
		return scheduling.InsertCreateRunOutcome{}, err
	}
	entryIDs := make([]string, len(intent.Entries))
	for index, entry := range intent.Entries {
		entryIDs[index] = entry.ExecutionID
	}
	return scheduling.InsertCreateRunOutcome{Status: scheduling.InsertCreateRunApplied, CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: scheduling.StoredCreateRunResult{Run: intent.Run, Snapshot: hydrated, SnapshotDigest: hydrated.Digest(), EntryIDs: entryIDs}}, nil
}

func TestExternalConsumerCanImplementCreateRunPorts(t *testing.T) {
	workflow := automation.FlowFragmentDependencySnapshot{FlowFragment: automation.FlowFragment{ID: "workflow", DisplayName: "FlowFragment", CurrentVersionID: "workflow-v1", Properties: automation.Properties{}, CreatedAt: 1, UpdatedAt: 1}, Version: automation.FlowFragmentVersion{ID: "workflow-v1", FlowFragmentID: "workflow", VersionNumber: 1, Definition: automation.FlowFragmentContent{Steps: []automation.FlowFragmentStep{{ID: "wait", DisplayName: "Wait", Kind: automation.StepWait, WaitKind: "sleep", WaitMS: 1}}}, CreatedAt: 1}, ResolvedFromLatest: true}
	plan := automation.ResolvedExecutionFlow{Task: automation.ExecutionFlow{ID: "task", DisplayName: "Task", CurrentVersionID: "task-v1", CreatedAt: 1, UpdatedAt: 1}, Version: automation.ExecutionFlowVersion{ID: "task-v1", ExecutionFlowID: "task", VersionNumber: 1, FailurePolicy: automation.FailurePolicyStopOnFailure, CreatedAt: 1, Items: []automation.ExecutionFlowItem{{ID: "item", TestTaskVersionID: "task-v1", SequenceNumber: 1, FlowFragmentID: "workflow", VersionPolicy: automation.FlowFragmentVersionLatest}}}, Workflows: []automation.FlowFragmentDependencySnapshot{workflow}}
	path := "3:run4:item"
	store := &consumerCreateRunStore{resolved: scheduling.ResolvedCreateRun{Plan: plan, Environment: automation.Environment{ID: "env", DisplayName: "Environment", BaseURL: "https://example.test", Revision: 1, Variables: automation.EnvironmentVariables{}}, Invocations: []execution.InvocationScopeSnapshot{{Path: path, FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1", Values: map[string]parameter.Value{}}}}}
	command := scheduling.CreateRunCommand{CommandID: "command", RunID: "run", ExecutionFlowID: "task", TestTaskVersionID: "task-v1", EnvironmentID: "env", Entries: map[string]map[string]parameter.Value{"item": {}}, FailurePolicy: execution.FailurePolicyStopOnFailure, CreatedAt: 1, ScreenshotPolicy: execution.ScreenshotPolicySnapshot{Version: execution.ScreenshotPolicyV1, Enabled: true, Destination: "artifacts"}, HealerPolicy: execution.DefaultHealerPolicySnapshot()}
	service, err := scheduling.NewCreateRunService(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.CreateRun(context.Background(), command)
	if err != nil || !result.WasApplied || store.digest == "" || store.input.RunID != "run" {
		t.Fatalf("external CreateRun contract: result=%#v digest=%q err=%v", result, store.digest, err)
	}
	snapshot, err := execution.HydrateRunSnapshot(store.input, store.digest)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := coreengine.CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := compiled.Entry(path)
	if !ok {
		t.Fatalf("compiled execution %q is missing", path)
	}
	authority := coreengine.ExecutionAuthority{
		RunID: entry.RunID, SnapshotDigest: entry.SnapshotDigest, ExecutionID: entry.ExecutionID, ClaimToken: "claim",
	}
	runResult, err := coreengine.RunProgram(context.Background(), entry, coreengine.Config{
		RunID: entry.RunID, SnapshotDigest: entry.SnapshotDigest, ExecutionID: entry.ExecutionID,
		ClaimToken: "claim", AuthorityVerifier: consumerAuthorityVerifier{want: authority},
		Driver: consumerDriver{},
	})
	if err != nil || runResult.ExecutionOutcome != coreengine.ExecutionSucceeded {
		t.Fatalf("external compile/run contract: result=%+v err=%v", runResult, err)
	}
}

func TestPublicConsumerCanUseCoreContracts(t *testing.T) {
	value, err := interpolation.Expand("${tenant}", consumerResolver{"tenant": "north"})
	if err != nil || value != "north" {
		t.Fatalf("public interpolation contract = %q, %v", value, err)
	}
	_ = coreengine.Config{Driver: consumerDriver{}, Healer: heal.NewDefaultHealer()}
	_ = coreengine.CompiledRun{}
	_ = sampling.MatchProfile{}
	_ = execution.Draft{}
	_ = execution.Seal
}

var _ interpolation.Resolver = consumerResolver{}
var _ node.Driver = consumerDriver{}
