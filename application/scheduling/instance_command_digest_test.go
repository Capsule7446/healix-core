package scheduling

import (
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
)

// These digests were computed with json.Marshal until an execution coordinate
// became a struct whose only field is unexported. json.Marshal encodes such a
// struct as {}, so the instance identity silently vanished from the digest and
// two cancellations of different instances under one command id hashed the
// same. Nothing failed: the replay check simply stopped being able to tell the
// two requests apart, which is the exact confusion the digest exists to catch.
//
// A digest is only as good as the proof that every field reaches it, so these
// tests mutate one field at a time and require the digest to move. A field
// added to a command without a case here is a field that can go missing again.

func TestCancelInstanceRequestDigestSeparatesEveryField(t *testing.T) {
	base := CancelInstanceCommand{
		CommandID:        "command-1",
		InstanceID:       mustInstanceID("instance-1"),
		ExpectedStatus:   domainexecution.Queued,
		ExpectedRevision: 1,
		At:               2,
	}
	mutations := map[string]func(*CancelInstanceCommand){
		"command id":        func(c *CancelInstanceCommand) { c.CommandID = "command-2" },
		"instance id":       func(c *CancelInstanceCommand) { c.InstanceID = mustInstanceID("instance-2") },
		"expected status":   func(c *CancelInstanceCommand) { c.ExpectedStatus = domainexecution.Running },
		"expected revision": func(c *CancelInstanceCommand) { c.ExpectedRevision = 2 },
		"timestamp":         func(c *CancelInstanceCommand) { c.At = 3 },
	}
	requireEveryMutationMovesDigest(t, base, CancelInstanceRequestDigest, mutations)
}

func TestAbortInstanceRequestDigestSeparatesEveryField(t *testing.T) {
	base := AbortInstanceCommand{
		CommandID:        "command-1",
		InstanceID:       mustInstanceID("instance-1"),
		ExpectedRevision: 1,
		At:               2,
		Fence:            domainexecution.WorkerFence{InstanceID: mustInstanceID("instance-1"), ClaimToken: "claim-1"},
	}
	mutations := map[string]func(*AbortInstanceCommand){
		"command id":        func(c *AbortInstanceCommand) { c.CommandID = "command-2" },
		"instance id":       func(c *AbortInstanceCommand) { c.InstanceID = mustInstanceID("instance-2") },
		"expected revision": func(c *AbortInstanceCommand) { c.ExpectedRevision = 2 },
		"timestamp":         func(c *AbortInstanceCommand) { c.At = 3 },
		"fence instance":    func(c *AbortInstanceCommand) { c.Fence.InstanceID = mustInstanceID("instance-2") },
		"fence claim":       func(c *AbortInstanceCommand) { c.Fence.ClaimToken = "claim-2" },
	}
	requireEveryMutationMovesDigest(t, base, AbortInstanceRequestDigest, mutations)
}

func TestReorderQueueRequestDigestSeparatesEveryField(t *testing.T) {
	base := ReorderQueueCommand{
		CommandID:        "command-1",
		ScopeID:          "scope-1",
		ExpectedRevision: 1,
		InstanceIDs:      []string{"instance-1", "instance-2"},
	}
	mutations := map[string]func(*ReorderQueueCommand){
		"command id":        func(c *ReorderQueueCommand) { c.CommandID = "command-2" },
		"scope id":          func(c *ReorderQueueCommand) { c.ScopeID = "scope-2" },
		"expected revision": func(c *ReorderQueueCommand) { c.ExpectedRevision = 2 },
		"membership":        func(c *ReorderQueueCommand) { c.InstanceIDs = []string{"instance-1", "instance-3"} },
		// Order is the whole point of a reorder command, so a permutation is a
		// different request even though the membership is identical.
		"order": func(c *ReorderQueueCommand) { c.InstanceIDs = []string{"instance-2", "instance-1"} },
		"length": func(c *ReorderQueueCommand) {
			c.InstanceIDs = []string{"instance-1", "instance-2", "instance-3"}
		},
	}
	requireEveryMutationMovesDigest(t, base, ReorderQueueRequestDigest, mutations)
}

// Concatenation without a length prefix would let two different membership
// lists hash alike; the shared writeDigestString prefixes every value, and this
// pins that it keeps doing so.
func TestReorderQueueRequestDigestResistsMembershipConcatenation(t *testing.T) {
	split, err := ReorderQueueRequestDigest(ReorderQueueCommand{
		CommandID: "command-1", ScopeID: "scope-1", ExpectedRevision: 1,
		InstanceIDs: []string{"instance", "one"},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined, err := ReorderQueueRequestDigest(ReorderQueueCommand{
		CommandID: "command-1", ScopeID: "scope-1", ExpectedRevision: 1,
		InstanceIDs: []string{"instanceone"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if split == joined {
		t.Fatalf("membership boundaries are not encoded: both hash to %s", split)
	}
}

// The three commands must not share a digest space either: an abort and a
// cancel that agree on every common field are still different requests.
func TestInstanceCommandDigestsAreDomainSeparated(t *testing.T) {
	cancel, err := CancelInstanceRequestDigest(CancelInstanceCommand{
		CommandID: "command-1", InstanceID: mustInstanceID("instance-1"),
		ExpectedStatus: domainexecution.Queued, ExpectedRevision: 1, At: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	abort, err := AbortInstanceRequestDigest(AbortInstanceCommand{
		CommandID: "command-1", InstanceID: mustInstanceID("instance-1"),
		ExpectedRevision: 1, At: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	reorder, err := ReorderQueueRequestDigest(ReorderQueueCommand{
		CommandID: "command-1", ScopeID: "instance-1", ExpectedRevision: 1,
		InstanceIDs: []string{"instance-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		name string
		a, b string
	}{
		{"cancel vs abort", cancel, abort},
		{"cancel vs reorder", cancel, reorder},
		{"abort vs reorder", abort, reorder},
	} {
		if pair.a == pair.b {
			t.Errorf("%s share a digest: %s", pair.name, pair.a)
		}
	}
}

func requireEveryMutationMovesDigest[C any](t *testing.T, base C, digest func(C) (string, error), mutations map[string]func(*C)) {
	t.Helper()
	baseline, err := digest(base)
	if err != nil {
		t.Fatalf("baseline digest: %v", err)
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			mutated := base
			mutate(&mutated)
			got, err := digest(mutated)
			if err != nil {
				t.Fatalf("mutated digest: %v", err)
			}
			if got == baseline {
				t.Fatalf("%s does not reach the digest: still %s", name, baseline)
			}
		})
	}
}
