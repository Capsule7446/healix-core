package execution

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

type recordingAbortTransaction struct {
	lookup   RequestAbortOutcome
	found    bool
	applied  RequestAbortOutcome
	intent   RequestAbortIntent
	lookups  int
	requests int
	lookupErr,
	requestErr error
}

func (t *recordingAbortTransaction) LookupAbortRequest(_ context.Context, _ domainexecution.EntryID, _ string) (RequestAbortOutcome, bool, error) {
	t.lookups++
	if t.lookupErr != nil {
		return RequestAbortOutcome{}, false, t.lookupErr
	}
	return t.lookup, t.found, nil
}

func (t *recordingAbortTransaction) RequestAbort(_ context.Context, intent RequestAbortIntent) (RequestAbortOutcome, error) {
	t.requests++
	t.intent = intent
	if t.requestErr != nil {
		return RequestAbortOutcome{}, t.requestErr
	}
	return t.applied, nil
}

func abortCommandFixture() RequestAbortCommand {
	return RequestAbortCommand{
		EntryID: mustEntryID("entry-1"),
		Fence:   domainexecution.WorkerFence{InstanceID: mustInstanceID("run-1"), ClaimToken: "claim-1"},
		State:   abortRunningState(TerminalIntentNone, 3, 1),
		Request: AbortRequest{AbortPendingCommandID: "command-1"},
	}
}

// TestRequestAbortDigestIsStableAndFieldSensitive pins the identity contract.
// Stability is what makes a retry a replay; field sensitivity is what stops two
// genuinely different requests from colliding on one receipt.
func TestRequestAbortDigestIsStableAndFieldSensitive(t *testing.T) {
	base := abortCommandFixture()
	first, err := RequestAbortDigest(base)
	if err != nil {
		t.Fatalf("RequestAbortDigest() = %v", err)
	}
	if !strings.HasPrefix(first, "sha256:") || len(first) != 71 {
		t.Fatalf("digest = %q, want a sha256 hex digest", first)
	}
	again, err := RequestAbortDigest(abortCommandFixture())
	if err != nil || again != first {
		t.Fatalf("digest is not stable: %q vs %q (%v)", again, first, err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*RequestAbortCommand)
	}{
		{"entry", func(c *RequestAbortCommand) { c.EntryID = mustEntryID("entry-2") }},
		{"instance", func(c *RequestAbortCommand) { c.Fence.InstanceID = mustInstanceID("run-2") }},
		{"claim token", func(c *RequestAbortCommand) { c.Fence.ClaimToken = "claim-2" }},
		{"entry status", func(c *RequestAbortCommand) { c.State.EntryStatus = domainexecution.EntryPending }},
		{"terminal intent", func(c *RequestAbortCommand) { c.State.TerminalIntent = TerminalIntentCancel }},
		{"intent revision", func(c *RequestAbortCommand) { c.State.TerminalIntentRevision = 4 }},
		{"cancellation generation", func(c *RequestAbortCommand) { c.State.CancellationGeneration = 2 }},
		{"command identity", func(c *RequestAbortCommand) { c.Request.AbortPendingCommandID = "command-2" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutated := abortCommandFixture()
			test.mutate(&mutated)
			digest, err := RequestAbortDigest(mutated)
			if err != nil {
				t.Fatalf("RequestAbortDigest() = %v", err)
			}
			if digest == first {
				t.Fatalf("%s does not change the digest", test.name)
			}
		})
	}
}

// TestRequestAbortDigestRefusesAnUndigestableCommand keeps a rejected command
// from being recorded under a plausible-looking identity.
func TestRequestAbortDigestRefusesAnUndigestableCommand(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*RequestAbortCommand)
	}{
		{"entry", func(c *RequestAbortCommand) { c.EntryID = domainexecution.EntryID{} }},
		{"fence", func(c *RequestAbortCommand) { c.Fence = domainexecution.WorkerFence{} }},
		{"state", func(c *RequestAbortCommand) { c.State.TerminalIntent = TerminalIntent("SHRUG") }},
		{"request", func(c *RequestAbortCommand) { c.Request = AbortRequest{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := abortCommandFixture()
			test.mutate(&command)
			digest, err := RequestAbortDigest(command)
			if !fault.IsCode(err, CodeRequestAbortCommandInvalid) {
				t.Fatalf("error = %v, want %s", err, CodeRequestAbortCommandInvalid)
			}
			if digest != "" {
				t.Fatalf("refused command produced digest %q", digest)
			}
		})
	}
}

// TestValidateRequestAbortIntentDigestRefusesASubstitutedDecision is the
// mechanical form of "the host never computes a counter". An adapter running
// this check cannot be handed a revision core did not produce.
func TestValidateRequestAbortIntentDigestRefusesASubstitutedDecision(t *testing.T) {
	command := abortCommandFixture()
	digest, err := RequestAbortDigest(command)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := DecideAbortRequest(command.State, command.Request)
	if err != nil {
		t.Fatal(err)
	}
	sound := RequestAbortIntent{EntryID: command.EntryID, RequestDigest: digest, Command: command, Decision: decision}
	if err := ValidateRequestAbortIntentDigest(sound); err != nil {
		t.Fatalf("well-formed intent rejected: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*RequestAbortIntent)
	}{
		{"digest", func(i *RequestAbortIntent) { i.RequestDigest = "sha256:" + strings.Repeat("0", 64) }},
		{"identity", func(i *RequestAbortIntent) { i.EntryID = mustEntryID("entry-2") }},
		{"forged revision", func(i *RequestAbortIntent) { i.Decision.NextIntentRevision += 1 }},
		{"forged generation", func(i *RequestAbortIntent) { i.Decision.NextCancellationGeneration += 1 }},
		{"forged intent", func(i *RequestAbortIntent) { i.Decision.NextIntent = TerminalIntentCancel }},
	} {
		t.Run(test.name, func(t *testing.T) {
			intent := sound
			test.mutate(&intent)
			if err := ValidateRequestAbortIntentDigest(intent); !fault.IsCode(err, CodeRequestAbortDigestMismatch) {
				t.Fatalf("error = %v, want %s", err, CodeRequestAbortDigestMismatch)
			}
		})
	}
}

// TestAbortRequestServiceAppliesOnceThenReplays is the idempotency contract the
// host's SQLite implementation is measured against.
func TestAbortRequestServiceAppliesOnceThenReplays(t *testing.T) {
	command := abortCommandFixture()
	digest, err := RequestAbortDigest(command)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := DecideAbortRequest(command.State, command.Request)
	if err != nil {
		t.Fatal(err)
	}

	transaction := &recordingAbortTransaction{
		applied: RequestAbortOutcome{Status: RequestAbortApplied, EntryID: command.EntryID, RequestDigest: digest, Decision: decision},
	}
	service := NewAbortRequestService(transaction)
	applied, err := service.Request(context.Background(), command)
	if err != nil {
		t.Fatalf("Request() = %v", err)
	}
	if applied.Status != RequestAbortApplied || applied.Decision != decision {
		t.Fatalf("applied = %+v", applied)
	}
	if transaction.intent.RequestDigest != digest || transaction.intent.Decision != decision {
		t.Fatalf("adapter received %+v", transaction.intent)
	}

	transaction.found = true
	transaction.lookup = RequestAbortOutcome{Status: RequestAbortReplayed, EntryID: command.EntryID, RequestDigest: digest, Decision: decision}
	replayed, err := service.Request(context.Background(), command)
	if err != nil {
		t.Fatalf("replay Request() = %v", err)
	}
	if replayed.Status != RequestAbortReplayed {
		t.Fatalf("replay status = %s", replayed.Status)
	}
	if transaction.requests != 1 {
		t.Fatalf("adapter applied %d times, want exactly 1", transaction.requests)
	}
}

// TestAbortRequestServiceRefusesBeforeTouchingStorage is why the service
// decides first: a request core would reject must cost no round trip and leave
// no partial trace.
func TestAbortRequestServiceRefusesBeforeTouchingStorage(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*RequestAbortCommand)
		code   fault.Code
	}{
		{"already aborting", func(c *RequestAbortCommand) { c.State.TerminalIntent = TerminalIntentAbort }, CodeAbortRequestAlreadyAborting},
		{"not running", func(c *RequestAbortCommand) { c.State.EntryStatus = domainexecution.EntrySucceeded }, CodeAbortRequestNotRunning},
		{"exhausted revision", func(c *RequestAbortCommand) {
			c.State.TerminalIntentRevision = MaxExpectedEntryCompletionRevision
		}, CodeEntryCompletionRevisionExhausted},
		{"undigestable", func(c *RequestAbortCommand) { c.Request = AbortRequest{} }, CodeRequestAbortCommandInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := abortCommandFixture()
			test.mutate(&command)
			transaction := &recordingAbortTransaction{}
			if _, err := NewAbortRequestService(transaction).Request(context.Background(), command); !fault.IsCode(err, test.code) {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
			if transaction.lookups != 0 || transaction.requests != 0 {
				t.Fatalf("adapter was reached: %d lookups, %d requests", transaction.lookups, transaction.requests)
			}
		})
	}
}

// TestAbortRequestServiceHoldsTheAdapterToItsContract refuses an outcome the
// caller must not act on rather than passing it through with a warning.
func TestAbortRequestServiceHoldsTheAdapterToItsContract(t *testing.T) {
	command := abortCommandFixture()
	digest, _ := RequestAbortDigest(command)
	decision, _ := DecideAbortRequest(command.State, command.Request)
	sound := RequestAbortOutcome{Status: RequestAbortApplied, EntryID: command.EntryID, RequestDigest: digest, Decision: decision}

	for _, test := range []struct {
		name    string
		outcome RequestAbortOutcome
	}{
		{"unknown status", func() RequestAbortOutcome { o := sound; o.Status = "SHRUG"; return o }()},
		{"different entry", func() RequestAbortOutcome { o := sound; o.EntryID = mustEntryID("entry-2"); return o }()},
		{"different request", func() RequestAbortOutcome { o := sound; o.RequestDigest = "sha256:x"; return o }()},
		{"recomputed decision", func() RequestAbortOutcome { o := sound; o.Decision.NextIntentRevision += 1; return o }()},
	} {
		t.Run(test.name, func(t *testing.T) {
			transaction := &recordingAbortTransaction{applied: test.outcome}
			if _, err := NewAbortRequestService(transaction).Request(context.Background(), command); !fault.IsCode(err, CodeRequestAbortAdapterContractViolation) {
				t.Fatalf("error = %v, want %s", err, CodeRequestAbortAdapterContractViolation)
			}
		})
	}

	t.Run("recorded hit not reported as replayed", func(t *testing.T) {
		transaction := &recordingAbortTransaction{found: true, lookup: sound}
		if _, err := NewAbortRequestService(transaction).Request(context.Background(), command); !fault.IsCode(err, CodeRequestAbortAdapterContractViolation) {
			t.Fatalf("error = %v, want %s", err, CodeRequestAbortAdapterContractViolation)
		}
	})
}

// TestAbortRequestServiceWithoutATransactionIsUnavailable covers the zero value
// and the typed-nil an interface conversion produces.
func TestAbortRequestServiceWithoutATransactionIsUnavailable(t *testing.T) {
	for _, service := range []AbortRequestService{
		{},
		NewAbortRequestService(nil),
		NewAbortRequestService((*recordingAbortTransaction)(nil)),
	} {
		if _, err := service.Request(context.Background(), abortCommandFixture()); !fault.IsCode(err, CodeRequestAbortUnavailable) {
			t.Fatalf("error = %v, want %s", err, CodeRequestAbortUnavailable)
		}
	}
}

// TestAbortRequestServicePropagatesAdapterFailures keeps an adapter error
// classified by the adapter rather than reclassified here, so a host can tell
// its own storage failure from a core refusal.
func TestAbortRequestServicePropagatesAdapterFailures(t *testing.T) {
	boom := errors.New("storage is down")
	for _, test := range []struct {
		name        string
		transaction *recordingAbortTransaction
	}{
		{"lookup", &recordingAbortTransaction{lookupErr: boom}},
		{"apply", &recordingAbortTransaction{requestErr: boom}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewAbortRequestService(test.transaction).Request(context.Background(), abortCommandFixture())
			if !errors.Is(err, boom) {
				t.Fatalf("error = %v, want it to wrap the adapter failure", err)
			}
		})
	}

	t.Run("identity conflict stays classified", func(t *testing.T) {
		transaction := &recordingAbortTransaction{requestErr: RequestAbortIdentityConflictError()}
		_, err := NewAbortRequestService(transaction).Request(context.Background(), abortCommandFixture())
		if !fault.IsCode(err, CodeRequestAbortIdentityConflict) {
			t.Fatalf("error = %v, want %s", err, CodeRequestAbortIdentityConflict)
		}
	})
}

// TestRequestAbortOutcomeIsComparable is what lets the service compare a
// recorded outcome against a freshly decided one with ==. A field added later
// that is not comparable would break that silently.
func TestRequestAbortOutcomeIsComparable(t *testing.T) {
	if !reflect.TypeOf(RequestAbortOutcome{}).Comparable() {
		t.Fatal("RequestAbortOutcome must stay comparable")
	}
	if !reflect.TypeOf(AbortRequestDecision{}).Comparable() {
		t.Fatal("AbortRequestDecision must stay comparable")
	}
}
