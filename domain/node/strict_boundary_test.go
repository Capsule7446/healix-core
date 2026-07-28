package node

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestRepeatNodeRejectsInvalidCountsAndChildren(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node RepeatNode
	}{
		{name: "negative count", node: RepeatNode{NodeID: "repeat", Times: -1}},
		{name: "nil child", node: RepeatNode{NodeID: "repeat", Times: 1, Children: []Node{nil}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &Runtime{}
			if err := tt.node.Run(context.Background(), rt); err == nil {
				t.Fatal("Run() error = nil, want invalid public parameter error")
			}
		})
	}
}

func TestCompositeNodesRejectNilChildren(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(context.Context, *Runtime) error
	}{
		{name: "workflow", run: (&WorkflowNode{NodeID: "workflow", Children: []Node{nil}}).Run},
		{name: "validation branch", run: (&ValidationGroupNode{NodeID: "group", Branches: []ValidationBranch{{ID: "branch", Nodes: []*ValidationNode{nil}}}}).Run},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("Run() panicked for invalid public input: %v", recovered)
				}
			}()
			if err := tt.run(context.Background(), &Runtime{}); err == nil {
				t.Fatal("Run() error = nil, want invalid composite error")
			}
		})
	}
}

func TestPublicDurationBoundaries(t *testing.T) {
	t.Parallel()

	waitCases := []struct {
		name    string
		wait    WaitNode
		wantErr bool
	}{
		{name: "sleep negative", wait: WaitNode{Kind: WaitSleep, Duration: -1}, wantErr: true},
		{name: "sleep zero", wait: WaitNode{Kind: WaitSleep}},
		{name: "sleep one", wait: WaitNode{Kind: WaitSleep, Duration: 1}},
		{name: "sleep max", wait: WaitNode{Kind: WaitSleep, Duration: time.Duration(math.MaxInt64)}},
		{name: "condition timeout negative", wait: WaitNode{Kind: WaitElement, Timeout: -1}, wantErr: true},
		{name: "condition timeout zero", wait: WaitNode{Kind: WaitElement}},
		{name: "condition timeout one", wait: WaitNode{Kind: WaitElement, Timeout: 1}},
		{name: "condition timeout max", wait: WaitNode{Kind: WaitElement, Timeout: time.Duration(math.MaxInt64)}},
	}
	for _, tt := range waitCases {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.wait.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	completionCases := []struct {
		name    string
		options NodeCompletionOptions
		wantErr bool
	}{
		{name: "handler negative", options: NodeCompletionOptions{HandlerTimeout: -1}, wantErr: true},
		{name: "chain negative", options: NodeCompletionOptions{ChainTimeout: -1}, wantErr: true},
		{name: "zero defaults", options: NodeCompletionOptions{}},
		{name: "one", options: NodeCompletionOptions{HandlerTimeout: 1, ChainTimeout: 1}},
		{name: "max", options: NodeCompletionOptions{HandlerTimeout: time.Duration(math.MaxInt64), ChainTimeout: time.Duration(math.MaxInt64)}},
	}
	for _, tt := range completionCases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewNodeCompletionChain(tt.options)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewNodeCompletionChain() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStepExecutionRejectsDuplicateAndUnknownTerminalTransitions(t *testing.T) {
	t.Parallel()

	for _, terminal := range []Phase{PhaseSucceeded, PhaseFailed, PhaseCanceled} {
		t.Run(string(terminal), func(t *testing.T) {
			execution := NewStepExecution("step")
			if err := execution.Transition(PhaseRunning); err != nil {
				t.Fatal(err)
			}
			if terminal == PhaseSucceeded {
				if err := execution.Transition(PhaseTransitioning); err != nil {
					t.Fatal(err)
				}
			}
			if err := execution.Transition(terminal); err != nil {
				t.Fatalf("first terminal transition: %v", err)
			}
			if err := execution.Transition(terminal); err == nil {
				t.Fatal("duplicate terminal transition succeeded")
			}
			if err := execution.Transition(Phase("UNKNOWN")); err == nil {
				t.Fatal("unknown transition succeeded")
			}
		})
	}
}

func TestLifecycleOutcomePreservesWrappedCancellation(t *testing.T) {
	t.Parallel()

	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		outcome, stepOutcome := lifecycleOutcome(errors.Join(errors.New("operation failed"), cause))
		if outcome != NodeOutcomeCanceled || stepOutcome != StepOutcomeCanceled {
			t.Fatalf("lifecycleOutcome(%v) = %q, %q", cause, outcome, stepOutcome)
		}
	}
}
