package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestValidationNodeRunBusinessMatrix(t *testing.T) {
	cases := []struct {
		name      string
		assertion ValidationAssertion
		element   *matrixElement
		configure func(*matrixDriver)
		wantErr   bool
		wantPhase Phase
	}{
		{name: "exists pass", assertion: ValidationAssertion{Kind: "exists"}, element: &matrixElement{exists: true}, wantPhase: PhaseSucceeded},
		{name: "visible pass", assertion: ValidationAssertion{Kind: "visible"}, element: &matrixElement{exists: true, visible: true}, wantPhase: PhaseSucceeded},
		{name: "text mismatch timeout", assertion: ValidationAssertion{Kind: "text_equals", Expected: "expected"}, element: &matrixElement{exists: true, text: "actual"}, wantErr: true, wantPhase: PhaseFailed},
		{name: "reader error", assertion: ValidationAssertion{Kind: "visible"}, element: &matrixElement{exists: true, visibleErr: errors.New("reader down")}, wantErr: true, wantPhase: PhaseFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &matrixDriver{element: tc.element}
			if tc.configure != nil {
				tc.configure(d)
			}
			facts := &testFacts{}
			err := (&ValidationNode{NodeID: "validation", Target: fingerprint.ElementTargetSpec{ID: "target"}, Assertion: tc.assertion, MaxWait: 500 * time.Millisecond, Stability: time.Millisecond}).Run(context.Background(), &Runtime{Driver: d, Facts: facts})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			if len(facts.events) == 0 || facts.events[len(facts.events)-1].Phase != tc.wantPhase {
				t.Fatalf("events=%v", facts.events)
			}
		})
	}
}
