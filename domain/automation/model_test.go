package automation

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestNodeAggregateValidate(t *testing.T) {
	aggregate := NodeAggregate{
		Node: Node{ID: "node-1", DisplayName: "提交", Properties: Properties{}, CurrentVersionID: "version-1"},
		Current: NodeVersion{
			ID: "version-1", NodeID: "node-1", VersionNumber: 1,
			Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorTestID, Value: "submit"}},
			Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}},
			Source:      SourceManual,
		},
	}
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("expected valid aggregate: %v", err)
	}
	aggregate.Node.DisplayName = " "
	if err := aggregate.Validate(); err == nil || !strings.Contains(err.Error(), "display name") {
		t.Fatalf("expected display name error, got %v", err)
	}
}

func TestEnvironmentValidateURL(t *testing.T) {
	environment := Environment{ID: "env-1", DisplayName: "测试", BaseURL: "javascript:alert(1)", Properties: Properties{}}
	if err := environment.Validate(); err == nil {
		t.Fatal("expected invalid URL")
	}
}
