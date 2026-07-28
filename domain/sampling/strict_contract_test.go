package sampling

import (
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestNewSessionURLContract(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{name: "empty", url: "", wantErr: "start URL"},
		{name: "whitespace", url: " \t\r\n", wantErr: "start URL"},
		{name: "malformed", url: "://bad", wantErr: "start URL"},
		{name: "relative", url: "path/to/page", wantErr: "start URL"},
		{name: "javascript scheme", url: "javascript:alert(1)", wantErr: "scheme"},
		{name: "file scheme", url: "file:///tmp/page.html", wantErr: "scheme"},
		{name: "http", url: "http://example.test"},
		{name: "https", url: "https://example.test/path?q=1"},
		{name: "missing http host", url: "http:///path", wantErr: "host"},
		{name: "missing https host", url: "https:/path", wantErr: "host"},
		{name: "control character", url: "https://example.test/\x00path", wantErr: "start URL"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session, err := NewSession("workflow", test.url)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("NewSession() error = %v", err)
				}
				if session.Snapshot().StartURL != test.url {
					t.Fatalf("StartURL = %q, want %q", session.Snapshot().StartURL, test.url)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("NewSession() error = %v, want containing %q", err, test.wantErr)
			}
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
	if err := session.Complete(); err == nil {
		t.Fatal("nil session Complete() succeeded")
	}
	session.Fail()
	if got := session.Snapshot(); !reflect.DeepEqual(got, Snapshot{}) {
		t.Fatalf("nil session Snapshot() = %+v", got)
	}
}

func TestDraftIndexBoundariesDoNotAllocateFromExtremeValues(t *testing.T) {
	workflow := draftFixture()
	step := workflow.Steps[0]
	for _, index := range []int{math.MinInt, -1, len(workflow.Steps) + 1, math.MaxInt} {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			if _, err := InsertDraftStep(workflow, StepContainer{}, index, step); err == nil {
				t.Fatalf("InsertDraftStep(index=%d) succeeded", index)
			}
		})
	}
}

func TestTemporaryWorkflowTimestampBoundaryValuesRemainLossless(t *testing.T) {
	values := []int64{-1, 0, 1, math.MaxInt64}
	for _, value := range values {
		workflow := TemporarySamplingWorkflow{StartedAt: value, PausedAt: value, EndedAt: value, InterruptedAt: value}
		cloned := cloneTemporaryWorkflow(workflow)
		if cloned.StartedAt != value || cloned.PausedAt != value || cloned.EndedAt != value || cloned.InterruptedAt != value {
			t.Fatalf("timestamp %d changed during clone: %+v", value, cloned)
		}
	}
}
