package execution

import (
	"fmt"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func violationSequenceEqual(t *testing.T, run int, got, want []fault.Violation) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("run %d: violation count = %d, want %d", run, len(got), len(want))
	}
	for index := range got {
		if got[index].Code() != want[index].Code() || got[index].Field() != want[index].Field() || got[index].Message() != want[index].Message() {
			t.Fatalf("run %d: violation[%d] = %#v, want %#v", run, index, got[index], want[index])
		}
	}
}

// TestWorkflowSnapshotValidateViolationOrderIsDeterministic repeats the same
// malformed workflow snapshot 50 times and requires the exact same ordered
// violation sequence every run: violation order must be a function of the
// input alone, never of map iteration.
func TestWorkflowSnapshotValidateViolationOrderIsDeterministic(t *testing.T) {
	workflow := validWorkflowSnapshot()
	workflow.Parameters = []Parameter{{Name: "dup"}, {Name: "dup"}, {Name: "dup"}}

	var first []fault.Violation
	for run := 0; run < 50; run++ {
		descriptor, ok := fault.Describe(workflow.Validate())
		if !ok {
			t.Fatalf("run %d: error is not a fault", run)
		}
		violations := descriptor.Violations()
		if run == 0 {
			first = violations
			if len(first) == 0 {
				t.Fatal("expected at least one violation")
			}
			continue
		}
		violationSequenceEqual(t, run, violations, first)
	}
}

// TestWorkflowSnapshotValidateCapsViolationsAtDeterministicPrefix exceeds
// fault.MaxViolations with a flat, individually-invalid step slice and
// requires the envelope to keep exactly the leading prefix.
func TestWorkflowSnapshotValidateCapsViolationsAtDeterministicPrefix(t *testing.T) {
	const stepCount = fault.MaxViolations + 8
	steps := make([]Step, stepCount)
	for index := range steps {
		steps[index] = Step{ID: fmt.Sprintf("step-%02d", index), DisplayName: fmt.Sprintf("Step %02d", index), Kind: StepKind("BOGUS")}
	}
	workflow := validWorkflowSnapshot()
	workflow.Steps = steps

	descriptor, ok := fault.Describe(workflow.Validate())
	if !ok {
		t.Fatal("error is not a fault")
	}
	violations := descriptor.Violations()
	if len(violations) != fault.MaxViolations {
		t.Fatalf("violations = %d, want capped at %d", len(violations), fault.MaxViolations)
	}
	for _, violation := range violations {
		if violation.Field() != "steps" || violation.Code() != fault.CodeFieldInvalid {
			t.Fatalf("violation = %#v, want steps/CodeFieldInvalid", violation)
		}
	}
}

func environmentEnvelopeFixture() (EnvironmentSnapshot, ScreenshotPolicySnapshot, HealerPolicySnapshot) {
	env := EnvironmentSnapshot{ID: "env", DisplayName: "Env", Revision: 1, Variables: map[string]parameter.Value{
		"bad-a\x00": parameter.TextValue("x"),
		"bad-b\x00": parameter.TextValue("x"),
		"bad-c\x00": parameter.TextValue("x"),
	}}
	return env, ScreenshotPolicySnapshot{Version: ScreenshotPolicyV1}, DefaultHealerPolicySnapshot()
}

// TestValidateEnvironmentSnapshotViolationOrderIsDeterministic is the
// environment-envelope counterpart: variable and property maps are walked in
// sorted key order, so the violation sequence must repeat exactly.
func TestValidateEnvironmentSnapshotViolationOrderIsDeterministic(t *testing.T) {
	env, screenshot, healer := environmentEnvelopeFixture()

	var first []fault.Violation
	for run := 0; run < 50; run++ {
		descriptor, ok := fault.Describe(validateEnvironmentSnapshot(InstanceSnapshotSchemaV2, env, screenshot, healer))
		if !ok {
			t.Fatalf("run %d: error is not a fault", run)
		}
		violations := descriptor.Violations()
		if run == 0 {
			first = violations
			if len(first) == 0 {
				t.Fatal("expected at least one violation")
			}
			continue
		}
		violationSequenceEqual(t, run, violations, first)
	}
}

// TestValidateEnvironmentSnapshotCapsViolationsAtDeterministicPrefix exceeds
// fault.MaxViolations with malformed environment variables and requires the
// envelope to keep exactly the sorted leading prefix.
func TestValidateEnvironmentSnapshotCapsViolationsAtDeterministicPrefix(t *testing.T) {
	const variableCount = fault.MaxViolations + 8
	variables := make(map[string]parameter.Value, variableCount)
	for index := 0; index < variableCount; index++ {
		variables[fmt.Sprintf("bad-%02d\x00", index)] = parameter.TextValue("x")
	}
	env := EnvironmentSnapshot{ID: "env", DisplayName: "Env", Revision: 1, Variables: variables}

	descriptor, ok := fault.Describe(validateEnvironmentSnapshot(InstanceSnapshotSchemaV2, env, ScreenshotPolicySnapshot{Version: ScreenshotPolicyV1}, DefaultHealerPolicySnapshot()))
	if !ok {
		t.Fatal("error is not a fault")
	}
	violations := descriptor.Violations()
	if len(violations) != fault.MaxViolations {
		t.Fatalf("violations = %d, want capped at %d", len(violations), fault.MaxViolations)
	}
	for _, violation := range violations {
		if violation.Field() != "environment.variables" || violation.Code() != fault.CodeFieldInvalid {
			t.Fatalf("violation = %#v, want environment.variables/CodeFieldInvalid", violation)
		}
	}
}
