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
	completion := &LeafCompletionError{NodeErr: nodeErr, TimelineErr: errors.New("timeline")}
	wrapped := fmt.Errorf("wrapped: %w", completion)
	for _, tt := range []struct {
		name        string
		input, want error
	}{{"plain", nodeErr, nodeErr}, {"completion", completion, nodeErr}, {"wrapped completion", wrapped, nodeErr}, {"nil", nil, nil}} {
		t.Run(tt.name, func(t *testing.T) {
			if got := LeafExecutionError(tt.input); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestIsTransientDirect(t *testing.T) {
	transient := TransientError("click", errors.New("retry"))
	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{{"nil", nil, false}, {"plain", errors.New("x"), false}, {"transient", transient, true}, {"wrapped", fmt.Errorf("outer: %w", transient), true}, {"permanent", ClassifyError("click", errors.New("bad")), false}} {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTransient(tt.err); got != tt.want {
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
