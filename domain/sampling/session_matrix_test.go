package sampling

import (
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestNewSessionRejectsMissingBusinessIdentity(t *testing.T) {
	tests := []struct {
		name       string
		workflowID string
		startURL   string
		wantField  string
	}{
		{name: "empty workflow", startURL: "https://example.test", wantField: "flowFragmentId"},
		{name: "blank workflow", workflowID: "  ", startURL: "https://example.test", wantField: "flowFragmentId"},
		{name: "empty start URL", workflowID: "flow", wantField: "startUrl"},
		{name: "blank start URL", workflowID: "flow", startURL: "\t", wantField: "startUrl"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewSession(test.workflowID, test.startURL)
			requireViolation(t, err, CodeSessionInputInvalid, fault.CodeFieldRequired, test.wantField)
			requireNoPublicLeak(t, err, "https://example.test")
		})
	}
}

func TestSessionLifecycleTransitionMatrix(t *testing.T) {
	type operation struct {
		name string
		run  func(*Session) error
	}
	operations := []operation{
		{name: "start", run: func(s *Session) error { return s.Start() }},
		{name: "pause", run: func(s *Session) error { return s.Pause() }},
		{name: "resume", run: func(s *Session) error { return s.Resume() }},
		{name: "end", run: func(s *Session) error { return s.End() }},
	}
	allowed := map[string]map[Status]Status{
		"start":  {StatusCreated: StatusRecording},
		"pause":  {StatusRecording: StatusPaused},
		"resume": {StatusPaused: StatusRecording},
		"end":    {StatusRecording: StatusEnded, StatusPaused: StatusEnded},
	}
	statuses := []Status{StatusCreated, StatusRecording, StatusPaused, StatusEnded, StatusInterrupted, "unknown"}
	for _, operation := range operations {
		for _, status := range statuses {
			t.Run(operation.name+"/"+string(status), func(t *testing.T) {
				session := &Session{status: status, startURL: "https://example.test"}
				err := operation.run(session)
				want, ok := allowed[operation.name][status]
				if !ok {
					if err == nil || session.status != status {
						t.Fatalf("invalid transition changed state: status=%q err=%v", session.status, err)
					}
					return
				}
				if err != nil || session.status != want {
					t.Fatalf("valid transition: status=%q want=%q err=%v", session.status, want, err)
				}
			})
		}
	}
	var nilSession *Session
	for _, operation := range operations {
		t.Run(operation.name+"/nil", func(t *testing.T) {
			// A nil receiver is a caller code defect with no runtime remediation,
			// so it is INTERNAL rather than a lifecycle precondition failure.
			requireEnvelope(t, operation.run(nilSession), CodeInternal)
		})
	}
}

func TestSessionPauseResumePreservesIdentityAndSequence(t *testing.T) {
	session, err := NewSession("flow", "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(); err != nil {
		t.Fatal(err)
	}
	first, err := session.Record(validCapture("capture-1", "submit", ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Pause(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Record(validCapture("while-paused", "submit", "")); err == nil {
		t.Fatal("paused session accepted a capture")
	}
	if err := session.Resume(); err != nil {
		t.Fatal(err)
	}
	second, err := session.Record(validCapture("capture-2", "submit", ""))
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || second.NodeUUID != first.NodeUUID || second.ElementTargetID != first.ElementTargetID || second.Sequence != first.Sequence+1 {
		t.Fatalf("identity/sequence changed across pause: first=%+v second=%+v", first, second)
	}
}

func TestSessionInterruptIsTerminalAndIdempotent(t *testing.T) {
	for _, initial := range []Status{StatusCreated, StatusRecording, StatusPaused} {
		t.Run("interrupt/"+string(initial), func(t *testing.T) {
			session := &Session{status: initial}
			session.Interrupt()
			session.Interrupt()
			if session.status != StatusInterrupted {
				t.Fatalf("status = %q, want %q", session.status, StatusInterrupted)
			}
		})
	}
	for _, terminal := range []Status{StatusEnded, StatusInterrupted} {
		session := &Session{status: terminal}
		session.Interrupt()
		if session.status != terminal {
			t.Fatalf("terminal status %q changed to %q", terminal, session.status)
		}
	}
	var nilSession *Session
	nilSession.Interrupt()
}

func TestSessionRecordActionContractMatrix(t *testing.T) {
	tests := []struct {
		name          string
		capture       Capture
		wantViolation fault.Code
		wantField     string
	}{
		{name: "click", capture: validCapture("click", "button", "")},
		{name: "input", capture: func() Capture { c := validCapture("input", "field", "alice"); c.Kind = ActionInput; return c }()},
		{name: "select", capture: func() Capture {
			c := validCapture("select", "region", "east")
			c.Kind = ActionSelect
			c.Values = []string{"east", "west"}
			return c
		}()},
		{name: "validation", capture: func() Capture {
			c := validCapture("validation", "status", "")
			c.Kind = ActionValidate
			c.Validation = &ValidationSample{Kind: "text_equals", Expected: "ok"}
			return c
		}()},
		{name: "press", capture: Capture{CaptureID: "press", Kind: ActionPress, Value: "Enter", PageURL: "https://example.test"}},
		{name: "missing capture id", capture: Capture{Kind: ActionPress, Value: "Enter"}, wantViolation: fault.CodeFieldRequired, wantField: "captureId"},
		{name: "missing identity", capture: Capture{CaptureID: "missing-identity", Kind: ActionClick}, wantViolation: fault.CodeFieldRequired, wantField: "identityKey"},
		{name: "missing validation", capture: func() Capture {
			c := validCapture("missing-validation", "status", "")
			c.Kind = ActionValidate
			return c
		}(), wantViolation: fault.CodeFieldRequired, wantField: "validation"},
		{name: "missing press value", capture: Capture{CaptureID: "press-empty", Kind: ActionPress}, wantViolation: fault.CodeFieldRequired, wantField: "value"},
		{name: "unsupported navigate", capture: Capture{CaptureID: "navigate", Kind: ActionNavigate}, wantViolation: fault.CodeFieldInvalid, wantField: "kind"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session, err := NewSession("flow", "https://example.test")
			if err != nil {
				t.Fatal(err)
			}
			if err := session.Start(); err != nil {
				t.Fatal(err)
			}
			_, err = session.Record(test.capture)
			if test.wantField == "" {
				if err != nil {
					t.Fatalf("valid capture rejected: %v", err)
				}
				return
			}
			requireViolation(t, err, CodeCaptureInvalid, test.wantViolation, test.wantField)
			requireNoPublicLeak(t, err, "Enter", string(ActionNavigate))
		})
	}
}

func TestSessionCaptureIDMakesRetriesIdempotentAcrossPayloadChanges(t *testing.T) {
	session, err := NewSession("flow", "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(); err != nil {
		t.Fatal(err)
	}
	first, err := session.Record(validCapture("capture", "first", ""))
	if err != nil {
		t.Fatal(err)
	}
	retry := validCapture("capture", "different", "changed")
	retry.Kind = ActionInput
	retry.Spec.Selectors[0].Value = "#different"
	second, err := session.Record(retry)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("retry result = %+v, want original %+v", second, first)
	}
	snapshot := session.Snapshot()
	if len(snapshot.Nodes) != 1 || len(snapshot.Actions) != 2 || snapshot.Actions[1].Kind != ActionClick ||
		snapshot.Nodes[0].Spec.Selectors[0].Value != "#submit" {
		t.Fatalf("retry changed captured state: %+v", snapshot)
	}
}

func TestSessionRejectsInvalidNewAndUpdatedNodeSpecifications(t *testing.T) {
	session, err := NewSession("flow", "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(); err != nil {
		t.Fatal(err)
	}
	invalid := validCapture("invalid-new", "invalid-new", "")
	invalid.Spec.Selectors = nil
	// The spec's own fault passes through unwrapped: nesting it inside a sampling
	// code would force the host to unwrap recursively before classifying.
	if _, err := session.Record(invalid); !fault.IsCode(err, fingerprint.CodeElementTargetSpecInvalid) {
		t.Fatalf("invalid new node error = %v, want code %s", err, fingerprint.CodeElementTargetSpecInvalid)
	}
	first, err := session.Record(validCapture("valid", "stable", ""))
	if err != nil {
		t.Fatal(err)
	}
	update := validCapture("invalid-update", "changed-identity", "")
	update.NodeUUID = first.NodeUUID
	update.Spec.Fingerprint.Tag = ""
	if _, err := session.Record(update); !fault.IsCode(err, fingerprint.CodeElementTargetSpecInvalid) {
		t.Fatalf("invalid update error = %v, want code %s", err, fingerprint.CodeElementTargetSpecInvalid)
	}
	snapshot := session.Snapshot()
	if len(snapshot.Nodes) != 1 || snapshot.Nodes[0].Spec.Fingerprint.Tag != "button" {
		t.Fatalf("invalid update changed stored node: %+v", snapshot.Nodes)
	}
}

func TestSessionIdentityAccessorsAndOriginNormalization(t *testing.T) {
	session, err := NewSession("flow", "https://example.test/start")
	if err != nil {
		t.Fatal(err)
	}
	if session.ID() == "" || session.Snapshot().ID != session.ID() {
		t.Fatalf("session identity is inconsistent: ID=%q snapshot=%+v", session.ID(), session.Snapshot())
	}
	var nilSession *Session
	if snapshot := nilSession.Snapshot(); !reflect.DeepEqual(snapshot, Snapshot{}) {
		t.Fatalf("nil session snapshot = %+v, want zero value", snapshot)
	}
	for _, test := range []struct {
		url  string
		want string
	}{
		{url: "https://example.test:8443/path?q=1", want: "https://example.test:8443"},
		{url: "file:///tmp/test.html"},
		{url: "relative/path"},
		{url: "://bad"},
	} {
		if got := originOf(test.url); got != test.want {
			t.Fatalf("originOf(%q) = %q, want %q", test.url, got, test.want)
		}
	}
}

func TestSessionSnapshotOwnsNestedData(t *testing.T) {
	session, err := NewSession("flow", "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(); err != nil {
		t.Fatal(err)
	}
	capture := validCapture("capture", "field", "value")
	capture.Kind = ActionValidate
	capture.Values = []string{"one", "two"}
	capture.Validation = &ValidationSample{Kind: "selected_set_equals", Expected: "one", SupportedKinds: []string{"selected_set_equals"}}
	capture.Spec.Selectors = []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#original"}}
	capture.Spec.Fingerprint.Attributes = map[string]string{"name": "original"}
	capture.Spec.Fingerprint.Path = []string{"html", "body"}
	if _, err := session.Record(capture); err != nil {
		t.Fatal(err)
	}
	capture.Values[0] = "mutated"
	capture.Validation.SupportedKinds[0] = "mutated"
	capture.Spec.Selectors[0].Value = "#mutated"
	capture.Spec.Fingerprint.Attributes["name"] = "mutated"
	capture.Spec.Fingerprint.Path[0] = "mutated"

	first := session.Snapshot()
	first.Actions[1].Values[0] = "snapshot-mutated"
	first.Actions[1].Validation.SupportedKinds[0] = "snapshot-mutated"
	first.Nodes[0].Spec.Selectors[0].Value = "#snapshot-mutated"
	first.Nodes[0].Spec.Fingerprint.Attributes["name"] = "snapshot-mutated"
	first.Nodes[0].Spec.Fingerprint.Path[0] = "snapshot-mutated"
	second := session.Snapshot()
	if got := second.Actions[1].Values; !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("action values aliased: %v", got)
	}
	if second.Actions[1].Validation.SupportedKinds[0] != "selected_set_equals" || second.Nodes[0].Spec.Selectors[0].Value != "#original" ||
		second.Nodes[0].Spec.Fingerprint.Attributes["name"] != "original" || second.Nodes[0].Spec.Fingerprint.Path[0] != "html" {
		t.Fatalf("snapshot nested data aliased: %+v", second)
	}
}
