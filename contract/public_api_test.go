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

func (consumerDriver) Navigate(context.Context, string) error { return nil }
func (consumerDriver) Press(context.Context, string) error    { return nil }
func (consumerDriver) Locate(context.Context, fingerprint.NodeSpec) (node.Element, error) {
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
	workflow := automation.WorkflowDependencySnapshot{Workflow: automation.Workflow{ID: "workflow", DisplayName: "Workflow", CurrentVersionID: "workflow-v1", Properties: automation.Properties{}, CreatedAt: 1, UpdatedAt: 1}, Version: automation.WorkflowVersion{ID: "workflow-v1", WorkflowID: "workflow", VersionNumber: 1, Definition: automation.WorkflowDefinition{Steps: []automation.WorkflowStep{{ID: "wait", DisplayName: "Wait", Kind: automation.StepWait, WaitKind: "sleep", WaitMS: 1}}}, CreatedAt: 1}, ResolvedFromLatest: true}
	plan := automation.TestTaskVersionPlan{Task: automation.TestTask{ID: "task", DisplayName: "Task", CurrentVersionID: "task-v1", CreatedAt: 1, UpdatedAt: 1}, Version: automation.TestTaskVersion{ID: "task-v1", TestTaskID: "task", VersionNumber: 1, FailurePolicy: automation.FailurePolicyStopOnFailure, CreatedAt: 1, Items: []automation.TestTaskItem{{ID: "item", TestTaskVersionID: "task-v1", SequenceNumber: 1, WorkflowID: "workflow", VersionPolicy: automation.WorkflowVersionLatest}}}, Workflows: []automation.WorkflowDependencySnapshot{workflow}}
	path := "3:run4:item"
	store := &consumerCreateRunStore{resolved: scheduling.ResolvedCreateRun{Plan: plan, Environment: automation.Environment{ID: "env", DisplayName: "Environment", BaseURL: "https://example.test", Revision: 1, Properties: automation.Properties{}}, Invocations: []execution.InvocationScopeSnapshot{{Path: path, WorkflowID: "workflow", WorkflowVersionID: "workflow-v1", Values: map[string]parameter.Value{}}}}}
	command := scheduling.CreateRunCommand{CommandID: "command", RunID: "run", TestTaskID: "task", TestTaskVersionID: "task-v1", EnvironmentID: "env", Entries: map[string]map[string]parameter.Value{"item": {}}, FailurePolicy: execution.FailurePolicyStopOnFailure, CreatedAt: 1, ScreenshotPolicy: execution.ScreenshotPolicySnapshot{Version: execution.ScreenshotPolicyV1, Enabled: true, Destination: "artifacts"}, HealerPolicy: execution.DefaultHealerPolicySnapshot()}
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
	runResult, err := coreengine.RunProgram(context.Background(), entry, coreengine.Config{
		RunID: entry.RunID, SnapshotDigest: entry.SnapshotDigest, ExecutionID: entry.ExecutionID,
		ClaimToken: "claim",
		Driver:     consumerDriver{},
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
