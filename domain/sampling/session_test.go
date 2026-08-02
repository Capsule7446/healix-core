package sampling

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func validCapture(id, identity, value string) Capture {
	return Capture{
		CaptureID: id, IdentityKey: identity, PageURL: "https://example.test/login",
		Kind: ActionClick, Value: value,
		Spec: fingerprint.ElementTargetSpec{
			Role:        "button",
			Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#submit"}},
			Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}},
		},
	}
}

func TestSessionAssignsStableNodeUUIDAndIdempotentCapture(t *testing.T) {
	session, err := NewSession("login-flow", "https://example.test/login")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(); err != nil {
		t.Fatal(err)
	}
	first, err := session.Record(validCapture("capture-1", "login|css:#submit", ""))
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || first.NodeUUID == "" || first.ElementTargetID == "" || first.Sequence != 2 {
		t.Fatalf("first result = %+v", first)
	}
	repeated, err := session.Record(validCapture("capture-1", "login|css:#submit", ""))
	if err != nil {
		t.Fatal(err)
	}
	if repeated != first || len(session.Snapshot().Actions) != 2 {
		t.Fatalf("repeated capture = %+v, actions=%d", repeated, len(session.Snapshot().Actions))
	}
	updatedCapture := validCapture("capture-2", "login|css:#submit-v2", "")
	updatedCapture.NodeUUID = first.NodeUUID
	updatedCapture.Spec.Selectors[0].Value = "#submit-v2"
	updated, err := session.Record(updatedCapture)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Created || updated.NodeUUID != first.NodeUUID || updated.ElementTargetID != first.ElementTargetID {
		t.Fatalf("updated result = %+v, first=%+v", updated, first)
	}
	snapshot := session.Snapshot()
	if len(snapshot.Nodes) != 1 || len(snapshot.Actions) != 3 || snapshot.Nodes[0].Spec.Selectors[0].Value != "#submit-v2" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot.Nodes[0].Spec.PageURL != "https://example.test/login" || snapshot.Nodes[0].Spec.Origin != "https://example.test" {
		t.Fatalf("node page context = url %q origin %q", snapshot.Nodes[0].Spec.PageURL, snapshot.Nodes[0].Spec.Origin)
	}
}

func TestNewUUIDUsesVersion7(t *testing.T) {
	id, err := NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 36 || id[14] != '7' || (id[19] != '8' && id[19] != '9' && id[19] != 'a' && id[19] != 'b') {
		t.Fatalf("NewUUID() = %q, want RFC 9562 UUIDv7", id)
	}
}

func TestSessionRejectsCaptureAfterCompletion(t *testing.T) {
	session, err := NewSession("flow", "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(); err != nil {
		t.Fatal(err)
	}
	if err := session.Complete(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Record(validCapture("capture", "key", "")); err == nil {
		t.Fatal("capture after completion unexpectedly succeeded")
	}
}

func TestNewUUIDFormatAndUniqueness(t *testing.T) {
	first, err := NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 36 || first == second || first[14] != '7' {
		t.Fatalf("UUIDs = %q, %q", first, second)
	}
}
