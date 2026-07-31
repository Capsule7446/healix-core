package engine

import (
	"context"
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/node"
)

type executionIdentityProbe struct {
	runtimeCalls   int
	driverCalls    int
	recorderCalls  int
	factCalls      int
	authorityCalls int
	authority      ExecutionAuthority
	authorityErr   error
}

func (probe *executionIdentityProbe) VerifyExecutionAuthority(_ context.Context, authority ExecutionAuthority) error {
	probe.authorityCalls++
	probe.authority = authority
	return probe.authorityErr
}

type executionIdentityProbeNode struct {
	probe *executionIdentityProbe
}

func (*executionIdentityProbeNode) ID() string { return "identity-probe" }

func (n *executionIdentityProbeNode) Run(ctx context.Context, runtime *node.Runtime) error {
	n.probe.runtimeCalls++
	if err := runtime.Driver.Navigate(ctx, "https://identity-mismatch.invalid"); err != nil {
		return err
	}
	return runtime.Facts.RecordProgress(ctx,
		domainexecution.WorkerFence{RunID: runtime.RunID, ClaimToken: runtime.ClaimToken},
		node.Event{RunID: runtime.RunID, NodeID: n.ID(), Occurrence: 1, Phase: node.PhaseRunning},
	)
}

type executionIdentityProbeDriver struct {
	probe *executionIdentityProbe
}

func (d executionIdentityProbeDriver) Navigate(context.Context, string) error {
	d.probe.driverCalls++
	return nil
}
func (executionIdentityProbeDriver) Press(context.Context, string) error { return nil }
func (executionIdentityProbeDriver) Locate(context.Context, fingerprint.ElementTargetSpec) (node.Element, error) {
	return nil, node.NewElementNotFoundError()
}
func (executionIdentityProbeDriver) Snapshot(context.Context) (heal.DOMSnapshot, error) {
	return nil, nil
}
func (executionIdentityProbeDriver) WaitNetworkIdle(context.Context) error { return nil }

type executionIdentityProbeRecorder struct {
	probe *executionIdentityProbe
}

func (r executionIdentityProbeRecorder) Start(context.Context, string) (node.RecordingTimeline, error) {
	r.probe.recorderCalls++
	return &engineTestTimeline{}, nil
}
func (r executionIdentityProbeRecorder) Stop(context.Context, bool) error {
	r.probe.recorderCalls++
	return nil
}

type executionIdentityProbeFacts struct {
	probe *executionIdentityProbe
}

func (f executionIdentityProbeFacts) RecordProgress(context.Context, domainexecution.WorkerFence, node.Event) error {
	f.probe.factCalls++
	return nil
}
func (f executionIdentityProbeFacts) StageHealDecision(context.Context, domainexecution.WorkerFence, string, string, fingerprint.Selector, heal.Decision) error {
	f.probe.factCalls++
	return nil
}
func (f executionIdentityProbeFacts) StageValidationObservation(context.Context, domainexecution.WorkerFence, node.ValidationObservation) error {
	f.probe.factCalls++
	return nil
}
func (f executionIdentityProbeFacts) StageValidationGroupTerminal(context.Context, domainexecution.WorkerFence, node.ValidationGroupTerminalObservation) error {
	f.probe.factCalls++
	return nil
}
func (f executionIdentityProbeFacts) CommitTerminal(context.Context, domainexecution.WorkerFence, node.TerminalCommit) error {
	f.probe.factCalls++
	return nil
}

func TestRunProgramRejectsExecutionIdentityMismatchWithoutSideEffects(t *testing.T) {
	snapshot, err := runSnapshotForCompilerTest(minimalCompilerPlan(), map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*CompiledEntry, *Config)
	}{
		{name: "config run", mutate: func(_ *CompiledEntry, cfg *Config) { cfg.RunID = "wrong-run" }},
		{name: "config snapshot", mutate: func(_ *CompiledEntry, cfg *Config) { cfg.SnapshotDigest = "wrong-digest" }},
		{name: "config execution", mutate: func(_ *CompiledEntry, cfg *Config) { cfg.ExecutionID = "wrong-execution" }},
		{name: "missing claim token without facts", mutate: func(_ *CompiledEntry, cfg *Config) {
			cfg.ClaimToken = ""
			cfg.Facts = nil
		}},
		{name: "entry run", mutate: func(entry *CompiledEntry, _ *Config) { entry.RunID = "wrong-run" }},
		{name: "entry snapshot", mutate: func(entry *CompiledEntry, _ *Config) { entry.SnapshotDigest = "wrong-digest" }},
		{name: "entry execution", mutate: func(entry *CompiledEntry, _ *Config) { entry.ExecutionID = "wrong-execution" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry, ok := compiled.Entry("execution-entry")
			if !ok {
				t.Fatal("execution-entry is missing")
			}
			probe := &executionIdentityProbe{}
			entry.program.Root = &executionIdentityProbeNode{probe: probe}
			cfg := Config{
				RunID:             entry.RunID,
				SnapshotDigest:    entry.SnapshotDigest,
				ExecutionID:       entry.ExecutionID,
				ClaimToken:        "claim",
				AuthorityVerifier: probe,
				Driver:            executionIdentityProbeDriver{probe: probe},
				Recorder:          executionIdentityProbeRecorder{probe: probe},
				Facts:             executionIdentityProbeFacts{probe: probe},
			}
			test.mutate(&entry, &cfg)

			result, err := RunProgram(context.Background(), entry, cfg)
			if !fault.IsCode(err, CodeExecutionIdentityMismatch) {
				t.Fatalf("RunProgram() error = %v, want stable execution identity mismatch; side effects = %+v", err, probe)
			}
			if result.ExecutionOutcome != ExecutionNotStarted {
				t.Fatalf("execution outcome = %q, want %q", result.ExecutionOutcome, ExecutionNotStarted)
			}
			if *probe != (executionIdentityProbe{}) {
				t.Fatalf("identity mismatch produced side effects: %+v", probe)
			}
		})
	}
}

func TestRunProgramRequiresCurrentExecutionAuthorityBeforeSideEffects(t *testing.T) {
	snapshot, err := runSnapshotForCompilerTest(minimalCompilerPlan(), map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := compiled.Entry("execution-entry")
	if !ok {
		t.Fatal("execution-entry is missing")
	}
	stale := domainexecution.NewStaleWorkerFenceError()
	tests := []struct {
		name     string
		verifier ExecutionAuthorityVerifier
		wantCode fault.Code
	}{
		{name: "missing verifier"},
		{name: "stale authority", verifier: &executionIdentityProbe{authorityErr: stale}, wantCode: domainexecution.CodeWorkerFenceStale},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := &executionIdentityProbe{}
			entryCopy := entry
			entryCopy.program.Root = &executionIdentityProbeNode{probe: probe}
			cfg := Config{RunID: entry.RunID, SnapshotDigest: entry.SnapshotDigest, ExecutionID: entry.ExecutionID,
				ClaimToken: "claim", AuthorityVerifier: test.verifier, Driver: executionIdentityProbeDriver{probe: probe},
				Recorder: executionIdentityProbeRecorder{probe: probe}, Facts: executionIdentityProbeFacts{probe: probe}}
			result, err := RunProgram(context.Background(), entryCopy, cfg)
			if test.wantCode != "" {
				if !fault.IsCode(err, test.wantCode) {
					t.Fatalf("RunProgram() error = %v, want code %v", err, test.wantCode)
				}
			} else if !fault.IsCode(err, CodeExecutionAuthorityVerifierRequired) {
				t.Fatalf("RunProgram() error = %v, want %v", err, ExecutionAuthorityVerifierRequiredError())
			}
			if result.ExecutionOutcome != ExecutionNotStarted || probe.runtimeCalls != 0 || probe.driverCalls != 0 || probe.recorderCalls != 0 || probe.factCalls != 0 {
				t.Fatalf("authority rejection produced side effects: result=%+v probe=%+v", result, probe)
			}
		})
	}
}

func TestRunProgramForwardsCompleteExecutionAuthority(t *testing.T) {
	snapshot, err := runSnapshotForCompilerTest(minimalCompilerPlan(), map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := compiled.Entry("execution-entry")
	if !ok {
		t.Fatal("execution-entry is missing")
	}
	probe := &executionIdentityProbe{}
	entry.program.Root = &executionIdentityProbeNode{probe: probe}
	cfg := Config{RunID: entry.RunID, SnapshotDigest: entry.SnapshotDigest, ExecutionID: entry.ExecutionID,
		ClaimToken: "claim", AuthorityVerifier: probe, Driver: executionIdentityProbeDriver{probe: probe}, Facts: executionIdentityProbeFacts{probe: probe}}
	if _, err := RunProgram(context.Background(), entry, cfg); err != nil {
		t.Fatal(err)
	}
	want := ExecutionAuthority{RunID: entry.RunID, SnapshotDigest: entry.SnapshotDigest, ExecutionID: entry.ExecutionID, ClaimToken: "claim"}
	if probe.authorityCalls != 1 || probe.authority != want {
		t.Fatalf("authority calls = %d, authority = %+v, want %+v", probe.authorityCalls, probe.authority, want)
	}
}
