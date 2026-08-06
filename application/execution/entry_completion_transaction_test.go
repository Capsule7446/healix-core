package execution

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/application/engine"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

func completeEntryCommandFixture(t *testing.T) CompleteEntryCommand {
	t.Helper()
	return CompleteEntryCommand{
		EntryID: mustEntryID("entry-1"),
		Fence: domainexecution.WorkerFence{
			InstanceID: mustInstanceID("instance-1"),
			ClaimToken: "claim-1",
		},
		State:                 runningCompletionState(TerminalIntentCancel),
		Outcome:               engineOutcome(engine.OutcomeFailed, engine.RecordingStopFailed, engine.TimelineComplete),
		AbortPendingCommandID: "abort-cmd-1",
	}
}

func mustCompleteEntryDigest(t *testing.T, command CompleteEntryCommand) string {
	t.Helper()
	digest, err := CompleteEntryRequestDigest(command)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	return digest
}

func mustEntryCompletionDecision(t *testing.T, command CompleteEntryCommand) EntryCompletionDecision {
	t.Helper()
	decision, err := DecideEntryCompletion(command.State, command.Outcome)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	return decision
}

// completionTransactionFixture is the smallest adapter that satisfies the port:
// one map of applied receipts plus the levers each negative test needs.
type completionTransactionFixture struct {
	records    map[string]CompleteEntryOutcome
	lookups    int
	applies    int
	lookupErr  error
	applyErr   error
	lookupHide bool
	mutate     func(CompleteEntryOutcome) CompleteEntryOutcome
	lastIntent CompleteEntryIntent
}

func newCompletionTransactionFixture() *completionTransactionFixture {
	return &completionTransactionFixture{records: map[string]CompleteEntryOutcome{}}
}

func (f *completionTransactionFixture) LookupEntryCompletion(_ context.Context, entryID domainexecution.EntryID, digest string) (CompleteEntryOutcome, bool, error) {
	f.lookups++
	if f.lookupErr != nil {
		return CompleteEntryOutcome{}, false, f.lookupErr
	}
	if f.lookupHide {
		return CompleteEntryOutcome{}, false, nil
	}
	record, ok := f.records[entryID.String()+"|"+digest]
	if !ok {
		return CompleteEntryOutcome{}, false, nil
	}
	record.Status = CompleteEntryReplayed
	if f.mutate != nil {
		record = f.mutate(record)
	}
	return record, true, nil
}

func (f *completionTransactionFixture) CompleteEntry(_ context.Context, intent CompleteEntryIntent) (CompleteEntryOutcome, error) {
	f.applies++
	f.lastIntent = intent
	if f.applyErr != nil {
		return CompleteEntryOutcome{}, f.applyErr
	}
	if err := ValidateCompleteEntryIntentDigest(intent); err != nil {
		return CompleteEntryOutcome{}, err
	}
	outcome := CompleteEntryOutcome{
		Status:        CompleteEntryApplied,
		EntryID:       intent.EntryID,
		RequestDigest: intent.RequestDigest,
		Decision:      intent.Decision,
	}
	f.records[intent.EntryID.String()+"|"+intent.RequestDigest] = outcome
	if f.mutate != nil {
		return f.mutate(outcome), nil
	}
	return outcome, nil
}

func mustEntryCompletionService(t *testing.T, transaction EntryCompletionTransaction) EntryCompletionService {
	t.Helper()
	return NewEntryCompletionService(transaction)
}

func TestCompleteEntryRequestDigestIsStableAndFieldSensitive(t *testing.T) {
	base := completeEntryCommandFixture(t)
	digest := mustCompleteEntryDigest(t, base)

	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("digest must be sha256 prefixed, got %q", digest)
	}
	if again := mustCompleteEntryDigest(t, base); again != digest {
		t.Fatalf("digest must be stable, got %q then %q", digest, again)
	}

	mutations := map[string]func(*CompleteEntryCommand){
		"entry id":                 func(c *CompleteEntryCommand) { c.EntryID = mustEntryID("entry-2") },
		"fence instance":           func(c *CompleteEntryCommand) { c.Fence.InstanceID = mustInstanceID("instance-2") },
		"fence claim token":        func(c *CompleteEntryCommand) { c.Fence.ClaimToken = "claim-2" },
		"state entry status":       func(c *CompleteEntryCommand) { c.State.EntryStatus = domainexecution.EntryPending },
		"state terminal intent":    func(c *CompleteEntryCommand) { c.State.TerminalIntent = TerminalIntentAbort },
		"state intent revision":    func(c *CompleteEntryCommand) { c.State.TerminalIntentRevision++ },
		"state generation":         func(c *CompleteEntryCommand) { c.State.CancellationGeneration++ },
		"execution outcome":        func(c *CompleteEntryCommand) { c.Outcome.Result.ExecutionOutcome = engine.OutcomeSucceeded },
		"recording outcome":        func(c *CompleteEntryCommand) { c.Outcome.Result.RecordingOutcome = engine.RecordingDisabled },
		"timeline outcome":         func(c *CompleteEntryCommand) { c.Outcome.Result.TimelineOutcome = engine.TimelineDisabled },
		"failure code":             func(c *CompleteEntryCommand) { c.Outcome.FailureCode = CodeSchedulingAdapterUnavailable },
		"abort pending command id": func(c *CompleteEntryCommand) { c.AbortPendingCommandID = "abort-cmd-2" },
	}

	seen := map[string]string{digest: "base"}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			command := base
			mutate(&command)
			mutated := mustCompleteEntryDigest(t, command)
			if mutated == digest {
				t.Fatalf("%s must change the digest", name)
			}
			if owner, ok := seen[mutated]; ok {
				t.Fatalf("%s collides with %s", name, owner)
			}
			seen[mutated] = name
		})
	}
}

// TestCompleteEntryRequestDigestExcludesWallClock guards replay idempotency:
// anything that changes between two attempts at the same completion would turn
// a retry into a second apply.
func TestCompleteEntryRequestDigestExcludesWallClock(t *testing.T) {
	commandType := reflect.TypeOf(CompleteEntryCommand{})
	for i := range commandType.NumField() {
		field := commandType.Field(i)
		if strings.Contains(field.Type.String(), "time.Time") || strings.Contains(field.Type.String(), "time.Duration") {
			t.Fatalf("CompleteEntryCommand.%s is wall-clock shaped: a retry would digest differently and apply twice", field.Name)
		}
	}
}

func TestCompleteEntryRequestDigestRejectsMalformedCommands(t *testing.T) {
	cases := map[string]func(*CompleteEntryCommand){
		"missing entry id":            func(c *CompleteEntryCommand) { c.EntryID = domainexecution.EntryID{} },
		"missing fence instance":      func(c *CompleteEntryCommand) { c.Fence.InstanceID = domainexecution.InstanceID{} },
		"missing fence claim token":   func(c *CompleteEntryCommand) { c.Fence.ClaimToken = "" },
		"unknown entry status":        func(c *CompleteEntryCommand) { c.State.EntryStatus = "MADE_UP" },
		"unknown terminal intent":     func(c *CompleteEntryCommand) { c.State.TerminalIntent = "STOP" },
		"negative intent revision":    func(c *CompleteEntryCommand) { c.State.TerminalIntentRevision = -1 },
		"unknown execution outcome":   func(c *CompleteEntryCommand) { c.Outcome.Result.ExecutionOutcome = "MADE_UP" },
		"blank abort pending command": func(c *CompleteEntryCommand) { c.AbortPendingCommandID = "   " },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			command := completeEntryCommandFixture(t)
			mutate(&command)
			digest, err := CompleteEntryRequestDigest(command)
			if !fault.IsCode(err, CodeCompleteEntryCommandInvalid) {
				t.Fatalf("want %q, got %v", CodeCompleteEntryCommandInvalid, err)
			}
			if digest != "" {
				t.Fatalf("refused digest must be empty, got %q", digest)
			}
		})
	}
}

func TestCompleteEntryRequestDigestAcceptsAnAbsentAbortPendingCommand(t *testing.T) {
	command := completeEntryCommandFixture(t)
	command.AbortPendingCommandID = ""
	if _, err := CompleteEntryRequestDigest(command); err != nil {
		t.Fatalf("an entry with no pending abort must still be completable: %v", err)
	}
}

func TestValidateCompleteEntryIntentDigestRejectsForgedIntents(t *testing.T) {
	command := completeEntryCommandFixture(t)
	digest := mustCompleteEntryDigest(t, command)
	decision := mustEntryCompletionDecision(t, command)
	valid := CompleteEntryIntent{
		EntryID:       command.EntryID,
		RequestDigest: digest,
		Command:       command,
		Decision:      decision,
	}

	if err := ValidateCompleteEntryIntentDigest(valid); err != nil {
		t.Fatalf("well formed intent must pass: %v", err)
	}

	cases := map[string]func(*CompleteEntryIntent){
		"digest does not match the command": func(i *CompleteEntryIntent) { i.RequestDigest = "sha256:deadbeef" },
		"empty digest":                      func(i *CompleteEntryIntent) { i.RequestDigest = "" },
		"identity does not match the command": func(i *CompleteEntryIntent) {
			i.EntryID = mustEntryID("entry-2")
		},
		"decision status was recomputed by the adapter": func(i *CompleteEntryIntent) {
			i.Decision.EntryStatus = domainexecution.EntrySucceeded
		},
		"adapter incremented the intent revision itself": func(i *CompleteEntryIntent) {
			i.Decision.NextIntentRevision++
		},
		"adapter incremented the cancellation generation itself": func(i *CompleteEntryIntent) {
			i.Decision.NextCancellationGeneration++
		},
		"adapter rewrote the next intent": func(i *CompleteEntryIntent) {
			i.Decision.NextIntent = TerminalIntentNone
		},
		"adapter rewrote the observed current revision": func(i *CompleteEntryIntent) {
			i.Decision.CurrentIntentRevision--
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			intent := valid
			mutate(&intent)
			if err := ValidateCompleteEntryIntentDigest(intent); !fault.IsCode(err, CodeCompleteEntryDigestMismatch) {
				t.Fatalf("want %q, got %v", CodeCompleteEntryDigestMismatch, err)
			}
		})
	}
}

func TestEntryCompletionServiceAppliesOnceThenReplays(t *testing.T) {
	transaction := newCompletionTransactionFixture()
	service := mustEntryCompletionService(t, transaction)
	command := completeEntryCommandFixture(t)
	want := mustEntryCompletionDecision(t, command)

	applied, err := service.Complete(context.Background(), command)
	if err != nil {
		t.Fatalf("first completion: %v", err)
	}
	if applied.Status != CompleteEntryApplied {
		t.Fatalf("first completion must apply, got %q", applied.Status)
	}
	if applied.EntryID != command.EntryID {
		t.Fatalf("outcome identity mismatch, got %q want %q", applied.EntryID, command.EntryID)
	}
	if applied.RequestDigest != mustCompleteEntryDigest(t, command) {
		t.Fatalf("outcome digest mismatch, got %q", applied.RequestDigest)
	}
	if !reflect.DeepEqual(applied.Decision, want) {
		t.Fatalf("outcome decision mismatch\n got %+v\nwant %+v", applied.Decision, want)
	}

	replayed, err := service.Complete(context.Background(), command)
	if err != nil {
		t.Fatalf("replayed completion: %v", err)
	}
	if replayed.Status != CompleteEntryReplayed {
		t.Fatalf("second completion must replay, got %q", replayed.Status)
	}
	if !reflect.DeepEqual(replayed.Decision, applied.Decision) {
		t.Fatalf("replay must return the recorded decision\n got %+v\nwant %+v", replayed.Decision, applied.Decision)
	}
	if transaction.applies != 1 {
		t.Fatalf("replay must not apply again, applies=%d", transaction.applies)
	}
}

// TestEntryCompletionServiceHandsTheAdapterTheCoreDecision is the mechanical
// half of "宿主不得推算": the intent the adapter receives already carries the
// exact triple it must persist.
func TestEntryCompletionServiceHandsTheAdapterTheCoreDecision(t *testing.T) {
	transaction := newCompletionTransactionFixture()
	service := mustEntryCompletionService(t, transaction)
	command := completeEntryCommandFixture(t)

	if _, err := service.Complete(context.Background(), command); err != nil {
		t.Fatalf("complete: %v", err)
	}

	want := mustEntryCompletionDecision(t, command)
	if !reflect.DeepEqual(transaction.lastIntent.Decision, want) {
		t.Fatalf("adapter must receive the core decision verbatim\n got %+v\nwant %+v", transaction.lastIntent.Decision, want)
	}
	if !reflect.DeepEqual(transaction.lastIntent.Command, command) {
		t.Fatalf("adapter must receive the command verbatim")
	}
	if transaction.lastIntent.RequestDigest != mustCompleteEntryDigest(t, command) {
		t.Fatalf("adapter must receive the command digest")
	}
}

func TestEntryCompletionServiceAcceptsAConcurrentReplayFromTheApplyPath(t *testing.T) {
	transaction := newCompletionTransactionFixture()
	transaction.mutate = func(outcome CompleteEntryOutcome) CompleteEntryOutcome {
		outcome.Status = CompleteEntryReplayed
		return outcome
	}
	service := mustEntryCompletionService(t, transaction)
	command := completeEntryCommandFixture(t)

	outcome, err := service.Complete(context.Background(), command)
	if err != nil {
		t.Fatalf("an adapter that discovers a duplicate inside the transaction must be accepted: %v", err)
	}
	if outcome.Status != CompleteEntryReplayed {
		t.Fatalf("want %q, got %q", CompleteEntryReplayed, outcome.Status)
	}
}

func TestEntryCompletionServiceRejectsAdapterOutcomeDeviations(t *testing.T) {
	command := completeEntryCommandFixture(t)
	decision := mustEntryCompletionDecision(t, command)

	cases := map[string]struct {
		mutate     func(CompleteEntryOutcome) CompleteEntryOutcome
		lookupPath bool
	}{
		"unknown status": {
			mutate: func(o CompleteEntryOutcome) CompleteEntryOutcome { o.Status = "DONE"; return o },
		},
		"empty status": {
			mutate: func(o CompleteEntryOutcome) CompleteEntryOutcome { o.Status = ""; return o },
		},
		"wrong entry identity": {
			mutate: func(o CompleteEntryOutcome) CompleteEntryOutcome {
				o.EntryID = mustEntryID("entry-2")
				return o
			},
		},
		"wrong request digest": {
			mutate: func(o CompleteEntryOutcome) CompleteEntryOutcome {
				o.RequestDigest = "sha256:deadbeef"
				return o
			},
		},
		"adapter rewrote the terminal status": {
			mutate: func(o CompleteEntryOutcome) CompleteEntryOutcome {
				o.Decision.EntryStatus = domainexecution.EntrySucceeded
				return o
			},
		},
		"adapter invented the next intent revision": {
			mutate: func(o CompleteEntryOutcome) CompleteEntryOutcome {
				o.Decision.NextIntentRevision = decision.NextIntentRevision + 1
				return o
			},
		},
		"adapter invented the next cancellation generation": {
			mutate: func(o CompleteEntryOutcome) CompleteEntryOutcome {
				o.Decision.NextCancellationGeneration = decision.NextCancellationGeneration + 1
				return o
			},
		},
		"adapter reported APPLIED on the replay path": {
			mutate: func(o CompleteEntryOutcome) CompleteEntryOutcome {
				o.Status = CompleteEntryApplied
				return o
			},
			lookupPath: true,
		},
		"adapter rewrote the decision on the replay path": {
			mutate: func(o CompleteEntryOutcome) CompleteEntryOutcome {
				o.Decision.NextIntent = TerminalIntentNone
				return o
			},
			lookupPath: true,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			transaction := newCompletionTransactionFixture()
			service := mustEntryCompletionService(t, transaction)
			if testCase.lookupPath {
				if _, err := service.Complete(context.Background(), command); err != nil {
					t.Fatalf("seed apply: %v", err)
				}
			}
			transaction.mutate = testCase.mutate

			outcome, err := service.Complete(context.Background(), command)
			if !fault.IsCode(err, CodeCompleteEntryAdapterContractViolation) {
				t.Fatalf("want %q, got %v", CodeCompleteEntryAdapterContractViolation, err)
			}
			if !reflect.DeepEqual(outcome, CompleteEntryOutcome{}) {
				t.Fatalf("refused outcome must be zero, got %+v", outcome)
			}
		})
	}
}

func TestEntryCompletionServiceRefusesToWorkWithoutATransaction(t *testing.T) {
	cases := map[string]EntryCompletionTransaction{
		"nil interface": nil,
		"typed nil":     (*completionTransactionFixture)(nil),
	}
	for name, transaction := range cases {
		t.Run(name, func(t *testing.T) {
			service := NewEntryCompletionService(transaction)
			_, err := service.Complete(context.Background(), completeEntryCommandFixture(t))
			if !fault.IsCode(err, CodeCompleteEntryUnavailable) {
				t.Fatalf("want %q, got %v", CodeCompleteEntryUnavailable, err)
			}
		})
	}
}

func TestEntryCompletionServiceWrapsAdapterFailures(t *testing.T) {
	cause := errors.New("sqlite is busy")

	t.Run("lookup", func(t *testing.T) {
		transaction := newCompletionTransactionFixture()
		transaction.lookupErr = cause
		_, err := mustEntryCompletionService(t, transaction).Complete(context.Background(), completeEntryCommandFixture(t))
		if !errors.Is(err, cause) {
			t.Fatalf("adapter failure must reach the caller, got %v", err)
		}
		if transaction.applies != 0 {
			t.Fatalf("a failed lookup must not apply, applies=%d", transaction.applies)
		}
	})

	t.Run("apply", func(t *testing.T) {
		transaction := newCompletionTransactionFixture()
		transaction.applyErr = cause
		_, err := mustEntryCompletionService(t, transaction).Complete(context.Background(), completeEntryCommandFixture(t))
		if !errors.Is(err, cause) {
			t.Fatalf("adapter failure must reach the caller, got %v", err)
		}
	})
}

func TestEntryCompletionServiceRefusesUndecidableCommandsBeforeTouchingTheAdapter(t *testing.T) {
	cases := map[string]struct {
		mutate func(*CompleteEntryCommand)
		code   fault.Code
	}{
		"entry is not running": {
			mutate: func(c *CompleteEntryCommand) { c.State.EntryStatus = domainexecution.EntryPending },
			code:   CodeEntryCompletionNotRunning,
		},
		"malformed command": {
			mutate: func(c *CompleteEntryCommand) { c.Fence.ClaimToken = "" },
			code:   CodeCompleteEntryCommandInvalid,
		},
		"revision has no successor": {
			mutate: func(c *CompleteEntryCommand) { c.State.TerminalIntentRevision = MaxExpectedEntryCompletionRevision },
			code:   CodeEntryCompletionRevisionExhausted,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			transaction := newCompletionTransactionFixture()
			command := completeEntryCommandFixture(t)
			testCase.mutate(&command)

			_, err := mustEntryCompletionService(t, transaction).Complete(context.Background(), command)
			if !fault.IsCode(err, testCase.code) {
				t.Fatalf("want %q, got %v", testCase.code, err)
			}
			if transaction.lookups != 0 || transaction.applies != 0 {
				t.Fatalf("an undecidable command must not reach the adapter, lookups=%d applies=%d", transaction.lookups, transaction.applies)
			}
		})
	}
}
