package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/node"
)

type runtimeIsolationNode struct {
	runtimes      []*node.Runtime
	overlayWasNil []bool
}

func (*runtimeIsolationNode) ID() string { return "isolation" }

func (n *runtimeIsolationNode) Run(_ context.Context, runtime *node.Runtime) error {
	n.runtimes = append(n.runtimes, runtime)
	n.overlayWasNil = append(n.overlayWasNil, runtime.SelectorOverlay == nil)
	runtime.Scratchpad["tenant"] = "mutated"
	runtime.SelectorOverlay = map[string][]fingerprint.Selector{"changed": nil}
	return nil
}

func TestRunProgramCreatesAnExecutionLocalRuntime(t *testing.T) {
	capture := &runtimeIsolationNode{}
	program := node.Program{Root: capture}
	config := Config{InstanceID: mustInstanceID("run"), Driver: &engineTestDriver{}}

	if _, err := runProgramForTest(context.Background(), compiledEntry(mustInstanceID("run"), program), config); err != nil {
		t.Fatal(err)
	}
	if _, err := runProgramForTest(context.Background(), compiledEntry(mustInstanceID("run"), program), config); err != nil {
		t.Fatal(err)
	}
	if len(capture.runtimes) != 2 || capture.runtimes[0] == capture.runtimes[1] {
		t.Fatalf("runtimes = %#v, want two independent values", capture.runtimes)
	}
	if len(capture.overlayWasNil) != 2 || !capture.overlayWasNil[0] || !capture.overlayWasNil[1] {
		t.Fatalf("selector overlay leaked between executions: %v", capture.overlayWasNil)
	}
	capture.runtimes[0].Scratchpad["only-first"] = "value"
	if _, exists := capture.runtimes[1].Scratchpad["only-first"]; exists {
		t.Fatal("execution scratchpads share one map")
	}
	if capture.runtimes[0].InstanceID != mustInstanceID("run") || capture.runtimes[0].Driver != config.Driver {
		t.Fatalf("runtime lost injected configuration: %#v", capture.runtimes[0])
	}
}

func TestRunProgramReturnsRecorderStopFailureAfterSuccessfulRoot(t *testing.T) {
	stopErr := errors.New("stop failed")
	recorder := &engineTestRecorder{stopErr: stopErr}
	_, err := runProgramForTest(context.Background(), compiledEntry(mustInstanceID("run"), node.Program{Root: &runtimeCaptureNode{}}), Config{
		InstanceID: mustInstanceID("run"), Driver: &engineTestDriver{}, Recorder: recorder,
	})
	if !errors.Is(err, stopErr) || !recorder.stopped || !recorder.retained {
		t.Fatalf("error=%v recorder=%+v", err, recorder)
	}
}
