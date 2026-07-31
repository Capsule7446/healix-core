package conformancetest_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	application "github.com/Capsule7446/healix-core/application/automation"
	"github.com/Capsule7446/healix-core/application/automation/conformancetest"
	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type healState struct {
	candidate domain.HealCandidate
	node      domain.ElementTargetAggregate
	streak    domain.HealStreak
	audits    []application.HealReviewResult
	outbox    []application.HealReviewResult
	replays   map[string]application.HealReviewOutcome
	digests   map[string]string
}
type healFixture struct {
	mu     sync.Mutex
	state  healState
	intent application.HealReviewIntent
	fault  conformancetest.HealReviewFaultPoint
}

func newHealFixture(t *testing.T) conformancetest.HealReviewFixture {
	t.Helper()
	base, err := domain.NewElementTarget(domain.ElementTarget{ID: "node", DisplayName: "ElementTarget", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1}, domain.ElementTargetVersion{ID: "node-v1", ElementTargetID: "node", VersionNumber: 1, Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#old"}}, Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"role": "old"}}, Source: domain.SourceManual, CreatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	next, err := base.PublishVersion("node-v2", "/new", "heal", []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#new"}}, fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"role": "new"}}, domain.SourceAutoHeal, 2)
	if err != nil {
		t.Fatal(err)
	}
	candidate := domain.HealCandidate{Hash: "candidate", ElementTargetID: "node", BaseNodeVersionID: "node-v1", Status: domain.HealCandidateAwaitingApproval, Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#new"}}, Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"role": "new"}}, Revision: 1}
	nextCandidate := candidate
	nextCandidate.Status = domain.HealCandidatePromoted
	nextCandidate.Revision = 2
	streak := domain.HealStreak{ElementTargetID: "node", BaseNodeVersionID: "node-v1", CandidateHash: "candidate", Disposition: domain.HealStreakAwaitApproval, LastSequence: 3, Contributions: []domain.ContributingHealFact{{FactID: "fact", Sequence: 3}}}
	intent := application.HealReviewIntent{CommandID: "command", Decision: application.HealReviewApprove, ElementTargetID: "node", BaseNodeVersionID: "node-v1", CandidateHash: "candidate", ExpectedCandidateRevision: 1, ExpectedNodeRevision: 1, NextCandidate: nextCandidate, NextNode: &next, ReviewedBy: "reviewer", ReviewedAt: 10}
	intent.RequestDigest = healDigest(t, intent)
	return &healFixture{state: healState{candidate: candidate, node: base, streak: streak, replays: map[string]application.HealReviewOutcome{}, digests: map[string]string{}}, intent: intent}
}
func (f *healFixture) Intent() application.HealReviewIntent { return cloneIntent(f.intent) }
func (f *healFixture) Snapshot() conformancetest.HealReviewSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneHealState(f.state)
}
func (f *healFixture) SetHealFault(p conformancetest.HealReviewFaultPoint) {
	if p == conformancetest.HealFaultAfterStreak {
		f.intent = f.rejectIntent()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fault = p
}
func (f *healFixture) MakeCandidateStale() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state.candidate.Revision++
}
func (f *healFixture) MakeNodeStale() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state.node.ElementTarget.Revision++
}
func (f *healFixture) MakeCurrentBaseStale() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state.node.Current.ID = "node-other"
	f.state.node.ElementTarget.CurrentVersionID = "node-other"
}
func (f *healFixture) MakeStreakStale() {
	f.mu.Lock()
	defer f.mu.Unlock()
	reject := f.rejectIntent()
	f.intent = reject
	f.state.streak.LastSequence++
}
func (f *healFixture) CompetingIntents() (application.HealReviewIntent, application.HealReviewIntent) {
	return f.Intent(), f.rejectIntent()
}
func (f *healFixture) rejectIntent() application.HealReviewIntent {
	i := cloneIntent(f.intent)
	i.CommandID = "reject-command"
	i.Decision = application.HealReviewReject
	i.NextCandidate.Status = domain.HealCandidateRejected
	i.NextNode = nil
	expected := f.state.streak
	next := expected
	next.Disposition = domain.HealStreakRejected
	next.LastSequence++
	next.Contributions = append(next.Contributions, domain.ContributingHealFact{FactID: "review", CommitID: "review", RunID: "review", Sequence: next.LastSequence})
	i.ExpectedStreak = &expected
	i.ExpectedStreakDigest, _ = application.HealReviewStreakDigest(expected)
	i.NextStreak = &next
	i.RequestDigest = healDigest(nil, i)
	return i
}
func (f *healFixture) AssertApplied(i application.HealReviewIntent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !reflect.DeepEqual(f.state.candidate, i.NextCandidate) || len(f.state.audits) != 1 || len(f.state.outbox) != 1 || len(f.state.replays) != 1 {
		return errors.New("incomplete materialization")
	}
	if i.NextNode != nil && !reflect.DeepEqual(f.state.node, *i.NextNode) {
		return errors.New("node mismatch")
	}
	return nil
}

func (f *healFixture) LookupHealReview(_ context.Context, commandID, digest string) (application.HealReviewOutcome, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	outcome, ok := f.state.replays[commandID]
	if !ok {
		return application.HealReviewOutcome{}, false, nil
	}
	if f.state.digests[commandID] != digest {
		return application.HealReviewOutcome{}, false, application.ErrHealReviewIdentityConflict
	}
	outcome.Status = application.HealReviewReplayed
	return cloneOutcome(outcome), true, nil
}

func (f *healFixture) CommitHealReview(_ context.Context, i application.HealReviewIntent) (application.HealReviewOutcome, error) {
	if err := application.ValidateHealReviewIntentDigest(i); err != nil {
		return application.HealReviewOutcome{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if old, ok := f.state.replays[i.CommandID]; ok {
		if f.state.digests[i.CommandID] != i.RequestDigest || !reflect.DeepEqual(old.Result, application.HealReviewResult{Decision: i.Decision, Candidate: i.NextCandidate, ElementTarget: i.NextNode, Streak: i.NextStreak}) {
			return application.HealReviewOutcome{}, application.ErrHealReviewIdentityConflict
		}
		old.Status = application.HealReviewReplayed
		return cloneOutcome(old), nil
	}
	if f.state.candidate.Status != domain.HealCandidateAwaitingApproval {
		return application.HealReviewOutcome{}, application.ErrHealReviewDecisionConflict
	}
	if f.state.candidate.Revision != i.ExpectedCandidateRevision || f.state.node.ElementTarget.Revision != i.ExpectedNodeRevision || f.state.node.Current.ID != i.BaseNodeVersionID {
		return application.HealReviewOutcome{}, application.HealReviewCASConflictError()
	}
	if i.ExpectedStreak != nil {
		streakDigest, _ := application.HealReviewStreakDigest(f.state.streak)
		if !reflect.DeepEqual(f.state.streak, *i.ExpectedStreak) || streakDigest != i.ExpectedStreakDigest {
			return application.HealReviewOutcome{}, application.HealReviewCASConflictError()
		}
	}
	n := cloneHealState(f.state)
	n.candidate = cloneCandidate(i.NextCandidate)
	if f.fail(conformancetest.HealFaultAfterCandidate) != nil {
		return application.HealReviewOutcome{}, errors.New("injected fault")
	}
	if i.NextNode != nil {
		n.node = i.NextNode.Clone()
		if f.fail(conformancetest.HealFaultAfterNode) != nil {
			return application.HealReviewOutcome{}, errors.New("injected fault")
		}
	}
	if i.NextStreak != nil {
		n.streak = cloneStreak(*i.NextStreak)
		if f.fail(conformancetest.HealFaultAfterStreak) != nil {
			return application.HealReviewOutcome{}, errors.New("injected fault")
		}
	}
	result := application.HealReviewResult{Decision: i.Decision, Candidate: cloneCandidate(i.NextCandidate), ElementTarget: cloneElementTarget(i.NextNode), Streak: cloneStreakPtr(i.NextStreak)}
	n.audits = append(n.audits, cloneResult(result))
	if f.fail(conformancetest.HealFaultAfterAudit) != nil {
		return application.HealReviewOutcome{}, errors.New("injected fault")
	}
	n.outbox = append(n.outbox, cloneResult(result))
	if f.fail(conformancetest.HealFaultAfterOutbox) != nil {
		return application.HealReviewOutcome{}, errors.New("injected fault")
	}
	if f.fail(conformancetest.HealFaultBeforeReplay) != nil {
		return application.HealReviewOutcome{}, errors.New("injected fault")
	}
	o := application.HealReviewOutcome{Status: application.HealReviewApplied, CommandID: i.CommandID, RequestDigest: i.RequestDigest, Result: result}
	n.replays[i.CommandID] = cloneOutcome(o)
	n.digests[i.CommandID] = i.RequestDigest
	f.state = n
	return cloneOutcome(o), nil
}
func (f *healFixture) fail(p conformancetest.HealReviewFaultPoint) error {
	if f.fault == p {
		return errors.New("fault")
	}
	return nil
}
func healDigest(t *testing.T, i application.HealReviewIntent) string {
	d, e := application.HealReviewRequestDigest(i)
	if e != nil {
		if t != nil {
			t.Fatal(e)
		}
		panic(e)
	}
	return d
}
func cloneIntent(i application.HealReviewIntent) application.HealReviewIntent {
	r := i
	r.NextCandidate = cloneCandidate(i.NextCandidate)
	r.NextNode = cloneElementTarget(i.NextNode)
	r.ExpectedStreak = cloneStreakPtr(i.ExpectedStreak)
	r.NextStreak = cloneStreakPtr(i.NextStreak)
	return r
}
func cloneCandidate(c domain.HealCandidate) domain.HealCandidate {
	r := c
	r.Selectors = append([]fingerprint.Selector(nil), c.Selectors...)
	r.Fingerprint.Path = append([]string(nil), c.Fingerprint.Path...)
	r.Fingerprint.Attributes = make(map[string]string, len(c.Fingerprint.Attributes))
	for key, value := range c.Fingerprint.Attributes {
		r.Fingerprint.Attributes[key] = value
	}
	return r
}
func cloneElementTarget(n *domain.ElementTargetAggregate) *domain.ElementTargetAggregate {
	if n == nil {
		return nil
	}
	r := n.Clone()
	return &r
}
func cloneStreak(s domain.HealStreak) domain.HealStreak {
	r := s
	r.Contributions = append([]domain.ContributingHealFact(nil), s.Contributions...)
	return r
}
func cloneStreakPtr(s *domain.HealStreak) *domain.HealStreak {
	if s == nil {
		return nil
	}
	r := cloneStreak(*s)
	return &r
}
func cloneResult(r application.HealReviewResult) application.HealReviewResult {
	return application.HealReviewResult{Decision: r.Decision, Candidate: cloneCandidate(r.Candidate), ElementTarget: cloneElementTarget(r.ElementTarget), Streak: cloneStreakPtr(r.Streak)}
}
func cloneOutcome(o application.HealReviewOutcome) application.HealReviewOutcome {
	o.Result = cloneResult(o.Result)
	return o
}
func cloneHealState(s healState) healState {
	r := healState{candidate: cloneCandidate(s.candidate), node: s.node.Clone(), streak: cloneStreak(s.streak), audits: make([]application.HealReviewResult, len(s.audits)), outbox: make([]application.HealReviewResult, len(s.outbox)), replays: map[string]application.HealReviewOutcome{}, digests: map[string]string{}}
	for i, v := range s.audits {
		r.audits[i] = cloneResult(v)
	}
	for i, v := range s.outbox {
		r.outbox[i] = cloneResult(v)
	}
	for k, v := range s.replays {
		r.replays[k] = cloneOutcome(v)
	}
	for k, v := range s.digests {
		r.digests[k] = v
	}
	return r
}
func TestReferenceHealReviewTransactionConformance(t *testing.T) {
	conformancetest.RunHealReview(t, newHealFixture)
}
