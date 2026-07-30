package node

import (
	"regexp"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestNodeFaultCodesAreStableAndWellFormed(t *testing.T) {
	codes := []fault.Code{
		CodeElementNotFound,
		CodeTimeout,
		CodeCanceled,
		CodeTransientDriver,
		CodeOperationFailed,
	}
	pattern := regexp.MustCompile(`^NODE_[A-Z][A-Z0-9_]{2,62}$`)
	seen := make(map[fault.Code]struct{}, len(codes))
	for _, code := range codes {
		if !pattern.MatchString(string(code)) {
			t.Fatalf("invalid node fault code %q", code)
		}
		if _, duplicate := seen[code]; duplicate {
			t.Fatalf("duplicate node fault code %q", code)
		}
		seen[code] = struct{}{}
	}
}
