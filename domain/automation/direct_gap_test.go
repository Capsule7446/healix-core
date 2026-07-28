package automation

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestParameterDefinitionValidateValueDirect(t *testing.T) {
	large := strings.Repeat("x", parameter.MaxValueStringBytes+1)
	tests := []struct {
		name       string
		definition ParameterDefinition
		value      parameter.Value
		wantErr    bool
	}{
		{"text", ParameterDefinition{Type: parameter.Text}, parameter.TextValue("你好🙂'"), false},
		{"type mismatch", ParameterDefinition{Type: parameter.Boolean}, parameter.TextValue("true"), true},
		{"invalid value", ParameterDefinition{Type: parameter.Text}, parameter.TextValue(large), true},
		{"single allowed", ParameterDefinition{Type: parameter.SingleSelect, Options: []string{"a"}}, parameter.SingleSelectValue("a"), false},
		{"single unknown", ParameterDefinition{Type: parameter.SingleSelect, Options: []string{"a"}}, parameter.SingleSelectValue("b"), true},
		{"multi empty", ParameterDefinition{Type: parameter.MultiSelect, Options: []string{"a"}}, parameter.MultiSelectValue(nil), false},
		{"multi unknown", ParameterDefinition{Type: parameter.MultiSelect, Options: []string{"a"}}, parameter.MultiSelectValue([]string{"b"}), true},
		{"multi duplicate", ParameterDefinition{Type: parameter.MultiSelect, Options: []string{"a"}}, parameter.MultiSelectValue([]string{"a", "a"}), true},
		{"zero value", ParameterDefinition{Type: parameter.Text}, parameter.Value{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.definition.ValidateValue(tt.value); (got != nil) != tt.wantErr {
				t.Fatalf("ValidateValue() error = %v, wantErr %v", got, tt.wantErr)
			}
		})
	}
}

func TestNormalizePoliciesAndFailurePolicyDirect(t *testing.T) {
	defaultHealer := DefaultHealerPolicySnapshotV1()
	explicit := HealerPolicySnapshotV1{Version: 1, ReviewCap: 0, AppliedCap: 0, Weights: HealerWeightsSnapshotV1{}}
	if got := NormalizeHealerPolicySnapshotV1(HealerPolicySnapshotV1{}); !reflect.DeepEqual(got, defaultHealer) {
		t.Fatalf("zero healer = %#v", got)
	}
	if got := NormalizeHealerPolicySnapshotV1(explicit); !reflect.DeepEqual(got, explicit) {
		t.Fatalf("explicit healer changed: %#v", got)
	}

	for _, tt := range []struct {
		name string
		in   ScreenshotPolicy
		want ScreenshotPolicy
	}{
		{"zero", ScreenshotPolicy{}, ScreenshotPolicy{}},
		{"trim destination", ScreenshotPolicy{Enabled: true, Destination: "  artifacts/截图  "}, ScreenshotPolicy{Enabled: true, Destination: "artifacts/截图"}},
		{"explicit disabled", ScreenshotPolicy{Enabled: false, Destination: ""}, ScreenshotPolicy{Enabled: false, Destination: ""}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeScreenshotPolicy(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v want %#v", got, tt.want)
			}
		})
	}

	for _, tt := range []struct {
		policy FailurePolicy
		valid  bool
	}{{FailurePolicyStopOnFailure, true}, {FailurePolicyContinueOnFailure, true}, {FailurePolicy(""), false}, {FailurePolicy("STOP"), false}} {
		if got := tt.policy.IsValid(); got != tt.valid {
			t.Errorf("%q.IsValid() = %v", tt.policy, got)
		}
	}
}
