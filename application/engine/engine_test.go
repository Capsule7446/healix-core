package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/node"
)

type engineTestDriver struct {
	navigated string
}

type runtimeCaptureNode struct {
	interval time.Duration
	runs     int
	err      error
}

type noopExecutionSink struct{}

func (noopExecutionSink) RecordProgress(context.Context, domainexecution.WorkerFence, node.Event) error {
	return nil
}
func (noopExecutionSink) StageHealDecision(context.Context, domainexecution.WorkerFence, string, string, fingerprint.Selector, heal.Decision) error {
	return nil
}
func (noopExecutionSink) StageValidationObservation(context.Context, domainexecution.WorkerFence, node.ValidationObservation) error {
	return nil
}
func (noopExecutionSink) StageValidationGroupTerminal(context.Context, domainexecution.WorkerFence, node.ValidationGroupTerminalObservation) error {
	return nil
}
func (noopExecutionSink) CommitTerminal(context.Context, domainexecution.WorkerFence, node.TerminalCommit) error {
	return nil
}

func (*runtimeCaptureNode) ID() string { return "capture-runtime" }

func (n *runtimeCaptureNode) Run(_ context.Context, runtime *node.Runtime) error {
	n.runs++
	n.interval = runtime.StepInterval
	return n.err
}

func TestRunCompiledEntryPropagatesStepIntervalToRuntime(t *testing.T) {
	capture := &runtimeCaptureNode{}
	err := RunCompiledEntry(context.Background(), CompiledEntry{Program: node.Program{Root: capture}}, Config{
		RunID: "run-paced", Driver: &engineTestDriver{}, StepInterval: 750 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if capture.interval != 750*time.Millisecond {
		t.Fatalf("runtime StepInterval = %s, want 750ms", capture.interval)
	}
}

func (d *engineTestDriver) Navigate(_ context.Context, url string) error {
	d.navigated = url
	return nil
}
func (*engineTestDriver) Press(context.Context, string) error { return nil }
func (*engineTestDriver) Locate(context.Context, fingerprint.NodeSpec) (node.Element, error) {
	return nil, errors.New("unexpected Locate")
}
func (*engineTestDriver) Snapshot(context.Context) (heal.DOMSnapshot, error) {
	return nil, errors.New("unexpected Snapshot")
}
func (*engineTestDriver) WaitNetworkIdle(context.Context) error { return nil }

type engineTestTimeline struct {
	sequence uint64
}

func (t *engineTestTimeline) Mark() node.TimelineMark {
	t.sequence++
	return node.TimelineMark{Sequence: t.sequence}
}

type engineTestRecorder struct {
	startedRunID string
	stopped      bool
	retained     bool
	startErr     error
	stopErr      error
	stopCtxErr   error
	nilTimeline  bool
}

func (r *engineTestRecorder) Start(_ context.Context, runID string) (node.RecordingTimeline, error) {
	r.startedRunID = runID
	if r.startErr != nil {
		return nil, r.startErr
	}
	if r.nilTimeline {
		return nil, nil
	}
	return &engineTestTimeline{}, nil
}

func (r *engineTestRecorder) Stop(ctx context.Context, retain bool) error {
	r.stopped = true
	r.retained = retain
	r.stopCtxErr = ctx.Err()
	return r.stopErr
}

func TestRunCompiledEntryRetainsSuccessfulRecording(t *testing.T) {
	recorder := &engineTestRecorder{}
	err := RunCompiledEntry(context.Background(), navigationCompiledEntry("retain-success", "https://example.test"), Config{
		RunID:    "run-retain-success",
		Driver:   &engineTestDriver{},
		Recorder: recorder,
	})
	if err != nil {
		t.Fatalf("RunCompiledEntry: %v", err)
	}
	if recorder.startedRunID != "run-retain-success" || !recorder.stopped || !recorder.retained {
		t.Fatalf("recorder lifecycle = %+v, want successful run retained", recorder)
	}
}

func TestRunCompiledEntryRejectsIncompleteConfigurationBeforeExecution(t *testing.T) {
	root := &runtimeCaptureNode{}
	tests := []struct {
		name    string
		program node.Program
		config  Config
	}{
		{"missing run id", node.Program{Root: root}, Config{Driver: &engineTestDriver{}}},
		{"missing claim token with facts", node.Program{Root: root}, Config{RunID: "run", Driver: &engineTestDriver{}, Facts: noopExecutionSink{}}},
		{"missing driver", node.Program{Root: root}, Config{RunID: "run"}},
		{"missing root", node.Program{}, Config{RunID: "run", Driver: &engineTestDriver{}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := RunCompiledEntry(context.Background(), compiledEntry(test.program), test.config); err == nil {
				t.Fatal("incomplete engine configuration was accepted")
			}
		})
	}
	if root.runs != 0 {
		t.Fatalf("root executed %d times during configuration rejection", root.runs)
	}
}

func TestRunCompiledEntryRecorderFailureAndDetachedCleanupContract(t *testing.T) {
	root := &runtimeCaptureNode{}
	startFailure := &engineTestRecorder{startErr: errors.New("start failed")}
	if err := RunCompiledEntry(context.Background(), compiledEntry(node.Program{Root: root}), Config{
		RunID: "run-start-failure", Driver: &engineTestDriver{}, Recorder: startFailure,
	}); err == nil || !strings.Contains(err.Error(), "start recorder") {
		t.Fatalf("recorder start error = %v", err)
	}
	if root.runs != 0 || startFailure.stopped {
		t.Fatalf("start failure executed root/stopped recorder: root=%d recorder=%+v", root.runs, startFailure)
	}

	rootFailure := errors.New("root failed")
	stopFailure := errors.New("stop failed")
	root = &runtimeCaptureNode{err: rootFailure}
	recorder := &engineTestRecorder{stopErr: stopFailure}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RunCompiledEntry(ctx, compiledEntry(node.Program{Root: root}), Config{
		RunID: "run-cleanup", Driver: &engineTestDriver{}, Recorder: recorder,
	})
	if !errors.Is(err, rootFailure) || !errors.Is(err, stopFailure) {
		t.Fatalf("combined run/stop error = %v", err)
	}
	if root.runs != 1 || !recorder.stopped || !recorder.retained || recorder.stopCtxErr != nil {
		t.Fatalf("detached recorder cleanup = root %d recorder %+v", root.runs, recorder)
	}
}

func compiledEntry(program node.Program) CompiledEntry {
	return CompiledEntry{Program: program}
}

func navigationCompiledEntry(id, url string) CompiledEntry {
	return CompiledEntry{Program: node.Program{Root: &node.WorkflowNode{NodeID: id,
		Children: []node.Node{&node.StepNode{NodeID: "open", Action: node.Action{Kind: node.ActionNavigate, Value: url}}}}}}
}
