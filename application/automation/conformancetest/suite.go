package conformancetest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	application "github.com/Capsule7446/healix-core/application/automation"
	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type FaultPoint string

const (
	FaultAfterNodes    FaultPoint = "AFTER_NODES"
	FaultAfterWorkflow FaultPoint = "AFTER_WORKFLOW"
	FaultAfterMappings FaultPoint = "AFTER_MAPPINGS"
	FaultAfterAudit    FaultPoint = "AFTER_AUDIT"
	FaultAfterOutbox   FaultPoint = "AFTER_OUTBOX"
	FaultBeforeReplay  FaultPoint = "BEFORE_REPLAY"
)

type Snapshot any

type Fixture interface {
	application.SamplingPublicationTransaction
	Intent() application.PublishSamplingIntent
	Snapshot() Snapshot
	SetFault(FaultPoint)
	ClearFault()
	MakeRevisionStale(nodeID string)
	MakeCurrentVersionStale(nodeID string)
	CompetingIntents() (application.PublishSamplingIntent, application.PublishSamplingIntent)
	AssertApplied(application.PublishSamplingIntent) error
	AssertOnlyApplied(application.PublishSamplingIntent, application.PublishSamplingIntent) error
}

type Factory func(t *testing.T) Fixture

func Run(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("applied-replay-and-identity-conflict", func(t *testing.T) {
		fixture := factory(t)
		intent := fixture.Intent()
		first, err := fixture.PublishSampling(context.Background(), intent)
		expected := application.PublishSamplingOutcome{Status: application.PublishSamplingApplied, PublicationID: intent.PublicationID, RequestDigest: intent.RequestDigest, Result: Result(intent.Publication)}
		if err != nil || !reflect.DeepEqual(first, expected) {
			t.Fatalf("first = %#v, %v; want %#v", first, err, expected)
		}
		if err := fixture.AssertApplied(intent); err != nil {
			t.Fatalf("materialized publication: %v", err)
		}
		after := fixture.Snapshot()
		replay, err := fixture.PublishSampling(context.Background(), intent)
		expected.Status = application.PublishSamplingReplayed
		if err != nil || !reflect.DeepEqual(replay, expected) {
			t.Fatalf("replay = %#v, %v", replay, err)
		}
		if got := fixture.Snapshot(); !reflect.DeepEqual(got, after) {
			t.Fatalf("replay changed state: before=%#v after=%#v", after, got)
		}
		changed := intent
		changed.RequestDigest = "sha256:changed"
		if _, err := fixture.PublishSampling(context.Background(), changed); !errors.Is(err, application.ErrSamplingPublicationDigestMismatch) {
			t.Fatalf("malformed digest error = %v", err)
		}
		changed = intent
		changed.Publication = intent.Publication.Clone()
		changed.Publication.FlowFragment.FlowFragment.DisplayName = "changed payload"
		if _, err := fixture.PublishSampling(context.Background(), changed); !errors.Is(err, application.ErrSamplingPublicationDigestMismatch) {
			t.Fatalf("payload digest mismatch error = %v", err)
		}
		changed = intent
		changed.Publication = intent.Publication.Clone()
		changed.Publication.FlowFragment.FlowFragment.DisplayName = "different valid payload"
		command := application.SamplingPublicationCommand{PublicationID: changed.PublicationID, Publication: changed.Publication}
		changed.RequestDigest, err = application.SamplingPublicationRequestDigest(command)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.PublishSampling(context.Background(), changed); !errors.Is(err, application.ErrSamplingPublicationIdentityConflict) {
			t.Fatalf("identity conflict error = %v", err)
		}
		changed = intent
		changed.RequestDigest, changed.PublicationID = "sha256:changed", "other-publication"
		if _, err := fixture.PublishSampling(context.Background(), changed); !errors.Is(err, application.ErrSamplingPublicationDigestMismatch) {
			t.Fatalf("arbitrary digest error = %v", err)
		}
	})

	t.Run("faults-roll-back-all-state", func(t *testing.T) {
		for _, point := range []FaultPoint{FaultAfterNodes, FaultAfterWorkflow, FaultAfterMappings, FaultAfterAudit, FaultAfterOutbox, FaultBeforeReplay} {
			t.Run(string(point), func(t *testing.T) {
				fixture := factory(t)
				before := fixture.Snapshot()
				fixture.SetFault(point)
				if _, err := fixture.PublishSampling(context.Background(), fixture.Intent()); err == nil {
					t.Fatal("faulted publication succeeded")
				}
				if got := fixture.Snapshot(); !reflect.DeepEqual(got, before) {
					t.Fatalf("fault changed state: before=%#v after=%#v", before, got)
				}
			})
		}
	})

	for _, test := range []struct {
		name   string
		mode   string
		mutate func(Fixture, string)
	}{
		{name: "merge-revision", mode: "MERGE", mutate: func(f Fixture, id string) { f.MakeRevisionStale(id) }},
		{name: "merge-current-version", mode: "MERGE", mutate: func(f Fixture, id string) { f.MakeCurrentVersionStale(id) }},
		{name: "reuse-revision", mode: "REUSE", mutate: func(f Fixture, id string) { f.MakeRevisionStale(id) }},
		{name: "reuse-current-version", mode: "REUSE", mutate: func(f Fixture, id string) { f.MakeCurrentVersionStale(id) }},
	} {
		t.Run("stale-authority-"+test.name, func(t *testing.T) {
			fixture := factory(t)
			intent := fixture.Intent()
			guarded := ""
			for _, node := range intent.Publication.Nodes {
				if node.ResolutionMode == test.mode {
					guarded = node.Aggregate.ElementTarget.ID
					break
				}
			}
			if guarded == "" {
				t.Fatalf("fixture has no %s node", test.mode)
			}
			test.mutate(fixture, guarded)
			before := fixture.Snapshot()
			if _, err := fixture.PublishSampling(context.Background(), intent); !errors.Is(err, application.ErrSamplingPublicationAuthorityConflict) {
				t.Fatalf("stale authority error = %v", err)
			}
			if got := fixture.Snapshot(); !reflect.DeepEqual(got, before) {
				t.Fatalf("stale publication changed state: before=%#v after=%#v", before, got)
			}
		})
	}

	t.Run("concurrent-competing-publication-ids-have-one-winner", func(t *testing.T) {
		fixture := factory(t)
		left, right := fixture.CompetingIntents()
		if left.PublicationID == right.PublicationID || left.RequestDigest == right.RequestDigest {
			t.Fatal("competing intents must have distinct identities")
		}
		type result struct {
			intent  application.PublishSamplingIntent
			outcome application.PublishSamplingOutcome
			err     error
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		start := make(chan struct{})
		results := make(chan result, 2)
		for _, intent := range []application.PublishSamplingIntent{left, right} {
			go func(intent application.PublishSamplingIntent) {
				<-start
				outcome, err := fixture.PublishSampling(ctx, intent)
				results <- result{intent: intent, outcome: outcome, err: err}
			}(intent)
		}
		close(start)
		completed := make([]result, 0, 2)
		for len(completed) < 2 {
			select {
			case got := <-results:
				completed = append(completed, got)
			case <-ctx.Done():
				t.Fatalf("competing publications did not complete: %v", ctx.Err())
			}
		}
		var winner, loser application.PublishSamplingIntent
		for _, got := range completed {
			if got.err == nil {
				expected := application.PublishSamplingOutcome{Status: application.PublishSamplingApplied, PublicationID: got.intent.PublicationID, RequestDigest: got.intent.RequestDigest, Result: Result(got.intent.Publication)}
				if !reflect.DeepEqual(got.outcome, expected) || winner.PublicationID != "" {
					t.Fatalf("competing winner = %#v, %v", got.outcome, got.err)
				}
				winner = got.intent
				continue
			}
			if !errors.Is(got.err, application.ErrSamplingPublicationAuthorityConflict) || loser.PublicationID != "" {
				t.Fatalf("competing loser error = %v", got.err)
			}
			loser = got.intent
		}
		if winner.PublicationID == "" || loser.PublicationID == "" {
			t.Fatalf("winner=%q loser=%q", winner.PublicationID, loser.PublicationID)
		}
		if err := fixture.AssertOnlyApplied(winner, loser); err != nil {
			t.Fatalf("competing publication state: %v", err)
		}
	})

	t.Run("concurrent-equal-publication-has-one-winner", func(t *testing.T) {
		fixture := factory(t)
		intent := fixture.Intent()
		type result struct {
			outcome application.PublishSamplingOutcome
			err     error
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		start := make(chan struct{})
		results := make(chan result, 2)
		for range 2 {
			go func() {
				<-start
				outcome, err := fixture.PublishSampling(ctx, intent)
				results <- result{outcome: outcome, err: err}
			}()
		}
		close(start)
		completed := make([]result, 0, 2)
		for len(completed) < 2 {
			select {
			case got := <-results:
				completed = append(completed, got)
			case <-ctx.Done():
				t.Fatalf("equal publications did not complete: %v", ctx.Err())
			}
		}
		applied, replayed := 0, 0
		expected := application.PublishSamplingOutcome{PublicationID: intent.PublicationID, RequestDigest: intent.RequestDigest, Result: Result(intent.Publication)}
		for _, got := range completed {
			if got.err != nil {
				t.Fatalf("concurrent publication error = %v", got.err)
			}
			expected.Status = got.outcome.Status
			if !reflect.DeepEqual(got.outcome, expected) {
				t.Fatalf("malformed concurrent outcome = %#v", got.outcome)
			}
			switch got.outcome.Status {
			case application.PublishSamplingApplied:
				applied++
			case application.PublishSamplingReplayed:
				replayed++
			}
		}
		if applied != 1 || replayed != 1 {
			t.Fatalf("statuses = applied %d replayed %d", applied, replayed)
		}
	})
}

func Result(publication domain.SamplingPublication) domain.SamplingPublicationResult {
	mappings := make([]domain.SamplingNodeMapping, len(publication.Nodes))
	for index, node := range publication.Nodes {
		mappings[index] = domain.SamplingNodeMapping{TemporaryElementTargetID: node.TemporaryElementTargetID, ElementTargetID: node.Aggregate.ElementTarget.ID, ElementTargetVersionID: node.Aggregate.Current.ID, ResolutionMode: node.ResolutionMode}
	}
	return domain.SamplingPublicationResult{FlowFragmentID: publication.FlowFragment.FlowFragment.ID, WorkflowVersionID: publication.FlowFragment.Current.ID, VersionNumber: publication.FlowFragment.Current.VersionNumber, Nodes: mappings}
}
