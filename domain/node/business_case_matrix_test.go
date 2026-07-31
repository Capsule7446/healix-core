package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type operationFacts struct{ observations []OperationObservation }

func (f *operationFacts) RecordOperation(_ context.Context, observation OperationObservation) error {
	f.observations = append(f.observations, observation)
	return nil
}

func TestStepActionFailureBusinessMatrix(t *testing.T) {
	cases := []struct {
		name      string
		action    Action
		configure func(*matrixElement)
		wantKind  fault.Kind
		wantCode  fault.Code
	}{
		{name: "click action error", action: Action{Kind: ActionClick}, configure: func(e *matrixElement) { e.actionErr = errors.New("click failed") }, wantKind: fault.Internal, wantCode: CodeOperationFailed},
		{name: "stable wait error", action: Action{Kind: ActionHover}, configure: func(e *matrixElement) { e.waitStableErr = errors.New("moving") }, wantKind: fault.Internal, wantCode: CodeOperationFailed},
		{name: "select without value", action: Action{Kind: ActionSelect}, configure: func(*matrixElement) {}, wantKind: fault.Internal, wantCode: CodeOperationFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &matrixElement{exists: true}
			tc.configure(e)
			d := &matrixDriver{element: e}
			facts := &operationFacts{}
			err := (&StepNode{NodeID: "step", Target: fingerprint.ElementTargetSpec{ID: "target"}, Action: tc.action}).Run(context.Background(), &Runtime{Driver: d, OperationObserver: facts})
			if err == nil {
				t.Fatal("expected action failure")
			}
			if len(facts.observations) == 0 || facts.observations[len(facts.observations)-1].FaultKind != tc.wantKind || facts.observations[len(facts.observations)-1].FaultCode != tc.wantCode {
				t.Fatalf("observations=%+v", facts.observations)
			}
		})
	}
}

func TestWaitControlBusinessMatrix(t *testing.T) {
	cases := []struct {
		name      string
		kind      WaitKind
		element   *matrixElement
		locateErr error
		wantErr   bool
	}{
		{name: "present", kind: WaitElement, element: &matrixElement{exists: true}},
		{name: "visible", kind: WaitElementVisible, element: &matrixElement{exists: true, visible: true}},
		{name: "invisible", kind: WaitElementInvisible, element: &matrixElement{exists: true, visible: false}},
		{name: "removed satisfies invisible", kind: WaitElementInvisible, locateErr: NewElementNotFoundError()},
		{name: "permanent locate error", kind: WaitElement, locateErr: errors.New("selector invalid"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &matrixDriver{element: tc.element, locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return tc.element, tc.locateErr }}
			err := (&WaitNode{NodeID: "wait", Kind: tc.kind, Target: fingerprint.ElementTargetSpec{ID: "target"}, Timeout: time.Second}).Run(context.Background(), &Runtime{Driver: d})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestOperationObservationBusinessMatrix(t *testing.T) {
	cases := []struct {
		name          string
		action        Action
		configure     func(*matrixDriver)
		wantOperation string
		wantSuccess   bool
	}{
		{name: "navigate success", action: Action{Kind: ActionNavigate, Value: "https://example.test/home"}, wantOperation: "navigate", wantSuccess: true},
		{name: "navigate failure", action: Action{Kind: ActionNavigate, Value: "https://example.test/home"}, configure: func(d *matrixDriver) { d.navigateErr = errors.New("navigation failed") }, wantOperation: "navigate", wantSuccess: false},
		{name: "press success", action: Action{Kind: ActionPress, Value: "Enter"}, wantOperation: "press", wantSuccess: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &matrixDriver{}
			if tc.configure != nil {
				tc.configure(d)
			}
			facts := &operationFacts{}
			err := (&StepNode{NodeID: "step", Action: tc.action}).Run(context.Background(), &Runtime{Driver: d, OperationObserver: facts})
			if (err == nil) != tc.wantSuccess {
				t.Fatalf("err=%v", err)
			}
			if len(facts.observations) != 1 || facts.observations[0].Operation != tc.wantOperation || facts.observations[0].Succeeded != tc.wantSuccess {
				t.Fatalf("observations=%+v", facts.observations)
			}
		})
	}
}
