package automation

import (
	"errors"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// requireMultiViolationEnvelope is the general form of requireExecutionFlowEnvelope
// for the aggregate envelopes introduced in this package: element target, flow
// fragment, and environment. It does not assume a fixed Kind because
// AUTOMATION_ELEMENT_TARGET_INVALID, AUTOMATION_FLOW_FRAGMENT_INVALID, and
// AUTOMATION_ENVIRONMENT_INVALID are all INVALID_ARGUMENT, but a future caller
// of this helper should not have to assume that.
func requireMultiViolationEnvelope(t *testing.T, err error, wantCode fault.Code, wantKind fault.Kind) fault.Descriptor {
	t.Helper()
	if err == nil {
		t.Fatal("Validate() accepted invalid input")
	}
	if !fault.IsCode(err, wantCode) {
		t.Fatalf("Validate() error = %v, want code %s", err, wantCode)
	}
	descriptor, ok := fault.Describe(err)
	if !ok {
		t.Fatalf("Validate() error is not a fault: %v", err)
	}
	if descriptor.Kind() != wantKind {
		t.Fatalf("Validate() kind = %s, want %s", descriptor.Kind(), wantKind)
	}
	if len(descriptor.Violations()) == 0 {
		t.Fatal("aggregate envelope carries no violations")
	}
	if unwrapped := errors.Unwrap(err); unwrapped != nil {
		t.Fatalf("aggregate envelope nests another error: %v", unwrapped)
	}
	return descriptor
}

// brokenElementTargetAggregate breaks every independent ElementTargetAggregate
// rule at once so the whole violation order is observable in one envelope.
func brokenElementTargetAggregate() ElementTargetAggregate {
	return ElementTargetAggregate{
		ElementTarget: ElementTarget{ID: " ", DisplayName: " ", Properties: Properties{" ": "value"}, CurrentVersionID: "mismatched"},
		Current: ElementTargetVersion{
			ID: "", ElementTargetID: "other-target", VersionNumber: 0,
			Selectors:   nil,
			Fingerprint: fingerprint.Fingerprint{Tag: " ", Attributes: nil},
			Source:      "UNKNOWN",
		},
	}
}

func TestElementTargetAggregateValidateOrdersViolationsDeterministically(t *testing.T) {
	want := []string{
		violationKey(fault.CodeFieldRequired, "id"),
		violationKey(fault.CodeFieldRequired, "displayName"),
		violationKey(fault.CodeFieldInvalid, "properties"),
		violationKey(fault.CodeFieldRequired, "currentVersionId"),
		violationKey(fault.CodeFieldMismatch, "currentVersionId"),
		violationKey(fault.CodeFieldMismatch, "current.elementTargetId"),
		violationKey(fault.CodeFieldInvalid, "current.versionNumber"),
		violationKey(fault.CodeFieldRequired, "current.selectors"),
		violationKey(fault.CodeFieldRequired, "current.fingerprint.tag"),
		violationKey(fault.CodeFieldRequired, "current.fingerprint.attributes"),
		violationKey(fault.CodeFieldInvalid, "current.source"),
	}
	descriptor := requireMultiViolationEnvelope(t, brokenElementTargetAggregate().Validate(), CodeElementTargetInvalid, fault.InvalidArgument)
	got := violationKeys(descriptor.Violations())
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("violation order =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for attempt := 0; attempt < 50; attempt++ {
		repeat := requireMultiViolationEnvelope(t, brokenElementTargetAggregate().Validate(), CodeElementTargetInvalid, fault.InvalidArgument)
		if keys := violationKeys(repeat.Violations()); strings.Join(keys, "\n") != strings.Join(want, "\n") {
			t.Fatalf("violation order is unstable on attempt %d:\n%s", attempt, strings.Join(keys, "\n"))
		}
	}
}

func TestElementTargetAggregateValidateTruncatesViolationsAtCap(t *testing.T) {
	selectors := make([]fingerprint.Selector, 40)
	for index := range selectors {
		selectors[index] = fingerprint.Selector{Type: fingerprint.SelectorCSS}
	}
	aggregate := ElementTargetAggregate{
		ElementTarget: ElementTarget{ID: "target", DisplayName: "Target", Properties: Properties{}, CurrentVersionID: "v1"},
		Current: ElementTargetVersion{
			ID: "v1", ElementTargetID: "target", VersionNumber: 1,
			Selectors:   selectors,
			Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}},
			Source:      SourceManual,
		},
	}
	first := requireMultiViolationEnvelope(t, aggregate.Validate(), CodeElementTargetInvalid, fault.InvalidArgument)
	if len(first.Violations()) != fault.MaxViolations {
		t.Fatalf("violations = %d, want the cap %d", len(first.Violations()), fault.MaxViolations)
	}
	second := requireMultiViolationEnvelope(t, aggregate.Validate(), CodeElementTargetInvalid, fault.InvalidArgument)
	if strings.Join(violationKeys(first.Violations()), "\n") != strings.Join(violationKeys(second.Violations()), "\n") {
		t.Fatal("truncated violation prefix is not deterministic")
	}
}

// brokenFlowFragmentAggregate breaks several independent FlowFragmentAggregate
// rules at once, spanning metadata, the step tree, and parameter definitions.
func brokenFlowFragmentAggregate() FlowFragmentAggregate {
	return FlowFragmentAggregate{
		FlowFragment: FlowFragment{ID: " ", DisplayName: " ", Properties: Properties{" ": "value"}, CurrentVersionID: "mismatched"},
		Current: FlowFragmentVersion{
			ID: "", FlowFragmentID: "other-fragment", VersionNumber: 0,
			Definition: FlowFragmentContent{
				Steps: []FlowFragmentStep{{ID: " ", DisplayName: " ", Kind: StepKind("UNKNOWN")}},
				Parameters: []ParameterDefinition{
					{DisplayName: "Missing Name", Type: parameter.Text},
					{Name: "dup", DisplayName: "First", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue(""))},
					{Name: "dup", DisplayName: "Second", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue(""))},
				},
			},
		},
	}
}

func TestFlowFragmentAggregateValidateOrdersViolationsDeterministically(t *testing.T) {
	want := []string{
		violationKey(fault.CodeFieldRequired, "id"),
		violationKey(fault.CodeFieldRequired, "displayName"),
		violationKey(fault.CodeFieldInvalid, "properties"),
		violationKey(fault.CodeFieldMismatch, "current"),
		violationKey(fault.CodeFieldMismatch, "currentVersionId"),
		violationKey(fault.CodeFieldInvalid, "current.versionNumber"),
		violationKey(fault.CodeFieldRequired, "steps.identity"),
		violationKey(fault.CodeFieldInvalid, "steps.kind"),
		violationKey(fault.CodeFieldInvalid, "definition.parameters.0"),
		violationKey(fault.CodeFieldDuplicate, "definition.parameters"),
	}
	descriptor := requireMultiViolationEnvelope(t, brokenFlowFragmentAggregate().Validate(), CodeFlowFragmentInvalid, fault.InvalidArgument)
	got := violationKeys(descriptor.Violations())
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("violation order =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for attempt := 0; attempt < 50; attempt++ {
		repeat := requireMultiViolationEnvelope(t, brokenFlowFragmentAggregate().Validate(), CodeFlowFragmentInvalid, fault.InvalidArgument)
		if keys := violationKeys(repeat.Violations()); strings.Join(keys, "\n") != strings.Join(want, "\n") {
			t.Fatalf("violation order is unstable on attempt %d:\n%s", attempt, strings.Join(keys, "\n"))
		}
	}
}

func TestFlowFragmentAggregateValidateTruncatesViolationsAtCap(t *testing.T) {
	steps := make([]FlowFragmentStep, 40)
	for index := range steps {
		steps[index] = FlowFragmentStep{
			ID: string(rune('a'+index%26)) + string(rune('A'+index/26)), DisplayName: "Step",
			Kind: StepAction, Action: "bogus",
			ElementTargetID: "node", ElementTargetVersionID: "node-v1",
		}
	}
	aggregate := FlowFragmentAggregate{
		FlowFragment: FlowFragment{ID: "workflow", DisplayName: "Workflow", Properties: Properties{}, CurrentVersionID: "v1"},
		Current:      FlowFragmentVersion{ID: "v1", FlowFragmentID: "workflow", VersionNumber: 1, Definition: FlowFragmentContent{Steps: steps}},
	}
	first := requireMultiViolationEnvelope(t, aggregate.Validate(), CodeFlowFragmentInvalid, fault.InvalidArgument)
	if len(first.Violations()) != fault.MaxViolations {
		t.Fatalf("violations = %d, want the cap %d", len(first.Violations()), fault.MaxViolations)
	}
	second := requireMultiViolationEnvelope(t, aggregate.Validate(), CodeFlowFragmentInvalid, fault.InvalidArgument)
	if strings.Join(violationKeys(first.Violations()), "\n") != strings.Join(violationKeys(second.Violations()), "\n") {
		t.Fatal("truncated violation prefix is not deterministic")
	}
}

func brokenEnvironment() Environment {
	return Environment{ID: " ", DisplayName: " ", BaseURL: "/relative", Variables: EnvironmentVariables{" ": parameter.TextValue("value")}}
}

func TestEnvironmentValidateOrdersViolationsDeterministically(t *testing.T) {
	want := []string{
		violationKey(fault.CodeFieldRequired, "id"),
		violationKey(fault.CodeFieldRequired, "displayName"),
		violationKey(fault.CodeFieldInvalid, "baseUrl"),
		violationKey(fault.CodeFieldInvalid, "variables"),
	}
	descriptor := requireMultiViolationEnvelope(t, brokenEnvironment().Validate(), CodeEnvironmentInvalid, fault.InvalidArgument)
	got := violationKeys(descriptor.Violations())
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("violation order =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for attempt := 0; attempt < 50; attempt++ {
		repeat := requireMultiViolationEnvelope(t, brokenEnvironment().Validate(), CodeEnvironmentInvalid, fault.InvalidArgument)
		if keys := violationKeys(repeat.Violations()); strings.Join(keys, "\n") != strings.Join(want, "\n") {
			t.Fatalf("violation order is unstable on attempt %d:\n%s", attempt, strings.Join(keys, "\n"))
		}
	}
}

// TestAggregateEnvelopesKeepInputOutOfPublicText plants distinctive sentinel
// identities and property/variable names across all three envelopes and
// confirms none of them reach the public message, params, or violation text.
// These envelopes carry no cause at all (fault.New, not fault.Wrap), so the
// sentinels are simply absent rather than merely private.
func TestAggregateEnvelopesKeepInputOutOfPublicText(t *testing.T) {
	const (
		sentinelElementTargetID = "sentinel-element-target-7f3a"
		sentinelPropertyKey     = "sentinel-property-key-7f3a"
		sentinelFlowFragmentID  = "sentinel-flow-fragment-7f3a"
		sentinelStepID          = "sentinel-step-id-7f3a"
		sentinelEnvironmentID   = "sentinel-environment-id-7f3a"
		sentinelVariableName    = "sentinel-variable-name-7f3a"
	)

	elementTarget := ElementTargetAggregate{
		ElementTarget: ElementTarget{ID: sentinelElementTargetID, DisplayName: "Target", Properties: Properties{sentinelPropertyKey: "value", " ": "value"}, CurrentVersionID: "v1"},
		Current: ElementTargetVersion{
			ID: "v1", ElementTargetID: sentinelElementTargetID, VersionNumber: 0,
			Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}, Source: SourceManual,
		},
	}
	flowFragment := FlowFragmentAggregate{
		FlowFragment: FlowFragment{ID: sentinelFlowFragmentID, DisplayName: "Fragment", Properties: Properties{}, CurrentVersionID: "v1"},
		Current: FlowFragmentVersion{ID: "v1", FlowFragmentID: sentinelFlowFragmentID, VersionNumber: 1,
			Definition: FlowFragmentContent{Steps: []FlowFragmentStep{{ID: sentinelStepID, DisplayName: "Step", Kind: StepAction, Action: "bogus"}}}},
	}
	environment := Environment{ID: sentinelEnvironmentID, DisplayName: "Env", Variables: EnvironmentVariables{sentinelVariableName: parameter.TextValue("value"), " ": parameter.TextValue("value")}}

	cases := []struct {
		name string
		err  error
	}{
		{"element target", elementTarget.Validate()},
		{"flow fragment", flowFragment.Validate()},
		{"environment", environment.Validate()},
	}
	sentinels := []string{sentinelElementTargetID, sentinelPropertyKey, sentinelFlowFragmentID, sentinelStepID, sentinelEnvironmentID, sentinelVariableName}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.err
			if err == nil {
				t.Fatal("Validate() accepted sentinel-carrying input")
			}
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
			for _, sentinel := range sentinels {
				for _, text := range texts {
					if strings.Contains(text, sentinel) {
						t.Fatalf("public fault text %q leaks input %q", text, sentinel)
					}
				}
			}
		})
	}
}

// TestHistoryAndTransitionEnvelopesKeepInputOutOfPublicText covers the
// single-violation short circuits: AUTOMATION_ELEMENT_TARGET_HISTORY_INVALID,
// AUTOMATION_FLOW_FRAGMENT_HISTORY_INVALID, and
// AUTOMATION_AGGREGATE_TRANSITION_INVALID. No version identity or timestamp
// value reaches public text.
func TestHistoryAndTransitionEnvelopesKeepInputOutOfPublicText(t *testing.T) {
	const sentinelVersionID = "sentinel-version-id-4c9e"

	elementTargetHistory := ElementTargetAggregate{
		ElementTarget: ElementTarget{ID: "target", DisplayName: "Target", Properties: Properties{}, CurrentVersionID: "v1"},
		Current: ElementTargetVersion{
			ID: "v1", ElementTargetID: "target", VersionNumber: 1,
			Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#submit"}},
			Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}},
			Source:      SourceManual,
		},
		// The current version ("v1") is absent from this history, which is
		// exactly the failure this test wants: the only listed version carries
		// the sentinel identity, and it must not surface in public text.
		Versions: []ElementTargetVersion{{ID: sentinelVersionID, ElementTargetID: "target", VersionNumber: 1}},
	}
	transition := func() error {
		_, err := (Environment{ID: "env", DisplayName: "Env", CreatedAt: 1, UpdatedAt: 1}).Delete(-1)
		return err
	}

	cases := []struct {
		name string
		err  error
	}{
		{"element target history", elementTargetHistory.ValidateLoadedHistory()},
		{"aggregate transition", transition()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.err
			if err == nil {
				t.Fatal("Validate() accepted invalid input")
			}
			descriptor, ok := fault.Describe(err)
			if !ok {
				t.Fatalf("error is not a fault: %v", err)
			}
			texts := []string{err.Error(), descriptor.Message()}
			for _, violation := range descriptor.Violations() {
				texts = append(texts, violation.Field(), violation.Message())
			}
			for _, text := range texts {
				if strings.Contains(text, sentinelVersionID) {
					t.Fatalf("public fault text %q leaks input %q", text, sentinelVersionID)
				}
			}
		})
	}
}
