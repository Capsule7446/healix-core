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

func TestOwnedCommitKeepsEveryIdentity(t *testing.T) {
	original := validStepTransitionCommit()
	owned, err := ownStepTransitionCommit(original)
	if err != nil {
		t.Fatalf("own commit: %v", err)
	}

	checks := []struct {
		what string
		got  string
		want string
	}{
		{"commit id", owned.CommitID, original.CommitID},
		{"event step execution id", owned.Event.ID.String(), original.Event.ID.String()},
		{"event entry id", owned.Event.EntryID.String(), original.Event.EntryID.String()},
	}
	for index := range original.FinalValidations {
		checks = append(checks,
			struct {
				what string
				got  string
				want string
			}{"final validation instance id", owned.FinalValidations[index].InstanceID.String(), original.FinalValidations[index].InstanceID.String()},
			struct {
				what string
				got  string
				want string
			}{"final validation step execution id", owned.FinalValidations[index].StepExecutionID.String(), original.FinalValidations[index].StepExecutionID.String()},
		)
	}
	for index := range original.HealObservations {
		checks = append(checks, struct {
			what string
			got  string
			want string
		}{"heal observation entry id", owned.HealObservations[index].EntryID.String(), original.HealObservations[index].EntryID.String()})
	}

	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s = %q, want %q; the owned copy lost it", check.what, check.got, check.want)
		}
		if check.want == "" {
			t.Errorf("%s is empty in the fixture, so this case proves nothing", check.what)
		}
	}
}

// The copy must also still be valid. A copy that Validate rejects is one the
// Host cannot use, and the round trip produced exactly that.
func TestOwnedCommitStillValidates(t *testing.T) {
	owned, err := ownStepTransitionCommit(validStepTransitionCommit())
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
	original := validStepTransitionCommit()
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
