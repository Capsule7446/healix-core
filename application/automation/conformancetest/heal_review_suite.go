package conformancetest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	application "github.com/Capsule7446/healix-core/application/automation"
)

type HealReviewFaultPoint string

const (
	HealFaultAfterCandidate HealReviewFaultPoint = "AFTER_CANDIDATE"
	HealFaultAfterNode      HealReviewFaultPoint = "AFTER_NODE"
	HealFaultAfterStreak    HealReviewFaultPoint = "AFTER_STREAK"
	HealFaultAfterAudit     HealReviewFaultPoint = "AFTER_AUDIT"
	HealFaultAfterOutbox    HealReviewFaultPoint = "AFTER_OUTBOX"
	HealFaultBeforeReplay   HealReviewFaultPoint = "BEFORE_REPLAY"
)

type HealReviewSnapshot any

type HealReviewFixture interface {
	application.HealReviewTransaction
	Intent() application.HealReviewIntent
	CompetingIntents() (application.HealReviewIntent, application.HealReviewIntent)
	Snapshot() HealReviewSnapshot
	SetHealFault(HealReviewFaultPoint)
	MakeCandidateStale()
	MakeNodeStale()
	MakeCurrentBaseStale()
	MakeStreakStale()
	AssertApplied(application.HealReviewIntent) error
}

type HealReviewFactory func(*testing.T) HealReviewFixture

func RunHealReview(t *testing.T, factory HealReviewFactory) {
	t.Helper()
	t.Run("apply-replay-digest-and-identity", func(t *testing.T) {
		f, intent := factory(t), application.HealReviewIntent{}
		intent = f.Intent()
		want := healOutcome(intent, application.HealReviewApplied)
		got, err := f.CommitHealReview(context.Background(), intent)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("apply = %#v, %v; want %#v", got, err, want)
		}
		if err := f.AssertApplied(intent); err != nil {
			t.Fatal(err)
		}
		after := f.Snapshot()
		got, err = f.CommitHealReview(context.Background(), intent)
		want.Status = application.HealReviewReplayed
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("replay = %#v, %v", got, err)
		}
		if !reflect.DeepEqual(after, f.Snapshot()) {
			t.Fatal("replay changed state")
		}
		bad := intent
		bad.RequestDigest = "sha256:bad"
		if _, err := f.CommitHealReview(context.Background(), bad); !errors.Is(err, application.ErrHealReviewIdentityConflict) {
			t.Fatalf("malformed digest = %v", err)
		}
		changed := intent
		changed.NextCandidate.PageURL = "/different-payload"
		if _, err := f.CommitHealReview(context.Background(), changed); !errors.Is(err, application.ErrHealReviewIdentityConflict) {
			t.Fatalf("payload mismatch = %v", err)
		}
		changed.RequestDigest = mustHealDigest(t, changed)
		if _, err := f.CommitHealReview(context.Background(), changed); !errors.Is(err, application.ErrHealReviewIdentityConflict) {
			t.Fatalf("identity conflict = %v", err)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(HealReviewFixture)
	}{
		{"candidate", func(f HealReviewFixture) { f.MakeCandidateStale() }},
		{"node", func(f HealReviewFixture) { f.MakeNodeStale() }},
		{"current-base", func(f HealReviewFixture) { f.MakeCurrentBaseStale() }},
		{"streak", func(f HealReviewFixture) { f.MakeStreakStale() }},
	} {
		t.Run("cas-"+tc.name, func(t *testing.T) {
			f := factory(t)
			tc.mutate(f)
			before := f.Snapshot()
			if _, err := f.CommitHealReview(context.Background(), f.Intent()); !errors.Is(err, application.ErrHealReviewCASConflict) {
				t.Fatalf("error = %v", err)
			}
			if !reflect.DeepEqual(before, f.Snapshot()) {
				t.Fatal("CAS failure changed state")
			}
		})
	}

	for _, point := range []HealReviewFaultPoint{HealFaultAfterCandidate, HealFaultAfterNode, HealFaultAfterStreak, HealFaultAfterAudit, HealFaultAfterOutbox, HealFaultBeforeReplay} {
		t.Run("rollback-"+string(point), func(t *testing.T) {
			f := factory(t)
			before := f.Snapshot()
			f.SetHealFault(point)
			if _, err := f.CommitHealReview(context.Background(), f.Intent()); err == nil {
				t.Fatal("fault succeeded")
			}
			if !reflect.DeepEqual(before, f.Snapshot()) {
				t.Fatal("fault changed state")
			}
		})
	}

	t.Run("concurrent-equal", func(t *testing.T) {
		f := factory(t)
		intent := f.Intent()
		results := concurrentHeal(t, f, intent, intent)
		applied, replayed := 0, 0
		for _, r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			switch r.outcome.Status {
			case application.HealReviewApplied:
				applied++
			case application.HealReviewReplayed:
				replayed++
			}
		}
		if applied != 1 || replayed != 1 {
			t.Fatalf("applied=%d replayed=%d", applied, replayed)
		}
	})
	t.Run("concurrent-competing-decisions", func(t *testing.T) {
		f := factory(t)
		left, right := f.CompetingIntents()
		results := concurrentHeal(t, f, left, right)
		winners, losers := 0, 0
		for _, r := range results {
			if r.err == nil {
				winners++
				continue
			}
			if errors.Is(r.err, application.ErrHealReviewDecisionConflict) || errors.Is(r.err, application.ErrHealReviewCASConflict) {
				losers++
				continue
			}
			t.Fatalf("loser error = %v", r.err)
		}
		if winners != 1 || losers != 1 {
			t.Fatalf("winners=%d losers=%d", winners, losers)
		}
	})
}

type healResult struct {
	outcome application.HealReviewOutcome
	err     error
}

func concurrentHeal(t *testing.T, tx application.HealReviewTransaction, intents ...application.HealReviewIntent) []healResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	ch := make(chan healResult, len(intents))
	for _, intent := range intents {
		go func(i application.HealReviewIntent) {
			<-start
			o, e := tx.CommitHealReview(ctx, i)
			ch <- healResult{o, e}
		}(intent)
	}
	close(start)
	out := make([]healResult, 0, len(intents))
	for len(out) < len(intents) {
		select {
		case r := <-ch:
			out = append(out, r)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	return out
}
func healOutcome(i application.HealReviewIntent, status application.HealReviewStatus) application.HealReviewOutcome {
	return application.HealReviewOutcome{Status: status, CommandID: i.CommandID, RequestDigest: i.RequestDigest, Result: application.HealReviewResult{Decision: i.Decision, Candidate: i.NextCandidate, ElementTarget: i.NextNode, Streak: i.NextStreak}}
}
func mustHealDigest(t *testing.T, i application.HealReviewIntent) string {
	t.Helper()
	d, e := application.HealReviewRequestDigest(i)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
