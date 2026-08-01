package execution

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/evidence"
)

// The owned copy handed to the Host used to be produced by marshalling the
// commit to JSON and reading it back. Every execution coordinate is a struct
// whose only field is unexported, so each one encoded as {} and decoded as a
// zero value: the copy reached the Host with a blank step execution id, a blank
// entry id, and blank identities on every observation attached to it.
//
// Nothing caught it. Validate and the fence binding check both run on the
// original, before the copy is made, and nothing re-checks the copy on the way
// out. The only existing assertion on the copy looked at an exported string
// field, which is exactly the kind that survived the round trip.

// commitWithObservations is the fixture these assertions need. The shared
// validStepTransitionCommit carries only an Event, so a test that loops over
// FinalValidations and HealObservations with it iterates zero times and passes
// while covering neither — which is what the first version of this test did.
func commitWithObservations(t *testing.T) evidence.StepTransitionCommit {
	t.Helper()
	commit := validStepTransitionCommit()
	commit.HealObservations = []evidence.HealObservation{{
		ID: "fact-1", InstanceID: mustInstanceID("instance-1"),
		EntryID: commit.Event.EntryID, StepExecutionID: commit.Event.ID,
		ElementTargetID: "node-1", BaseNodeVersionID: "base-1", CandidateHash: "candidate-1",
		Confidence: 0.9, DecisionBand: evidence.DecisionApplied, Succeeded: true, ObservedAt: 1,
	}}
	if err := commit.Validate(); err != nil {
		t.Fatalf("fixture is not a valid commit: %v", err)
	}
	return commit
}

func TestOwnedCommitKeepsEveryIdentity(t *testing.T) {
	original := commitWithObservations(t)
	// Without this the loops below can iterate zero times and the test reports a
	// surface it never touched.
	if len(original.HealObservations) == 0 {
		t.Fatal("the fixture carries no observations, so the per-observation checks would prove nothing")
	}

	owned, err := ownStepTransitionCommit(original)
	if err != nil {
		t.Fatalf("own commit: %v", err)
	}

	type check struct{ what, got, want string }
	checks := []check{
		{"commit id", owned.CommitID, original.CommitID},
		{"event step execution id", owned.Event.ID.String(), original.Event.ID.String()},
		{"event entry id", owned.Event.EntryID.String(), original.Event.EntryID.String()},
	}
	for index := range original.FinalValidations {
		checks = append(checks,
			check{"final validation instance id", owned.FinalValidations[index].InstanceID.String(), original.FinalValidations[index].InstanceID.String()},
			check{"final validation step execution id", owned.FinalValidations[index].StepExecutionID.String(), original.FinalValidations[index].StepExecutionID.String()},
		)
	}
	for index := range original.HealObservations {
		checks = append(checks,
			check{"heal observation instance id", owned.HealObservations[index].InstanceID.String(), original.HealObservations[index].InstanceID.String()},
			check{"heal observation entry id", owned.HealObservations[index].EntryID.String(), original.HealObservations[index].EntryID.String()},
			check{"heal observation step execution id", owned.HealObservations[index].StepExecutionID.String(), original.HealObservations[index].StepExecutionID.String()},
		)
	}

	for _, item := range checks {
		if item.want == "" {
			t.Errorf("%s is empty in the fixture, so this case proves nothing", item.what)
			continue
		}
		if item.got != item.want {
			t.Errorf("%s = %q, want %q; the owned copy lost it", item.what, item.got, item.want)
		}
	}
}

// The copy must also still be valid. A copy that Validate rejects is one the
// Host cannot use, and the round trip produced exactly that.
func TestOwnedCommitStillValidates(t *testing.T) {
	owned, err := ownStepTransitionCommit(commitWithObservations(t))
	if err != nil {
		t.Fatalf("own commit: %v", err)
	}
	if err := owned.Validate(); err != nil {
		t.Fatalf("the owned copy no longer validates: %v", err)
	}
}

// Owning it means the caller can keep mutating the original without the Host's
// copy following along.
func TestOwnedCommitDoesNotAliasTheCaller(t *testing.T) {
	original := commitWithObservations(t)
	if len(original.OriginalSelectorResets) == 0 {
		original.OriginalSelectorResets = []evidence.HealCandidateReset{{
			EntryID:           original.Event.EntryID,
			StepExecutionID:   original.Event.ID,
			ElementTargetID:   "node",
			BaseNodeVersionID: "base",
			ObservedAt:        1,
		}}
	}
	owned, err := ownStepTransitionCommit(original)
	if err != nil {
		t.Fatalf("own commit: %v", err)
	}
	original.OriginalSelectorResets[0].ElementTargetID = "mutated"
	if owned.OriginalSelectorResets[0].ElementTargetID == "mutated" {
		t.Fatal("the owned copy shares the caller's selector reset slice")
	}
}
