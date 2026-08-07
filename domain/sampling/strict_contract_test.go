package sampling

import (
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestNewSessionURLContract(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		wantViolation fault.Code
	}{
		{name: "empty", url: "", wantViolation: fault.CodeFieldRequired},
		{name: "whitespace", url: " \t\r\n", wantViolation: fault.CodeFieldRequired},
		{name: "malformed", url: "://bad", wantViolation: fault.CodeFieldInvalid},
		{name: "relative", url: "path/to/page", wantViolation: fault.CodeFieldInvalid},
		{name: "javascript scheme", url: "javascript:alert(1)", wantViolation: fault.CodeFieldInvalid},
		{name: "file scheme", url: "file:///tmp/page.html", wantViolation: fault.CodeFieldInvalid},
		{name: "http", url: "http://example.test"},
		{name: "https", url: "https://example.test/path?q=1"},
		{name: "missing http host", url: "http:///path", wantViolation: fault.CodeFieldRequired},
		{name: "missing https host", url: "https:/path", wantViolation: fault.CodeFieldRequired},
		{name: "control character", url: "https://example.test/\x00path", wantViolation: fault.CodeFieldInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session, err := NewSession("workflow", test.url)
			if test.wantViolation == "" {
				if err != nil {
					t.Fatalf("NewSession() error = %v", err)
				}
				if session.Snapshot().StartURL != test.url {
					t.Fatalf("StartURL = %q, want %q", session.Snapshot().StartURL, test.url)
				}
				return
			}
			requireViolation(t, err, CodeSessionInputInvalid, test.wantViolation, "startUrl")
			// url.Error formats the whole URL into its text; none of it may surface.
			requireNoPublicLeak(t, err, test.url, "alert(1)", "/tmp/page.html")
		})
	}
}

func TestSessionPublicMethodsAreNilSafe(t *testing.T) {
	var session *Session
	if got := session.ID(); got != "" {
		t.Fatalf("nil session ID = %q, want empty", got)
	}
	if _, err := session.Record(Capture{}); err == nil {
		t.Fatal("nil session Record() succeeded")
	}
	if err := session.End(); err == nil {
		t.Fatal("nil session End() succeeded")
	}
	session.Interrupt()
	if got := session.Snapshot(); !reflect.DeepEqual(got, Snapshot{}) {
		t.Fatalf("nil session Snapshot() = %+v", got)
	}
}

func TestDraftIndexBoundariesDoNotAllocateFromExtremeValues(t *testing.T) {
	workflow := draftFixture()
	step := workflow.Steps[0]
	for _, index := range []int{math.MinInt, -1, len(workflow.Steps) + 1, math.MaxInt} {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			if _, err := InsertUnpublishedFlowFragmentStep(workflow, FlowFragmentStepContainer{}, index, step); err == nil {
				t.Fatalf("InsertUnpublishedFlowFragmentStep(index=%d) succeeded", index)
			}
		})
	}
}

func TestTemporaryWorkflowTimestampBoundaryValuesRemainLossless(t *testing.T) {
	values := []int64{-1, 0, 1, math.MaxInt64}
	for _, value := range values {
		workflow := UnpublishedFlowFragment{StartedAt: value}
		cloned := cloneUnpublishedFlowFragment(workflow)
		if cloned.StartedAt != value {
			t.Fatalf("timestamp %d changed during clone: %+v", value, cloned)
		}
	}
}
