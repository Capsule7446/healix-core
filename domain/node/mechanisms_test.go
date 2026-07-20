package node

import (
	"context"
	"errors"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type failingOperationObserver struct{}

func (failingOperationObserver) RecordOperation(context.Context, OperationObservation) error {
	return errors.New("observer unavailable")
}

func TestRuntimeBestEffortObservationDoesNotReturnError(t *testing.T) {
	rt := &Runtime{OperationObserver: failingOperationObserver{}}
	rt.observeOperationBestEffort(context.Background(), OperationObservation{NodeID: "node", Operation: "locate"})
}

func TestRuntimeLocatorUsesSelectorOverlay(t *testing.T) {
	driver := &matrixDriver{}
	var got fingerprint.NodeSpec
	driver.locate = func(_ context.Context, spec fingerprint.NodeSpec) (Element, error) {
		got = spec
		return &matrixElement{exists: true}, nil
	}
	rt := &Runtime{Driver: driver, SelectorOverlay: map[string][]fingerprint.Selector{
		"target": {{Type: fingerprint.SelectorCSS, Value: "#healed"}},
	}}
	if _, err := rt.locator().Locate(context.Background(), fingerprint.NodeSpec{
		ID: "target", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#old"}},
	}); err != nil {
		t.Fatal(err)
	}
	if got.Selectors[0].Value != "#healed" {
		t.Fatalf("selector=%v", got.Selectors)
	}
}
