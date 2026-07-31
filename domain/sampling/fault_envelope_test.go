package sampling

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// envelopeKinds mirrors the Kind column of the registry's SAMPLING_* rows, so a
// helper that hardcoded one Kind can no longer silently misclassify a code.
var envelopeKinds = map[fault.Code]fault.Kind{
	CodeSessionInputInvalid:        fault.InvalidArgument,
	CodeSessionStateInvalid:        fault.FailedPrecondition,
	CodeCaptureInvalid:             fault.InvalidArgument,
	CodeDraftInvalid:               fault.InvalidArgument,
	CodeDraftStepNotFound:          fault.NotFound,
	CodeDraftElementTargetNotFound: fault.NotFound,
	CodeDraftElementTargetInUse:    fault.FailedPrecondition,
	CodeDraftIndexOutOfRange:       fault.OutOfRange,
	CodePublicationMappingInvalid:  fault.InvalidArgument,
	CodeWorkspaceInvalid:           fault.InvalidArgument,
	CodeInternal:                   fault.Internal,
}

func violationKey(code fault.Code, field string) string {
	return string(code) + "@" + field
}

func violationKeys(violations []fault.Violation) []string {
	keys := make([]string, 0, len(violations))
	for _, violation := range violations {
		keys = append(keys, violationKey(violation.Code(), violation.Field()))
	}
	return keys
}

func requireEnvelope(t *testing.T, err error, wantCode fault.Code) fault.Descriptor {
	t.Helper()
	if err == nil {
		t.Fatal("operation accepted invalid input")
	}
	if !fault.IsCode(err, wantCode) {
		t.Fatalf("error = %v, want code %s", err, wantCode)
	}
	descriptor, ok := fault.Describe(err)
	if !ok {
		t.Fatalf("error is not a fault: %v", err)
	}
	wantKind, registered := envelopeKinds[wantCode]
	if !registered {
		t.Fatalf("code %s has no registered Kind in envelopeKinds", wantCode)
	}
	if descriptor.Kind() != wantKind {
		t.Fatalf("kind = %s, want %s", descriptor.Kind(), wantKind)
	}
	return descriptor
}

func requireViolation(t *testing.T, err error, wantCode, wantViolation fault.Code, wantField string) {
	t.Helper()
	descriptor := requireEnvelope(t, err, wantCode)
	for _, violation := range descriptor.Violations() {
		if violation.Code() == wantViolation && violation.Field() == wantField {
			return
		}
	}
	t.Fatalf("violations = [%s], want one with %s", strings.Join(violationKeys(descriptor.Violations()), ", "), violationKey(wantViolation, wantField))
}

func requireNoPublicLeak(t *testing.T, err error, secrets ...string) {
	t.Helper()
	descriptor, ok := fault.Describe(err)
	if !ok {
		t.Fatalf("error is not a fault: %v", err)
	}
	texts := []string{err.Error(), descriptor.Message()}
	for _, param := range descriptor.Params() {
		texts = append(texts, string(param.Key()), param.Value())
	}
	for _, violation := range descriptor.Violations() {
		texts = append(texts, violation.Field(), violation.Message())
		for _, param := range violation.Params() {
			texts = append(texts, string(param.Key()), param.Value())
		}
	}
	for _, secret := range secrets {
		// An empty secret is contained in every string, so it would assert nothing
		// while looking like it did.
		if secret == "" {
			continue
		}
		for _, text := range texts {
			if strings.Contains(text, secret) {
				t.Fatalf("public fault text %q leaks %q", text, secret)
			}
		}
	}
}

// Each of these used to report only the first failure, so a caller fixed one
// field and immediately hit the next. The contract asks for one fault carrying
// every field failure, and that is only worth anything if it is actually every.
func TestRecordReportsEveryCaptureShapeFailureAtOnce(t *testing.T) {
	session, err := NewSession("flow", "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(); err != nil {
		t.Fatal(err)
	}

	// A validate capture missing its id, its identity key, and its validation.
	_, recordErr := session.Record(Capture{Kind: ActionValidate})
	descriptor := requireEnvelope(t, recordErr, CodeCaptureInvalid)
	want := []string{
		violationKey(fault.CodeFieldRequired, "captureId"),
		violationKey(fault.CodeFieldRequired, "identityKey"),
		violationKey(fault.CodeFieldRequired, "validation"),
	}
	if got := violationKeys(descriptor.Violations()); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("violations =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestDraftIdentityReportsEveryBadStepAtOnce(t *testing.T) {
	workflow := draftFixture()
	// Two separate steps with blank ids, in different branches of the tree.
	workflow.Steps[0].ID = " "
	workflow.Steps[1].Children[0].ID = " "

	_, err := InsertUnpublishedFlowFragmentStep(workflow, FlowFragmentStepContainer{}, len(workflow.Steps),
		automation.FlowFragmentStep{ID: "new", DisplayName: "new", Kind: automation.StepAction, ElementTargetID: "node-c"})
	descriptor := requireEnvelope(t, err, CodeDraftInvalid)

	blanks := 0
	for _, violation := range descriptor.Violations() {
		if violation.Code() == fault.CodeFieldRequired && violation.Field() == "steps.id" {
			blanks++
		}
	}
	if blanks != 2 {
		t.Fatalf("reported %d blank step ids, want both: %v", blanks, violationKeys(descriptor.Violations()))
	}
}

// A parent that exists but cannot hold the requested container is a different
// failure from an absent parent, and has a different fix.
func TestContainerShapeMismatchIsNotReportedAsAMissingStep(t *testing.T) {
	workflow := draftFixture()
	step := automation.FlowFragmentStep{ID: "new", DisplayName: "new", Kind: automation.StepAction, ElementTargetID: "node-a"}

	_, absent := InsertUnpublishedFlowFragmentStep(workflow, FlowFragmentStepContainer{ParentStepID: "not-here"}, 0, step)
	requireEnvelope(t, absent, CodeDraftStepNotFound)

	// "a" is an action step: it exists, but it cannot hold children.
	_, incompatible := InsertUnpublishedFlowFragmentStep(workflow, FlowFragmentStepContainer{ParentStepID: "a"}, 0, step)
	requireViolation(t, incompatible, CodeDraftInvalid, fault.CodeFieldMismatch, "container")
	if fault.IsCode(incompatible, CodeDraftStepNotFound) {
		t.Fatal("an existing parent was reported as a missing step")
	}
}
