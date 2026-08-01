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
	if err != nil || runResult.ExecutionOutcome != coreengine.EntrySucceeded {
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
}

var _ interpolation.Resolver = consumerResolver{}
var _ node.Driver = consumerDriver{}
