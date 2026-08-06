package execution

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/application/engine"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// allExecutionOutcomes, allRecordingOutcomes and allTimelineOutcomes spell the
// engine vocabulary out by hand. A new engine constant that nobody adds here
// leaves the matrix below silently narrower, so the inventory guard test
// asserts the counts separately.
func allExecutionOutcomes() []engine.ExecutionOutcome {
	return []engine.ExecutionOutcome{
		engine.OutcomeSucceeded,
		engine.OutcomeFailed,
		engine.OutcomeCanceled,
		engine.ExecutionNotStarted,
	}
}

func allRecordingOutcomes() []engine.RecordingOutcome {
	return []engine.RecordingOutcome{
		engine.RecordingDisabled,
		engine.RecordingSucceeded,
		engine.RecordingStartFailed,
		engine.RecordingStopFailed,
	}
}

func allTimelineOutcomes() []engine.TimelineOutcome {
	return []engine.TimelineOutcome{
		engine.TimelineDisabled,
		engine.TimelineComplete,
		engine.TimelineStartFailed,
		engine.TimelineFinishFailed,
	}
}

func allTerminalIntents() []TerminalIntent {
	return []TerminalIntent{TerminalIntentNone, TerminalIntentCancel, TerminalIntentAbort}
}

func allEntryStatuses() []domainexecution.EntryStatus {
	return []domainexecution.EntryStatus{
		domainexecution.EntryPending,
		domainexecution.EntryRunning,
		domainexecution.EntrySucceeded,
		domainexecution.EntryFailed,
		domainexecution.EntryCanceled,
		domainexecution.EntryAborted,
		domainexecution.EntrySkipped,
	}
}

// entryCompletionStatusMatrix is the D-12 decision written as data rather than
// as a second copy of the algorithm. Every execution outcome crossed with every
// terminal intent has a row; a missing row fails the test instead of inheriting
// a neighbour's answer.
var entryCompletionStatusMatrix = map[engine.ExecutionOutcome]map[TerminalIntent]domainexecution.EntryStatus{
	engine.OutcomeSucceeded: {
		// 裁决一：引擎已经跑完，事实压过意图。外部副作用已经落地，
		// 记 CANCELED/ABORTED 就是在记录里撒谎。
		TerminalIntentNone:   domainexecution.EntrySucceeded,
		TerminalIntentCancel: domainexecution.EntrySucceeded,
		TerminalIntentAbort:  domainexecution.EntrySucceeded,
	},
	engine.OutcomeFailed: {
		TerminalIntentNone:   domainexecution.EntryFailed,
		TerminalIntentCancel: domainexecution.EntryCanceled,
		TerminalIntentAbort:  domainexecution.EntryAborted,
	},
	engine.OutcomeCanceled: {
		// 没有意图时的引擎取消是一次失败：Entry 已经 RUNNING，
		// 不落终态就永远释放不了租约。
		TerminalIntentNone:   domainexecution.EntryFailed,
		TerminalIntentCancel: domainexecution.EntryCanceled,
		TerminalIntentAbort:  domainexecution.EntryAborted,
	},
	engine.ExecutionNotStarted: {
		TerminalIntentNone:   domainexecution.EntryFailed,
		TerminalIntentCancel: domainexecution.EntryCanceled,
		TerminalIntentAbort:  domainexecution.EntryAborted,
	},
}

// entryCompletionGenerationBump records whether the terminal status means the
// intent was actually carried out. Only a carried-out intent advances the
// cancellation generation.
var entryCompletionGenerationBump = map[domainexecution.EntryStatus]int64{
	domainexecution.EntrySucceeded: 0,
	domainexecution.EntryFailed:    0,
	domainexecution.EntryCanceled:  1,
	domainexecution.EntryAborted:   1,
}

func wantEntryCompletionDecision(t *testing.T, state EntryCompletionState, result engine.EntryResult) EntryCompletionDecision {
	t.Helper()
	byIntent, ok := entryCompletionStatusMatrix[result.ExecutionOutcome]
	if !ok {
		t.Fatalf("decision matrix has no row for execution outcome %q", result.ExecutionOutcome)
	}
	status, ok := byIntent[state.TerminalIntent]
	if !ok {
		t.Fatalf("decision matrix has no cell for execution outcome %q and terminal intent %q", result.ExecutionOutcome, state.TerminalIntent)
	}
	bump, ok := entryCompletionGenerationBump[status]
	if !ok {
		t.Fatalf("generation table has no row for terminal status %q", status)
	}
	return EntryCompletionDecision{
		EntryStatus:                   status,
		CurrentIntent:                 state.TerminalIntent,
		CurrentIntentRevision:         state.TerminalIntentRevision,
		CurrentCancellationGeneration: state.CancellationGeneration,
		NextIntent:                    state.TerminalIntent,
		NextIntentRevision:            state.TerminalIntentRevision + 1,
		NextCancellationGeneration:    state.CancellationGeneration + bump,
	}
}

func runningCompletionState(intent TerminalIntent) EntryCompletionState {
	return EntryCompletionState{
		EntryStatus:            domainexecution.EntryRunning,
		TerminalIntent:         intent,
		TerminalIntentRevision: 7,
		CancellationGeneration: 3,
	}
}

func engineOutcome(exec engine.ExecutionOutcome, recording engine.RecordingOutcome, timeline engine.TimelineOutcome) EngineOutcome {
	return EngineOutcome{
		Result: engine.EntryResult{
			ExecutionOutcome: exec,
			RecordingOutcome: recording,
			TimelineOutcome:  timeline,
		},
	}
}

func TestDecideEntryCompletionCoversEveryOutcomeIntentAndStartingStatus(t *testing.T) {
	for _, status := range allEntryStatuses() {
		for _, intent := range allTerminalIntents() {
			for _, exec := range allExecutionOutcomes() {
				for _, recording := range allRecordingOutcomes() {
					for _, timeline := range allTimelineOutcomes() {
						name := fmt.Sprintf("%s-%s-%s-%s-%s", status, intent, exec, recording, timeline)
						t.Run(name, func(t *testing.T) {
							state := runningCompletionState(intent)
							state.EntryStatus = status
							outcome := engineOutcome(exec, recording, timeline)

							decision, err := DecideEntryCompletion(state, outcome)

							if status != domainexecution.EntryRunning {
								if !fault.IsCode(err, CodeEntryCompletionNotRunning) {
									t.Fatalf("starting status %q must be refused with %q, got %v", status, CodeEntryCompletionNotRunning, err)
								}
								if !reflect.DeepEqual(decision, EntryCompletionDecision{}) {
									t.Fatalf("refused decision must be zero, got %+v", decision)
								}
								return
							}
							if err != nil {
								t.Fatalf("running entry must decide, got %v", err)
							}
							want := wantEntryCompletionDecision(t, state, outcome.Result)
							if !reflect.DeepEqual(decision, want) {
								t.Fatalf("decision mismatch\n got %+v\nwant %+v", decision, want)
							}
							if err := domainexecution.ValidateEntryStatusTransition(domainexecution.EntryRunning, decision.EntryStatus); err != nil {
								t.Fatalf("decided status %q is not a legal RUNNING transition: %v", decision.EntryStatus, err)
							}
							if !domainexecution.IsTerminalEntryStatus(decision.EntryStatus) {
								t.Fatalf("decided status %q is not terminal", decision.EntryStatus)
							}
						})
					}
				}
			}
		}
	}
}

func TestDecideEntryCompletionMatrixHasNoUnreachableOrMissingCell(t *testing.T) {
	if len(entryCompletionStatusMatrix) != len(allExecutionOutcomes()) {
		t.Fatalf("matrix covers %d execution outcomes, vocabulary has %d", len(entryCompletionStatusMatrix), len(allExecutionOutcomes()))
	}
	for _, exec := range allExecutionOutcomes() {
		byIntent, ok := entryCompletionStatusMatrix[exec]
		if !ok {
			t.Fatalf("matrix has no row for execution outcome %q", exec)
		}
		if len(byIntent) != len(allTerminalIntents()) {
			t.Fatalf("execution outcome %q covers %d intents, vocabulary has %d", exec, len(byIntent), len(allTerminalIntents()))
		}
		for _, intent := range allTerminalIntents() {
			if _, ok := byIntent[intent]; !ok {
				t.Fatalf("matrix has no cell for execution outcome %q and terminal intent %q", exec, intent)
			}
		}
	}
}

// TestDecideEntryCompletionKeepsSucceededWhenCancelIntentRacesEngineSuccess is
// 裁决一 written as an assertion: a finished entry is SUCCEEDED, and the pending
// intent still travels intact into the Next* fields so DecideAdvance can stop
// the instance.
func TestDecideEntryCompletionKeepsSucceededWhenCancelIntentRacesEngineSuccess(t *testing.T) {
	for _, intent := range []TerminalIntent{TerminalIntentCancel, TerminalIntentAbort} {
		t.Run(string(intent), func(t *testing.T) {
			state := runningCompletionState(intent)
			outcome := engineOutcome(engine.OutcomeSucceeded, engine.RecordingSucceeded, engine.TimelineComplete)

			decision, err := DecideEntryCompletion(state, outcome)
			if err != nil {
				t.Fatalf("decide: %v", err)
			}

			if decision.EntryStatus != domainexecution.EntrySucceeded {
				t.Fatalf("engine success under %q intent must stay SUCCEEDED, got %q", intent, decision.EntryStatus)
			}
			if decision.NextIntent != intent {
				t.Fatalf("intent must survive the completion, got %q want %q", decision.NextIntent, intent)
			}
			if decision.NextIntentRevision != state.TerminalIntentRevision+1 {
				t.Fatalf("intent revision must advance exactly once, got %d want %d", decision.NextIntentRevision, state.TerminalIntentRevision+1)
			}
			if decision.NextCancellationGeneration != state.CancellationGeneration {
				t.Fatalf("an uncarried intent must not consume a cancellation generation, got %d want %d", decision.NextCancellationGeneration, state.CancellationGeneration)
			}
		})
	}
}

// TestDecideEntryCompletionIgnoresAbortPendingCommandIdentity is 裁决二: the
// pending abort command identity is transaction identity, never decision basis.
// Two different command identities that reach the same state must produce
// field-by-field identical decisions.
func TestDecideEntryCompletionIgnoresAbortPendingCommandIdentity(t *testing.T) {
	stateType := reflect.TypeOf(EntryCompletionState{})
	for i := range stateType.NumField() {
		if name := stateType.Field(i).Name; name == "AbortPendingCommandID" {
			t.Fatalf("EntryCompletionState must not carry %s: a pending abort is not a terminal intent", name)
		}
	}

	state := runningCompletionState(TerminalIntentAbort)
	outcome := engineOutcome(engine.OutcomeFailed, engine.RecordingStopFailed, engine.TimelineFinishFailed)

	first, err := DecideEntryCompletion(state, outcome)
	if err != nil {
		t.Fatalf("decide first: %v", err)
	}
	second, err := DecideEntryCompletion(state, outcome)
	if err != nil {
		t.Fatalf("decide second: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("decision must be a pure function of state and outcome\nfirst  %+v\nsecond %+v", first, second)
	}

	base := completeEntryCommandFixture(t)
	base.State = state
	base.Outcome = outcome
	for _, identity := range []string{"", "abort-cmd-a", "abort-cmd-b"} {
		command := base
		command.AbortPendingCommandID = identity
		decision, err := DecideEntryCompletion(command.State, command.Outcome)
		if err != nil {
			t.Fatalf("decide with identity %q: %v", identity, err)
		}
		if !reflect.DeepEqual(decision, first) {
			t.Fatalf("abort pending command identity %q changed the decision\n got %+v\nwant %+v", identity, decision, first)
		}
	}
}

func TestDecideEntryCompletionRejectsMalformedStateAndOutcome(t *testing.T) {
	valid := runningCompletionState(TerminalIntentNone)
	validOutcome := engineOutcome(engine.OutcomeSucceeded, engine.RecordingDisabled, engine.TimelineDisabled)

	cases := []struct {
		name    string
		state   EntryCompletionState
		outcome EngineOutcome
		code    fault.Code
	}{
		{
			name:    "unknown entry status",
			state:   EntryCompletionState{EntryStatus: "MADE_UP", TerminalIntent: TerminalIntentNone},
			outcome: validOutcome,
			code:    CodeEntryCompletionStateInvalid,
		},
		{
			name:    "empty entry status",
			state:   EntryCompletionState{TerminalIntent: TerminalIntentNone},
			outcome: validOutcome,
			code:    CodeEntryCompletionStateInvalid,
		},
		{
			name:    "unknown terminal intent",
			state:   EntryCompletionState{EntryStatus: domainexecution.EntryRunning, TerminalIntent: "STOP"},
			outcome: validOutcome,
			code:    CodeEntryCompletionStateInvalid,
		},
		{
			name:    "empty terminal intent",
			state:   EntryCompletionState{EntryStatus: domainexecution.EntryRunning},
			outcome: validOutcome,
			code:    CodeEntryCompletionStateInvalid,
		},
		{
			name:    "negative intent revision",
			state:   EntryCompletionState{EntryStatus: domainexecution.EntryRunning, TerminalIntent: TerminalIntentNone, TerminalIntentRevision: -1},
			outcome: validOutcome,
			code:    CodeEntryCompletionStateInvalid,
		},
		{
			name:    "negative cancellation generation",
			state:   EntryCompletionState{EntryStatus: domainexecution.EntryRunning, TerminalIntent: TerminalIntentNone, CancellationGeneration: -1},
			outcome: validOutcome,
			code:    CodeEntryCompletionStateInvalid,
		},
		{
			name:    "unknown execution outcome",
			state:   valid,
			outcome: engineOutcome("MADE_UP", engine.RecordingDisabled, engine.TimelineDisabled),
			code:    CodeEngineOutcomeInvalid,
		},
		{
			name:    "unknown recording outcome",
			state:   valid,
			outcome: engineOutcome(engine.OutcomeSucceeded, "MADE_UP", engine.TimelineDisabled),
			code:    CodeEngineOutcomeInvalid,
		},
		{
			name:    "unknown timeline outcome",
			state:   valid,
			outcome: engineOutcome(engine.OutcomeSucceeded, engine.RecordingDisabled, "MADE_UP"),
			code:    CodeEngineOutcomeInvalid,
		},
		{
			name:    "blank failure code",
			state:   valid,
			outcome: EngineOutcome{Result: validOutcome.Result, FailureCode: "  "},
			code:    CodeEngineOutcomeInvalid,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			decision, err := DecideEntryCompletion(testCase.state, testCase.outcome)
			if !fault.IsCode(err, testCase.code) {
				t.Fatalf("want %q, got %v", testCase.code, err)
			}
			if !reflect.DeepEqual(decision, EntryCompletionDecision{}) {
				t.Fatalf("refused decision must be zero, got %+v", decision)
			}
		})
	}
}

func TestDecideEntryCompletionRefusesRevisionsWithoutARepresentableSuccessor(t *testing.T) {
	outcome := engineOutcome(engine.OutcomeFailed, engine.RecordingDisabled, engine.TimelineDisabled)

	cases := []struct {
		name  string
		state EntryCompletionState
	}{
		{
			name: "intent revision at the ceiling",
			state: EntryCompletionState{
				EntryStatus:            domainexecution.EntryRunning,
				TerminalIntent:         TerminalIntentCancel,
				TerminalIntentRevision: MaxExpectedEntryCompletionRevision,
			},
		},
		{
			name: "cancellation generation at the ceiling",
			state: EntryCompletionState{
				EntryStatus:            domainexecution.EntryRunning,
				TerminalIntent:         TerminalIntentCancel,
				CancellationGeneration: MaxExpectedEntryCompletionRevision,
			},
		},
		{
			name: "intent revision beyond the ceiling",
			state: EntryCompletionState{
				EntryStatus:            domainexecution.EntryRunning,
				TerminalIntent:         TerminalIntentCancel,
				TerminalIntentRevision: math.MaxInt64,
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			decision, err := DecideEntryCompletion(testCase.state, outcome)
			if !fault.IsCode(err, CodeEntryCompletionRevisionExhausted) {
				t.Fatalf("want %q, got %v", CodeEntryCompletionRevisionExhausted, err)
			}
			if !reflect.DeepEqual(decision, EntryCompletionDecision{}) {
				t.Fatalf("refused decision must be zero, got %+v", decision)
			}
		})
	}
}

// TestDecideEntryCompletionLeavesEvidenceQualityOutOfTheTerminalStatus pins the
// reachable SUCCEEDED + STOP_FAILED pair: a recorder hiccup degrades the
// evidence, never the run.
func TestDecideEntryCompletionLeavesEvidenceQualityOutOfTheTerminalStatus(t *testing.T) {
	state := runningCompletionState(TerminalIntentNone)
	for _, recording := range allRecordingOutcomes() {
		for _, timeline := range allTimelineOutcomes() {
			t.Run(fmt.Sprintf("%s-%s", recording, timeline), func(t *testing.T) {
				decision, err := DecideEntryCompletion(state, engineOutcome(engine.OutcomeSucceeded, recording, timeline))
				if err != nil {
					t.Fatalf("decide: %v", err)
				}
				if decision.EntryStatus != domainexecution.EntrySucceeded {
					t.Fatalf("recording %q timeline %q must not change the terminal status, got %q", recording, timeline, decision.EntryStatus)
				}
			})
		}
	}
}

func TestDecideEntryCompletionCarriesFailureCodeWithoutChangingTheDecision(t *testing.T) {
	state := runningCompletionState(TerminalIntentNone)
	result := engine.EntryResult{
		ExecutionOutcome: engine.OutcomeFailed,
		RecordingOutcome: engine.RecordingDisabled,
		TimelineOutcome:  engine.TimelineDisabled,
	}

	bare, err := DecideEntryCompletion(state, EngineOutcome{Result: result})
	if err != nil {
		t.Fatalf("decide bare: %v", err)
	}
	coded, err := DecideEntryCompletion(state, EngineOutcome{Result: result, FailureCode: CodeSchedulingAdapterUnavailable})
	if err != nil {
		t.Fatalf("decide coded: %v", err)
	}
	if !reflect.DeepEqual(bare, coded) {
		t.Fatalf("failure code must not steer the decision\nbare  %+v\ncoded %+v", bare, coded)
	}
}
