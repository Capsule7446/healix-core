package node

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type directGapHandler struct{}

func (directGapHandler) Name() string                                        { return "handler" }
func (directGapHandler) Handle(context.Context, NodeCompletionContext) error { return nil }

func TestNodeCompletionChainHasHandlersDirect(t *testing.T) {
	var nilChain *NodeCompletionChain
	empty, err := NewNodeCompletionChain(NodeCompletionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	populated, err := NewNodeCompletionChain(NodeCompletionOptions{}, directGapHandler{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		chain *NodeCompletionChain
		want  bool
	}{{"nil", nilChain, false}, {"empty", empty, false}, {"populated", populated, true}} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.chain.HasHandlers(); got != tt.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestLeafExecutionErrorDirect(t *testing.T) {
	nodeErr := errors.New("node")
	completion := newLeafCompletionError(nodeErr, stepTimelineFinishError(errors.New("timeline")), nil)
	wrapped := fmt.Errorf("wrapped: %w", completion)
	telemetryOnly := newLeafCompletionError(nil, stepTimelineFinishError(errors.New("timeline only")), nil)
	canceled := newLeafCompletionError(context.Canceled, stepTimelineFinishError(errors.New("timeline canceled")), nil)
	independent := errors.New("independent")
	joinedTelemetry := errors.Join(telemetryOnly, independent)
	wrappedJoinedTelemetry := fmt.Errorf("wrapped: %w", joinedTelemetry)
	joinedNode := errors.Join(completion, independent)
	joinedCanceled := errors.Join(telemetryOnly, context.Canceled)
	for _, tt := range []struct {
		name        string
		input, want error
	}{
		{"plain", nodeErr, nodeErr},
		{"completion", completion, nodeErr},
		{"wrapped completion", wrapped, nodeErr},
		{"telemetry only", telemetryOnly, nil},
		{"canceled node", canceled, context.Canceled},
		{"joined telemetry", joinedTelemetry, independent},
		{"wrapped joined telemetry", wrappedJoinedTelemetry, independent},
		{"joined node", joinedNode, nodeErr},
		{"joined cancellation", joinedCanceled, context.Canceled},
		{"nil", nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := LeafExecutionError(tt.input)
			if tt.want == nil && got != nil || tt.want != nil && !errors.Is(got, tt.want) {
				t.Fatalf("got %v want cause %v", got, tt.want)
			}
			if tt.name == "joined node" && !errors.Is(got, independent) {
				t.Fatalf("joined sibling cause was lost: %v", got)
			}
		})
	}
}

func TestExclusiveTransientDriverFaultDirect(t *testing.T) {
	transient := transientDriverFault(errors.New("retry"))
	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain", errors.New("x"), false},
		{"transient", transient, true},
		{"wrapped", fmt.Errorf("outer: %w", transient), true},
		{"mixed", errors.Join(transient, errors.New("bad")), false},
		{"nested mixed", fmt.Errorf("outer: %w", errors.Join(transient, errors.New("bad"))), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isExclusiveTransientDriverFault(tt.err); got != tt.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestRuntimeLeafExecutionStartedDirect(t *testing.T) {
	var nilRuntime *Runtime
	if nilRuntime.LeafExecutionStarted() {
		t.Fatal("nil runtime started")
	}
	runtime := &Runtime{}
	if runtime.LeafExecutionStarted() {
		t.Fatal("zero runtime started")
	}
	runtime.leafExecutionStarted = true
	if !runtime.LeafExecutionStarted() {
		t.Fatal("marked runtime not started")
	}
}
