package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/node"
)

// The origin guard only means something if it is reachable from the entry a
// host actually calls. A unit test that hand-builds a node.Runtime with an
// Origin already set proves the comparison works while saying nothing about
// whether anything ever supplies that Origin in production — which is exactly
// how this guard came to be dead code. Every test here goes through the real
// CompilePlan -> RunProgram pair for that reason.

const (
	originGuardTrustedOrigin = "https://trusted.test"
	originGuardTrustedPage   = "https://trusted.test/checkout"
	originGuardEvilOrigin    = "https://evil.test"
	originGuardEvilPage      = "https://evil.test/checkout"
	originGuardStaleSelector = "submit"
	originGuardHealSelector  = "evil-submit"
)

type originGuardProbe struct {
	clicks   int
	inputs   int
	locates  int
	snapshot int
	heals    int
	located  int
}

type originGuardElement struct{ probe *originGuardProbe }

func (e originGuardElement) Exists(context.Context) (bool, error)  { return true, nil }
func (e originGuardElement) Visible(context.Context) (bool, error) { return true, nil }
func (e originGuardElement) Text(context.Context) (string, error)  { return "", nil }
func (e originGuardElement) Attribute(context.Context, string) (string, bool, error) {
	return "", false, nil
}
func (e originGuardElement) Click(context.Context) error {
	e.probe.clicks++
	return nil
}
func (e originGuardElement) Input(_ context.Context, _ string) error {
	e.probe.inputs++
	return nil
}
func (e originGuardElement) Select(context.Context, string, ...string) error { return nil }
func (e originGuardElement) Hover(context.Context) error                     { return nil }
func (e originGuardElement) WaitStable(context.Context) error                { return nil }

type originGuardSnapshot struct{ candidates []heal.SnapshotCandidate }

func (s originGuardSnapshot) Candidates(context.Context) ([]heal.SnapshotCandidate, error) {
	return s.candidates, nil
}

// originGuardDriver refuses the recorded selector and accepts only the healed
// one, so a click can happen if and only if the heal was applied.
type originGuardDriver struct{ probe *originGuardProbe }

func (originGuardDriver) Navigate(context.Context, string) error { return nil }
func (originGuardDriver) Press(context.Context, string) error    { return nil }
func (d originGuardDriver) Locate(_ context.Context, spec fingerprint.ElementTargetSpec) (node.Element, error) {
	d.probe.locates++
	for _, selector := range spec.Selectors {
		if selector.Value == originGuardHealSelector {
			d.probe.located++
			return originGuardElement{probe: d.probe}, nil
		}
	}
	return nil, node.NewElementNotFoundError()
}
func (d originGuardDriver) Snapshot(context.Context) (heal.DOMSnapshot, error) {
	d.probe.snapshot++
	return originGuardSnapshot{candidates: []heal.SnapshotCandidate{{
		Selector:    fingerprint.Selector{Type: fingerprint.SelectorTestID, Value: originGuardHealSelector},
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}},
	}}}, nil
}
func (originGuardDriver) WaitNetworkIdle(context.Context) error { return nil }

// originGuardHealer is the hostile case: a perfect-looking candidate. Only the
// origin comparison stands between it and a click.
type originGuardHealer struct{ probe *originGuardProbe }

func (h originGuardHealer) Heal(_ context.Context, _ fingerprint.ElementTargetSpec, _ heal.DOMSnapshot) (heal.Decision, error) {
	h.probe.heals++
	candidate := heal.Candidate{
		Selector:    fingerprint.Selector{Type: fingerprint.SelectorTestID, Value: originGuardHealSelector},
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}},
		Score:       0.99,
	}
	return heal.Decision{Outcome: heal.OutcomeApplied, Best: &candidate, Candidates: []heal.Candidate{candidate}}, nil
}

type originGuardLocator struct {
	location node.PageLocation
	err      error
	calls    int
}

func (l *originGuardLocator) CurrentLocation(context.Context) (node.PageLocation, error) {
	l.calls++
	return l.location, l.err
}

// originGuardSink keeps the staged decision so a test can tell a Block from a
// Review. Both surface as CodeHealingRefused, but only a Block rewrites the
// outcome to SafetyRejected — asserting on the code alone would let a
// page-level Review stand in for the origin-level refusal under test.
type originGuardSink struct{ decisions []heal.Decision }

func (s *originGuardSink) RecordProgress(context.Context, domainexecution.WorkerFence, node.Event) error {
	return nil
}
func (s *originGuardSink) StageHealDecision(_ context.Context, _ domainexecution.WorkerFence, _, _ string, _ fingerprint.Selector, decision heal.Decision) error {
	s.decisions = append(s.decisions, decision)
	return nil
}
func (s *originGuardSink) StageValidationObservation(context.Context, domainexecution.WorkerFence, node.ValidationObservation) error {
	return nil
}
func (s *originGuardSink) StageValidationGroupTerminal(context.Context, domainexecution.WorkerFence, node.ValidationGroupTerminalObservation) error {
	return nil
}
func (s *originGuardSink) CommitTerminal(context.Context, domainexecution.WorkerFence, node.TerminalCommit) error {
	return nil
}

type originGuardAuthority struct{}

func (originGuardAuthority) VerifyExecutionAuthority(context.Context, ExecutionAuthority) error {
	return nil
}

// crossOriginHealEntry compiles a real plan whose only step clicks an element
// captured on originGuardTrustedOrigin.
func crossOriginHealEntry(t *testing.T) CompiledEntry {
	t.Helper()
	draft := minimalCompilerPlan()
	draft.Workflows[0].Steps = []domainexecution.Step{{
		ID: "click", DisplayName: "Click", Kind: domainexecution.ActionStep, Action: "click",
		ElementTargetID: compilerNodeID, ElementTargetVersionID: compilerNodeV1,
	}}
	dependency := compilerNodeSnapshot(compilerNodeV1, originGuardStaleSelector)
	dependency.PageURL = originGuardTrustedPage
	dependency.Origin = originGuardTrustedOrigin
	draft.Nodes = []domainexecution.NodeSnapshot{dependency}

	snapshot, err := instanceSnapshotForCompilerTest(draft, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := compiled.Entry(mustEntryID("execution-entry"))
	if !ok {
		t.Fatal("execution-entry is missing")
	}
	return entry
}

func originGuardConfig(entry CompiledEntry, probe *originGuardProbe, locator node.PageLocator, sink node.ExecutionSink) Config {
	return Config{
		InstanceID: entry.InstanceID, SnapshotDigest: entry.SnapshotDigest, EntryID: entry.EntryID,
		ClaimToken: "claim", AuthorityVerifier: originGuardAuthority{},
		Driver: originGuardDriver{probe: probe}, Healer: originGuardHealer{probe: probe},
		Facts: sink, PageLocator: locator,
	}
}

func TestRunProgramRefusesHealAcrossAnOriginBoundary(t *testing.T) {
	tests := []struct {
		name       string
		locator    *originGuardLocator
		wantReason string
	}{
		{
			name:       "redirected onto another origin",
			locator:    &originGuardLocator{location: node.PageLocation{URL: originGuardEvilPage, Origin: originGuardEvilOrigin}},
			wantReason: string(heal.ReasonOriginMismatch),
		},
		{
			// Fail-closed: not knowing where the browser is cannot be softer
			// than knowing it is somewhere wrong.
			name:       "location unavailable",
			locator:    &originGuardLocator{err: errors.New("page closed")},
			wantReason: string(heal.ReasonOriginUnknown),
		},
		{
			name:       "location reported blank",
			locator:    &originGuardLocator{location: node.PageLocation{}},
			wantReason: string(heal.ReasonOriginUnknown),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry := crossOriginHealEntry(t)
			probe := &originGuardProbe{}
			sink := &originGuardSink{}

			_, err := RunProgram(context.Background(), entry, originGuardConfig(entry, probe, test.locator, sink))

			if !fault.IsCode(err, node.CodeHealingRefused) {
				t.Fatalf("RunProgram() error = %v, want %q", err, node.CodeHealingRefused)
			}
			if probe.clicks != 0 || probe.inputs != 0 {
				t.Fatalf("refused heal still interacted: %d clicks, %d inputs", probe.clicks, probe.inputs)
			}
			if probe.located != 0 {
				t.Fatalf("refused heal still resolved the healed selector %d times", probe.located)
			}
			if test.locator.calls == 0 {
				t.Fatal("the live location was never consulted")
			}
			// The origin refusal must be the reason, not a side effect of the
			// weaker page check also failing.
			cause := privateCause(t, err)
			if !strings.Contains(cause, test.wantReason) {
				t.Fatalf("private cause = %q, want it to carry %q", cause, test.wantReason)
			}
			// SafetyRejected is written only on a Block. A Review would reach
			// the same error code while leaving the decision applied.
			if len(sink.decisions) != 1 || sink.decisions[0].Outcome != heal.OutcomeSafetyRejected {
				t.Fatalf("staged decisions = %+v, want exactly one SafetyRejected", sink.decisions)
			}
			if descriptor, ok := fault.Describe(err); ok && leaksAny(descriptor.Message(), originGuardEvilOrigin, originGuardTrustedOrigin, originGuardHealSelector, test.wantReason) {
				t.Fatalf("public message leaked page, selector, or reason detail: %q", descriptor.Message())
			}
		})
	}
}

func privateCause(t *testing.T, err error) string {
	t.Helper()
	var classified *fault.Error
	if !errors.As(err, &classified) {
		t.Fatalf("no classified fault in the chain: %v", err)
	}
	cause := errors.Unwrap(classified)
	if cause == nil {
		t.Fatalf("classified fault %v carries no private cause", classified)
	}
	return cause.Error()
}

// TestRunProgramAllowsHealOnTheRecordedOrigin is the other half of the guard:
// a fail-closed rule that also refuses the legitimate case is just an outage.
func TestRunProgramAllowsHealOnTheRecordedOrigin(t *testing.T) {
	entry := crossOriginHealEntry(t)
	probe := &originGuardProbe{}
	locator := &originGuardLocator{location: node.PageLocation{URL: originGuardTrustedPage, Origin: originGuardTrustedOrigin}}

	if _, err := RunProgram(context.Background(), entry, originGuardConfig(entry, probe, locator, &originGuardSink{})); err != nil {
		t.Fatalf("RunProgram() on the recorded origin = %v", err)
	}
	if probe.clicks != 1 {
		t.Fatalf("clicks = %d, want the healed element to be clicked once", probe.clicks)
	}
}

// TestRunProgramRejectsHealingWithoutALocationPort makes the misconfiguration
// a startup failure. Left to runtime it degrades into "healing mysteriously
// never applies", which is the kind of silence hosts work around by disabling
// the guard.
func TestRunProgramRejectsHealingWithoutALocationPort(t *testing.T) {
	entry := crossOriginHealEntry(t)
	probe := &originGuardProbe{}
	cfg := originGuardConfig(entry, probe, nil, &originGuardSink{})

	_, err := RunProgram(context.Background(), entry, cfg)

	if !fault.IsCode(err, CodeRuntimeConfigurationInvalid) {
		t.Fatalf("RunProgram() error = %v, want %q", err, CodeRuntimeConfigurationInvalid)
	}
	if probe.locates != 0 || probe.snapshot != 0 || probe.heals != 0 {
		t.Fatalf("configuration failure still observed runtime ports: %+v", probe)
	}
}

// leaksAny reports whether a public message carries any detail that belongs
// only in the private cause: the page it was on, the selector a healer
// proposed, or the reason the refusal fired.
func leaksAny(message string, secrets ...string) bool {
	for _, secret := range secrets {
		if secret != "" && strings.Contains(message, secret) {
			return true
		}
	}
	return false
}
