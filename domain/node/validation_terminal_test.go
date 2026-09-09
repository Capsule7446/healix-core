package node

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type terminalValidationElement struct {
	testElement
	read func(context.Context) (ValidationState, error)
}

func (e terminalValidationElement) ValidationState(ctx context.Context) (ValidationState, error) {
	return e.read(ctx)
}

func TestValidationFinalKeepsLastSuccessfulRead(t *testing.T) {
	tests := []struct {
		name         string
		assertion    ValidationAssertion
		states       []ValidationState
		actual       string
		actualValues []string
	}{
		{
			name:      "scalar",
			assertion: ValidationAssertion{Kind: "value_equals", Expected: "123"},
			states:    []ValidationState{{Value: "999"}},
			actual:    "999",
		},
		{
			name:         "collection preserves source order",
			assertion:    ValidationAssertion{Kind: "selected_set_equals", ExpectedValues: []string{"expected"}},
			states:       []ValidationState{{SelectedTexts: []string{"Taiwan", "Japan"}}},
			actual:       "Japan, Taiwan",
			actualValues: []string{"Taiwan", "Japan"},
		},
		{
			name:      "successful empty scalar replaces prior value",
			assertion: ValidationAssertion{Kind: "value_equals", Expected: "123"},
			states:    []ValidationState{{Value: "999"}, {}},
		},
		{
			name:      "successful empty collection replaces prior values",
			assertion: ValidationAssertion{Kind: "selected_set_equals", ExpectedValues: []string{"expected"}},
			states:    []ValidationState{{SelectedTexts: []string{"Taiwan", "Japan"}}, {}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				element := terminalValidationElement{read: func(ctx context.Context) (ValidationState, error) {
					calls++
					if calls <= len(test.states) {
						return test.states[calls-1], nil
					}
					<-ctx.Done()
					return ValidationState{}, ctx.Err()
				}}
				validation, runtime, facts := terminalValidationRuntime(t, test.assertion, element)
				err := validation.Run(t.Context(), runtime)
				if code, ok := fault.CodeOf(err); !ok || code != CodeTimeout || !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("ValidationNode.Run(%s) error = %v, code = %q; want timeout retaining deadline", test.name, err, code)
				}
				if calls != len(test.states)+1 {
					t.Errorf("ValidationState(%s) calls = %d, want %d", test.name, calls, len(test.states)+1)
				}
				assertValidationFinal(t, facts.validationObservations, ValidationObservation{
					NodeID: "validation", Assertion: test.assertion,
					Actual: test.actual, ActualValues: test.actualValues, Reason: "timeout",
					Selector:     fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#target"},
					ObservedAtMS: time.Now().UnixMilli(), Final: true,
				})
			})
		})
	}
}

func TestValidationFinalKeepsActualWhenParentStops(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "cancel"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				parent, cancel := context.WithCancel(t.Context())
				wantCause, wantCode := context.Canceled, CodeCanceled
				if deadline {
					cancel()
					parent, cancel = context.WithTimeout(t.Context(), 500*time.Millisecond)
					wantCause, wantCode = context.DeadlineExceeded, CodeTimeout
				}
				t.Cleanup(cancel)
				calls := 0
				element := terminalValidationElement{read: func(ctx context.Context) (ValidationState, error) {
					calls++
					if calls == 1 {
						return ValidationState{Value: "999"}, nil
					}
					if !deadline {
						cancel()
					}
					<-ctx.Done()
					return ValidationState{}, ctx.Err()
				}}
				assertion := ValidationAssertion{Kind: "value_equals", Expected: "123"}
				validation, runtime, facts := terminalValidationRuntime(t, assertion, element)
				err := validation.Run(parent, runtime)
				if code, ok := fault.CodeOf(err); !ok || code != wantCode || !errors.Is(err, wantCause) {
					t.Errorf("ValidationNode.Run(parent %s) error = %v, code = %q; want cause %v and code %q", name, err, code, wantCause, wantCode)
				}
				assertValidationFinal(t, facts.validationObservations, ValidationObservation{
					NodeID: "validation", Assertion: assertion, Actual: "999", Reason: "canceled",
					Selector:     fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#target"},
					ObservedAtMS: time.Now().UnixMilli(), Final: true,
				})
			})
		})
	}
}

func TestValidationFinalKeepsActualAfterIndependentReadError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		driverErr := errors.New("validation reader stopped")
		calls := 0
		element := terminalValidationElement{read: func(context.Context) (ValidationState, error) {
			calls++
			if calls == 1 {
				return ValidationState{Value: "999"}, nil
			}
			return ValidationState{}, driverErr
		}}
		assertion := ValidationAssertion{Kind: "value_equals", Expected: "123"}
		validation, runtime, facts := terminalValidationRuntime(t, assertion, element)
		err := validation.Run(t.Context(), runtime)
		if !errors.Is(err, driverErr) || fault.IsCode(err, CodeTimeout) {
			t.Errorf("ValidationNode.Run(driver error) error = %v; want original driver cause without a timeout", err)
		}
		assertValidationFinal(t, facts.validationObservations, ValidationObservation{
			NodeID: "validation", Assertion: assertion, Actual: "999", Reason: "system_error",
			Selector:     fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#target"},
			ObservedAtMS: time.Now().UnixMilli(), Final: true,
		})
	})
}

func TestValidationFinalKeepsActualAfterComparisonError(t *testing.T) {
	for _, kind := range []string{"text_matches", "value_matches"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				element := terminalValidationElement{read: func(context.Context) (ValidationState, error) {
					return ValidationState{Value: "value"}, nil
				}}
				assertion := ValidationAssertion{Kind: kind, Expected: "["}
				validation, runtime, facts := terminalValidationRuntime(t, assertion, element)
				if err := validation.Run(t.Context(), runtime); err == nil {
					t.Errorf("ValidationNode.Run(%s with invalid expression) error = nil, want comparison error", kind)
				}
				assertValidationFinal(t, facts.validationObservations, ValidationObservation{
					NodeID: "validation", Assertion: assertion, Actual: "value", Reason: "system_error",
					Selector:     fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#target"},
					ObservedAtMS: time.Now().UnixMilli(), Final: true,
				})
			})
		})
	}
}

func TestValidationFinalPreservesIndependentContextFailure(t *testing.T) {
	classified, err := fault.Wrap(context.DeadlineExceeded, fault.Internal, CodeOperationFailed, "independent driver failure")
	if err != nil {
		t.Fatalf("construct classified driver failure: %v", err)
	}
	tests := []struct {
		name string
		err  error
	}{
		{name: "driver deadline", err: context.DeadlineExceeded},
		{name: "driver cancellation", err: context.Canceled},
		{name: "classified driver deadline", err: classified},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				element := terminalValidationElement{read: func(context.Context) (ValidationState, error) {
					calls++
					if calls == 1 {
						return ValidationState{Value: "999"}, nil
					}
					return ValidationState{}, test.err
				}}
				assertion := ValidationAssertion{Kind: "value_equals", Expected: "123"}
				validation, runtime, facts := terminalValidationRuntime(t, assertion, element)
				err := validation.Run(t.Context(), runtime)
				if !errors.Is(err, test.err) || fault.IsCode(err, CodeTimeout) {
					t.Errorf("ValidationNode.Run(%s) error = %v; want independent driver cause without a timeout", test.name, err)
				}
				assertValidationFinal(t, facts.validationObservations, ValidationObservation{
					NodeID: "validation", Assertion: assertion, Actual: "999", Reason: "system_error",
					Selector:     fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#target"},
					ObservedAtMS: time.Now().UnixMilli(), Final: true,
				})
			})
		})
	}
}

func terminalValidationRuntime(t *testing.T, assertion ValidationAssertion, element Element) (*ValidationNode, *Runtime, *testFacts) {
	t.Helper()
	facts := &testFacts{rejectCanceled: true}
	validation := &ValidationNode{
		NodeID: "validation", Assertion: assertion, MaxWait: time.Second,
		Target: fingerprint.ElementTargetSpec{ID: "target", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#target"}}},
	}
	runtime := &Runtime{
		InstanceID: mustInstanceID("validation-run"), Facts: facts,
		Driver: &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return element, nil }},
	}
	return validation, runtime, facts
}

func assertValidationFinal(t *testing.T, observations []ValidationObservation, want ValidationObservation) {
	t.Helper()
	var finals []ValidationObservation
	for _, observation := range observations {
		if observation.Final {
			finals = append(finals, observation)
		}
	}
	if len(finals) != 1 {
		t.Errorf("ValidationNode.Run final observations = %+v, want exactly one", finals)
		return
	}
	// Compare every exported field, including collection order and nil values,
	// without adding a dependency to the framework-independent module.
	gotJSON, err := json.Marshal(finals[0])
	if err != nil {
		t.Fatalf("marshal actual validation observation: %v", err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal expected validation observation: %v", err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("ValidationNode.Run final observation = %s, want %s", gotJSON, wantJSON)
	}
}
