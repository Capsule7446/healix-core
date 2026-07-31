package execution

import (
	"errors"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

// requireCreateInstancePlanRejection asserts the boundary-classification
// contract for EXECUTION_CREATE_INSTANCE_PLAN_INVALID: the code and Kind are
// public, the bare internal detail is retained only on the private cause, and
// the detail never reaches the public message.
func requireCreateInstancePlanRejection(t *testing.T, err error, wantDetail string) {
	t.Helper()
	if err == nil {
		t.Fatal("an invalid execution plan was accepted")
	}
	if !fault.IsCode(err, CodeCreateInstancePlanInvalid) {
		t.Fatalf("error = %v, want code %s", err, CodeCreateInstancePlanInvalid)
	}
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Kind() != fault.InvalidArgument {
		t.Fatalf("descriptor = %#v (ok=%v)", descriptor, ok)
	}
	if strings.Contains(descriptor.Message(), wantDetail) {
		t.Fatalf("public message %q carries the detail %q", descriptor.Message(), wantDetail)
	}
	cause := errors.Unwrap(err)
	if cause == nil || !strings.Contains(cause.Error(), wantDetail) {
		t.Fatalf("private cause = %v, want it to retain %q", cause, wantDetail)
	}
}

// requireCreateInstanceSnapshotRejection is the run-snapshot boundary
// counterpart of requireCreateInstancePlanRejection.
func requireCreateInstanceSnapshotRejection(t *testing.T, err error, wantDetails ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("an invalid run snapshot was accepted")
	}
	if !fault.IsCode(err, CodeCreateInstanceSnapshotInvalid) {
		t.Fatalf("error = %v, want code %s", err, CodeCreateInstanceSnapshotInvalid)
	}
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Kind() != fault.InvalidArgument {
		t.Fatalf("descriptor = %#v (ok=%v)", descriptor, ok)
	}
	cause := errors.Unwrap(err)
	if cause == nil {
		t.Fatalf("private cause is nil for %v", err)
	}
	for _, detail := range wantDetails {
		if strings.Contains(descriptor.Message(), detail) {
			t.Fatalf("public message %q carries the detail %q", descriptor.Message(), detail)
		}
		if !strings.Contains(cause.Error(), detail) {
			t.Fatalf("private cause = %v, want it to retain %q", cause, detail)
		}
	}
}

// requireStepShapeViolation asserts that err is the step-shape envelope and
// that it carries a violation at wantField with wantCode.
func requireStepShapeViolation(t *testing.T, err error, wantField string, wantCode fault.Code) {
	t.Helper()
	if !fault.IsCode(err, CodeCreateInstanceStepShapeInvalid) {
		t.Fatalf("error = %v, want code %s", err, CodeCreateInstanceStepShapeInvalid)
	}
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Kind() != fault.InvalidArgument {
		t.Fatalf("descriptor = %#v (ok=%v)", descriptor, ok)
	}
	for _, violation := range descriptor.Violations() {
		if violation.Field() == wantField && violation.Code() == wantCode {
			return
		}
	}
	t.Fatalf("violations = %#v, want %s at %q", descriptor.Violations(), wantCode, wantField)
}

// requireEnvironmentSnapshotViolation is the environment-envelope counterpart
// of requireStepShapeViolation.
func requireEnvironmentSnapshotViolation(t *testing.T, err error, wantField string, wantCode fault.Code) {
	t.Helper()
	if !fault.IsCode(err, CodeEnvironmentSnapshotInvalid) {
		t.Fatalf("error = %v, want code %s", err, CodeEnvironmentSnapshotInvalid)
	}
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Kind() != fault.InvalidArgument {
		t.Fatalf("descriptor = %#v (ok=%v)", descriptor, ok)
	}
	for _, violation := range descriptor.Violations() {
		if violation.Field() == wantField && violation.Code() == wantCode {
			return
		}
	}
	t.Fatalf("violations = %#v, want %s at %q", descriptor.Violations(), wantCode, wantField)
}
