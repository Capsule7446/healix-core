package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
)

type pacingDriver struct {
	navigations int
}

func (d *pacingDriver) Navigate(context.Context, string) error {
	d.navigations++
	return nil
}

func (d *pacingDriver) Press(context.Context, string) error { return nil }
func (d *pacingDriver) Locate(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
	return testElement{}, nil
}
func (d *pacingDriver) Snapshot(context.Context) (heal.DOMSnapshot, error) {
	return testSnapshot{}, nil
}
func (d *pacingDriver) WaitNetworkIdle(context.Context) error { return nil }

func TestRuntimePacesLeafStepsAcrossWorkflowRepeatAndReferenceBoundaries(t *testing.T) {
	driver := &pacingDriver{}
	var waits []time.Duration
	runtime := &Runtime{RunID: mustInstanceID("run"), Driver: driver, StepInterval: 500 * time.Millisecond,
		pacer: stepPacer{wait: func(_ context.Context, duration time.Duration) error {
			waits = append(waits, duration)
			return nil
		}}}
	navigate := func(id string) Node {
		return &StepNode{NodeID: id, Action: Action{Kind: ActionNavigate, Value: "https://example.test"}}
	}
	program := &WorkflowNode{NodeID: "root", Children: []Node{
		navigate("first"),
		&WorkflowCallNode{NodeID: "call", Target: &WorkflowNode{NodeID: "child", Children: []Node{
			navigate("child-first"),
			&RepeatNode{NodeID: "repeat", Times: 2, Children: []Node{
				&WaitNode{NodeID: "nested-wait", Kind: WaitSleep},
			}},
		}}},
		navigate("last"),
	}}

	if err := program.Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	if got, want := len(waits), 4; got != want {
		t.Fatalf("interval waits = %d, want %d for five leaf Step occurrences", got, want)
	}
	for _, duration := range waits {
		if duration != 500*time.Millisecond {
			t.Fatalf("interval = %s, want 500ms", duration)
		}
	}
	if driver.navigations != 3 {
		t.Fatalf("navigations = %d, want 3", driver.navigations)
	}
}

func TestRuntimeStepIntervalCancellationPreventsNextLeafFromStarting(t *testing.T) {
	driver := &pacingDriver{}
	runtime := &Runtime{RunID: mustInstanceID("run"), Driver: driver, StepInterval: time.Hour}
	first := &StepNode{NodeID: "first", Action: Action{Kind: ActionNavigate, Value: "https://example.test/first"}}
	if err := first.Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	second := &StepNode{NodeID: "second", Action: Action{Kind: ActionNavigate, Value: "https://example.test/second"}}
	if err := second.Run(ctx, runtime); !errors.Is(err, context.Canceled) {
		t.Fatalf("second Run error = %v, want context canceled", err)
	}
	if driver.navigations != 1 {
		t.Fatalf("navigations = %d, want only the first Step", driver.navigations)
	}
}

func TestValidationLeavesConsumeOneIntervalWithoutPacingGroupMembers(t *testing.T) {
	sentinel := errors.New("paced")
	for _, test := range []struct {
		name string
		run  func(context.Context, *Runtime) error
	}{
		{name: "validation", run: (&ValidationNode{NodeID: "validation"}).Run},
		{name: "validation group", run: (&ValidationGroupNode{NodeID: "group"}).Run},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := &Runtime{RunID: mustInstanceID("run"), StepInterval: time.Millisecond,
				pacer: stepPacer{started: true, wait: func(context.Context, time.Duration) error { return sentinel }}}
			if err := test.run(context.Background(), runtime); !errors.Is(err, sentinel) {
				t.Fatalf("Run error = %v, want pacing sentinel", err)
			}
		})
	}
}
