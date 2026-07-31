package automation

import (
	"errors"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

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

func requireExecutionFlowEnvelope(t *testing.T, err error) fault.Descriptor {
	t.Helper()
	if err == nil {
		t.Fatal("Validate() accepted invalid input")
	}
	if !fault.IsCode(err, CodeExecutionFlowInvalid) {
		t.Fatalf("Validate() error = %v, want code %s", err, CodeExecutionFlowInvalid)
	}
	descriptor, ok := fault.Describe(err)
	if !ok {
		t.Fatalf("Validate() error is not a fault: %v", err)
	}
	if descriptor.Kind() != fault.InvalidArgument {
		t.Fatalf("Validate() kind = %s, want %s", descriptor.Kind(), fault.InvalidArgument)
	}
	if len(descriptor.Violations()) == 0 {
		t.Fatal("aggregate envelope carries no violations")
	}
	if unwrapped := errors.Unwrap(err); unwrapped != nil {
		t.Fatalf("aggregate envelope nests another error: %v", unwrapped)
	}
	return descriptor
}

func requireExecutionFlowViolation(t *testing.T, err error, wantCode fault.Code, wantField string) {
	t.Helper()
	descriptor := requireExecutionFlowEnvelope(t, err)
	for _, violation := range descriptor.Violations() {
		if violation.Code() == wantCode && violation.Field() == wantField {
			return
		}
	}
	t.Fatalf("Validate() violations = [%s], want one with %s", strings.Join(violationKeys(descriptor.Violations()), ", "), violationKey(wantCode, wantField))
}

// brokenExecutionFlowVersion breaks every independent rule at once so the whole
// violation order is observable in one envelope.
func brokenExecutionFlowVersion() ExecutionFlowVersion {
	version := cloneTestTaskVersion(validTestTaskVersionPlan().Version)
	version.ID = " "
	version.ExecutionFlowID = " "
	version.VersionNumber = 0
	version.CreatedAt = 0
	version.FailurePolicy = "UNKNOWN"
	version.RequiredEnvironmentKeys = []string{" ", "region", "region"}
	first := version.Items[0]
	first.ID = " "
	first.SequenceNumber = 7
	first.FlowFragmentID = " "
	first.VersionPolicy = "UNKNOWN"
	second := version.Items[0]
	second.SequenceNumber = 9
	second.VersionPolicy = FlowFragmentVersionFixed
	second.WorkflowVersionID = ""
	version.Items = []ExecutionFlowItem{first, second}
	return version
}

func TestExecutionFlowVersionValidateOrdersViolationsDeterministically(t *testing.T) {
	want := []string{
		violationKey(fault.CodeFieldRequired, "id"),
		violationKey(fault.CodeFieldRequired, "executionFlowId"),
		violationKey(fault.CodeFieldInvalid, "versionNumber"),
		violationKey(fault.CodeFieldRequired, "createdAt"),
		violationKey(fault.CodeFieldInvalid, "failurePolicy"),
		violationKey(fault.CodeFieldRequired, "requiredEnvironmentKeys.0"),
		violationKey(fault.CodeFieldDuplicate, "requiredEnvironmentKeys.2"),
		violationKey(fault.CodeFieldRequired, "items.0.id"),
		violationKey(fault.CodeFieldMismatch, "items.0.executionFlowVersionId"),
		violationKey(fault.CodeFieldInvalid, "items.0.sequenceNumber"),
		violationKey(fault.CodeFieldRequired, "items.0.flowFragmentId"),
		violationKey(fault.CodeFieldInvalid, "items.0.versionPolicy"),
		violationKey(fault.CodeFieldMismatch, "items.1.executionFlowVersionId"),
		violationKey(fault.CodeFieldInvalid, "items.1.sequenceNumber"),
		violationKey(fault.CodeFieldRequired, "items.1.flowFragmentVersionId"),
	}
	version := brokenExecutionFlowVersion()
	descriptor := requireExecutionFlowEnvelope(t, version.Validate())
	got := violationKeys(descriptor.Violations())
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("violation order =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// A later map iteration slipping into the walk would show up as instability.
	for attempt := 0; attempt < 50; attempt++ {
		repeat := requireExecutionFlowEnvelope(t, brokenExecutionFlowVersion().Validate())
		if keys := violationKeys(repeat.Violations()); strings.Join(keys, "\n") != strings.Join(want, "\n") {
			t.Fatalf("violation order is unstable on attempt %d:\n%s", attempt, strings.Join(keys, "\n"))
		}
	}
}

func TestExecutionFlowValidateOrdersViolationsDeterministically(t *testing.T) {
	want := []string{
		violationKey(fault.CodeFieldRequired, "id"),
		violationKey(fault.CodeFieldRequired, "displayName"),
		violationKey(fault.CodeFieldRequired, "currentVersionId"),
		violationKey(fault.CodeFieldRequired, "createdAt"),
		violationKey(fault.CodeFieldRequired, "updatedAt"),
	}
	descriptor := requireExecutionFlowEnvelope(t, ExecutionFlow{}.Validate())
	if got := violationKeys(descriptor.Violations()); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("violation order =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestExecutionFlowVersionValidateTruncatesViolationsAtCap(t *testing.T) {
	version := cloneTestTaskVersion(validTestTaskVersionPlan().Version)
	version.ID = " "
	template := version.Items[0]
	items := make([]ExecutionFlowItem, 0, 12)
	for index := 0; index < 12; index++ {
		item := template
		item.ID = " "
		item.SequenceNumber = 0
		item.FlowFragmentID = " "
		item.VersionPolicy = "UNKNOWN"
		items = append(items, item)
	}
	version.Items = items

	first := requireExecutionFlowEnvelope(t, version.Validate())
	if len(first.Violations()) != fault.MaxViolations {
		t.Fatalf("violations = %d, want the cap %d", len(first.Violations()), fault.MaxViolations)
	}
	second := requireExecutionFlowEnvelope(t, version.Validate())
	if strings.Join(violationKeys(first.Violations()), "\n") != strings.Join(violationKeys(second.Violations()), "\n") {
		t.Fatal("truncated violation prefix is not deterministic")
	}
}

func TestExecutionFlowVersionValidateKeepsInputOutOfPublicText(t *testing.T) {
	const (
		secretItemID = "sentinel-item-8f21"
		secretOwner  = "sentinel-owner-8f21"
		secretEnvKey = "sentinel-env-8f21"
		secretPolicy = "SENTINEL_POLICY_8F21"
	)
	version := cloneTestTaskVersion(validTestTaskVersionPlan().Version)
	version.FailurePolicy = FailurePolicy(secretPolicy)
	version.RequiredEnvironmentKeys = []string{secretEnvKey, secretEnvKey}
	first := version.Items[0]
	first.ID = secretItemID
	first.TestTaskVersionID = secretOwner
	first.VersionPolicy = FlowFragmentVersionPolicy(secretPolicy)
	second := first
	second.SequenceNumber = 2
	version.Items = []ExecutionFlowItem{first, second}

	err := version.Validate()
	descriptor := requireExecutionFlowEnvelope(t, err)
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
	for _, secret := range []string{secretItemID, secretOwner, secretEnvKey, secretPolicy} {
		for _, text := range texts {
			if strings.Contains(text, secret) {
				t.Fatalf("public fault text %q leaks input %q", text, secret)
			}
		}
	}
}

func TestExecutionFlowAggregateValidatePropagatesVersionEnvelope(t *testing.T) {
	plan := validTestTaskVersionPlan()
	aggregate, err := NewExecutionFlow(plan.Task, plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Versions[0].Items[0].FlowFragmentID = " "
	requireExecutionFlowViolation(t, aggregate.Validate(), fault.CodeFieldRequired, "items.0.flowFragmentId")
}
