package automation

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestNodeAggregateValidate(t *testing.T) {
	aggregate := ElementTargetAggregate{
		ElementTarget: ElementTarget{ID: "node-1", DisplayName: "提交", Properties: Properties{}, CurrentVersionID: "version-1"},
		Current: ElementTargetVersion{
			ID: "version-1", ElementTargetID: "node-1", VersionNumber: 1,
			Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorTestID, Value: "submit"}},
			Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}},
			Source:      SourceManual,
		},
	}
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("expected valid aggregate: %v", err)
	}
	aggregate.ElementTarget.DisplayName = " "
	requireViolationOf(t, aggregate.Validate(), CodeElementTargetInvalid, fault.CodeFieldRequired, "displayName")
}

func TestEnvironmentValidateURL(t *testing.T) {
	environment := Environment{ID: "env-1", DisplayName: "测试", BaseURL: "javascript:alert(1)", Variables: EnvironmentVariables{}}
	if err := environment.Validate(); err == nil {
		t.Fatal("expected invalid URL")
	}
}
