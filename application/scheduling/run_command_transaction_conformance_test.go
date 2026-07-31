package scheduling

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

type referenceRun struct {
	run      domainexecution.Run
	revision int64
	scope    string
	claimed  bool
	fence    domainexecution.WorkerFence
}
type storedCommand struct {
	digest string
	run    RunCommandResult
	queue  ReorderQueueResult
}
type referenceState struct {
	runs          map[string]referenceRun
	queueRevision map[string]int64
	commands      map[string]storedCommand
}
type referenceCommandStore struct {
	mu               sync.Mutex
	state            referenceState
	failBeforeCommit bool
	unknownCommit    bool
}

func newReferenceCommandStore(runs ...referenceRun) *referenceCommandStore {
	state := referenceState{runs: map[string]referenceRun{}, queueRevision: map[string]int64{}, commands: map[string]storedCommand{}}
	for _, run := range runs {
		state.runs[run.run.ID] = run
	}
	return &referenceCommandStore{state: state}
}
func cloneReferenceState(state referenceState) referenceState {
	clone := referenceState{runs: make(map[string]referenceRun, len(state.runs)), queueRevision: make(map[string]int64, len(state.queueRevision)), commands: make(map[string]storedCommand, len(state.commands))}
	for id, run := range state.runs {
		clone.runs[id] = run
	}
	for id, revision := range state.queueRevision {
		clone.queueRevision[id] = revision
	}
	for id, command := range state.commands {
		command.queue.RunIDs = append([]string(nil), command.queue.RunIDs...)
		clone.commands[id] = command
	}
	return clone
}
func (s *referenceCommandStore) transact(apply func(*referenceState) (storedCommand, error)) (storedCommand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneReferenceState(s.state)
	result, err := apply(&next)
	if err != nil {
		return storedCommand{}, err
	}
	if s.failBeforeCommit {
		return storedCommand{}, errors.New("modeled write failure")
	}
	s.state = next
	if s.unknownCommit {
		return storedCommand{}, errors.New("unknown commit result")
	}
	return result, nil
}
func replay(state *referenceState, id, digest string) (storedCommand, bool, error) {
	stored, ok := state.commands[id]
	if !ok {
		return storedCommand{}, false, nil
	}
	if stored.digest != digest {
		return storedCommand{}, true, runCommandConflictError()
	}
	stored.run.WasApplied, stored.queue.WasApplied = false, false
	return stored, true, nil
}
func (s *referenceCommandStore) Cancel(_ context.Context, command CancelRunCommand) (RunCommandResult, error) {
	digest, _ := CancelRunRequestDigest(command)
	stored, err := s.transact(func(state *referenceState) (storedCommand, error) {
		if prior, ok, err := replay(state, command.CommandID, digest); ok || err != nil {
			return prior, err
		}
		record, ok := state.runs[command.RunID]
		if !ok {
			return storedCommand{}, runIdentityConflictError()
		}
		if record.revision != command.ExpectedRevision {
			return storedCommand{}, runRevisionConflictError()
		}
		if record.run.Status != command.ExpectedStatus {
			return storedCommand{}, runStatusConflictError()
		}
		record.run.Status, record.run.FinishedAt, record.revision = domainexecution.Canceled, command.At, record.revision+1
		record.claimed, record.fence = false, domainexecution.WorkerFence{}
		state.runs[command.RunID] = record
		if command.ExpectedStatus == domainexecution.Queued {
			remaining := make([]string, 0)
			for id, queued := range state.runs {
				if id != command.RunID && queued.scope == record.scope && queued.run.Status == domainexecution.Queued && !queued.claimed {
					remaining = append(remaining, id)
				}
			}
			sort.Slice(remaining, func(i, j int) bool {
				return state.runs[remaining[i]].run.QueuePosition < state.runs[remaining[j]].run.QueuePosition
			})
			for position, id := range remaining {
				queued := state.runs[id]
				queued.run.QueuePosition = position
				state.runs[id] = queued
			}
			state.queueRevision[record.scope]++
		}
		result := RunCommandResult{Run: record.run, Revision: record.revision, WasApplied: true, SignalRequired: command.ExpectedStatus == domainexecution.Running}
		stored := storedCommand{digest: digest, run: result}
		state.commands[command.CommandID] = stored
		return stored, nil
	})
	return stored.run, err
}
func (s *referenceCommandStore) Abort(_ context.Context, command AbortRunCommand) (RunCommandResult, error) {
	digest, _ := AbortRunRequestDigest(command)
	stored, err := s.transact(func(state *referenceState) (storedCommand, error) {
		if prior, ok, err := replay(state, command.CommandID, digest); ok || err != nil {
			return prior, err
		}
		record, ok := state.runs[command.RunID]
		if !ok {
			return storedCommand{}, runIdentityConflictError()
		}
		if record.fence != command.Fence {
			return storedCommand{}, domainexecution.NewStaleWorkerFenceError()
		}
		if record.revision != command.ExpectedRevision {
			return storedCommand{}, runRevisionConflictError()
		}
		if record.run.Status != domainexecution.Running {
			return storedCommand{}, runStatusConflictError()
		}
		record.run.Status, record.run.FinishedAt, record.revision = domainexecution.Aborted, command.At, record.revision+1
		record.claimed, record.fence = false, domainexecution.WorkerFence{}
		state.runs[command.RunID] = record
		result := RunCommandResult{Run: record.run, Revision: record.revision, WasApplied: true, SignalRequired: true}
		stored := storedCommand{digest: digest, run: result}
		state.commands[command.CommandID] = stored
		return stored, nil
	})
	return stored.run, err
}
func (s *referenceCommandStore) Reorder(_ context.Context, command ReorderQueueCommand) (ReorderQueueResult, error) {
	digest, _ := ReorderQueueRequestDigest(command)
	stored, err := s.transact(func(state *referenceState) (storedCommand, error) {
		if prior, ok, err := replay(state, command.CommandID, digest); ok || err != nil {
			return prior, err
		}
		actual := state.queueRevision[command.ScopeID]
		if actual != command.ExpectedRevision {
			return storedCommand{}, queueRevisionConflictError()
		}
		eligible := map[string]struct{}{}
		for id, record := range state.runs {
			if record.scope == command.ScopeID && record.run.Status == domainexecution.Queued && !record.claimed {
				eligible[id] = struct{}{}
			}
		}
		if len(eligible) != len(command.RunIDs) {
			return storedCommand{}, queueMembershipConflictError()
		}
		for index, id := range command.RunIDs {
			if _, ok := eligible[id]; !ok {
				return storedCommand{}, queueMembershipConflictError()
			}
			record := state.runs[id]
			record.run.QueuePosition = index
			state.runs[id] = record
			delete(eligible, id)
		}
		if len(eligible) != 0 {
			return storedCommand{}, queueMembershipConflictError()
		}
		state.queueRevision[command.ScopeID] = actual + 1
		result := ReorderQueueResult{ScopeID: command.ScopeID, Revision: actual + 1, RunIDs: append([]string(nil), command.RunIDs...), WasApplied: true}
		stored := storedCommand{digest: digest, queue: result}
		state.commands[command.CommandID] = stored
		return stored, nil
	})
	return stored.queue, err
}
func (s *referenceCommandStore) claim(runID string, expectedQueueRevision int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneReferenceState(s.state)
	record := next.runs[runID]
	if record.run.Status != domainexecution.Queued || record.claimed || next.queueRevision[record.scope] != expectedQueueRevision {
		return errors.New("claim conflict")
	}
	record.run.Status, record.revision, record.claimed = domainexecution.Running, record.revision+1, true
	record.fence = domainexecution.WorkerFence{RunID: runID, ClaimToken: "claim"}
	next.runs[runID] = record
	next.queueRevision[record.scope]++
	s.state = next
	return nil
}

func TestQueuedCancelAtomicallyRemovesAndNormalizesQueue(t *testing.T) {
	store := newReferenceCommandStore(
		referenceRun{run: domainexecution.Run{ID: "a", Status: domainexecution.Queued, QueuePosition: 0}, revision: 1, scope: "scope"},
		referenceRun{run: domainexecution.Run{ID: "b", Status: domainexecution.Queued, QueuePosition: 1}, revision: 1, scope: "scope"},
		referenceRun{run: domainexecution.Run{ID: "c", Status: domainexecution.Queued, QueuePosition: 2}, revision: 1, scope: "scope"},
	)
	result, err := store.Cancel(context.Background(), CancelRunCommand{CommandID: "cancel", RunID: "b", ExpectedStatus: domainexecution.Queued, ExpectedRevision: 1, At: 2})
	if err != nil || !result.WasApplied || store.state.queueRevision["scope"] != 1 || store.state.runs["a"].run.QueuePosition != 0 || store.state.runs["c"].run.QueuePosition != 1 {
		t.Fatalf("result/state=%#v/%#v", result, store.state)
	}
	if err := store.claim("a", 0); err == nil {
		t.Fatal("stale pre-cancel claim revision succeeded")
	}
	stale := ReorderQueueCommand{CommandID: "stale", ScopeID: "scope", ExpectedRevision: 0, RunIDs: []string{"c", "a"}}
	if _, err := store.Reorder(context.Background(), stale); !fault.IsCode(err, CodeQueueRevisionConflict) {
		t.Fatalf("stale reorder=%v", err)
	}
	fresh := stale
	fresh.CommandID, fresh.ExpectedRevision = "fresh", 1
	if _, err := store.Reorder(context.Background(), fresh); err != nil {
		t.Fatalf("fresh reorder=%v", err)
	}
}

func TestRunningCancelDoesNotChangeQueueRevision(t *testing.T) {
	fence := domainexecution.WorkerFence{RunID: "run", ClaimToken: "token"}
	store := newReferenceCommandStore(referenceRun{run: domainexecution.Run{ID: "run", Status: domainexecution.Running}, revision: 1, scope: "scope", claimed: true, fence: fence})
	store.state.queueRevision["scope"] = 4
	_, err := store.Cancel(context.Background(), CancelRunCommand{CommandID: "cancel", RunID: "run", ExpectedStatus: domainexecution.Running, ExpectedRevision: 1, At: 2})
	if err != nil || store.state.queueRevision["scope"] != 4 || store.state.runs["run"].fence != (domainexecution.WorkerFence{}) {
		t.Fatalf("revision/state/error=%d/%#v/%v", store.state.queueRevision["scope"], store.state.runs["run"], err)
	}
}

func TestReferenceStoreReplayConflictRollbackAndUnknownCommit(t *testing.T) {
	store := newReferenceCommandStore(referenceRun{run: domainexecution.Run{ID: "run", Status: domainexecution.Queued}, revision: 1, scope: "scope"})
	command := CancelRunCommand{CommandID: "cancel", RunID: "run", ExpectedStatus: domainexecution.Queued, ExpectedRevision: 1, At: 2}
	first, err := store.Cancel(context.Background(), command)
	if err != nil || !first.WasApplied {
		t.Fatalf("first=%#v/%v", first, err)
	}
	replay, err := store.Cancel(context.Background(), command)
	if err != nil || replay.WasApplied || replay.Run.Status != domainexecution.Canceled {
		t.Fatalf("replay=%#v/%v", replay, err)
	}
	changed := command
	changed.At++
	if _, err := store.Cancel(context.Background(), changed); !fault.IsCode(err, CodeRunCommandIdentityConflict) {
		t.Fatalf("conflict=%v", err)
	}
	rollback := newReferenceCommandStore(referenceRun{run: domainexecution.Run{ID: "rollback", Status: domainexecution.Queued}, revision: 1})
	rollback.failBeforeCommit = true
	if _, err := rollback.Cancel(context.Background(), CancelRunCommand{CommandID: "c", RunID: "rollback", ExpectedStatus: domainexecution.Queued, ExpectedRevision: 1, At: 2}); err == nil || rollback.state.runs["rollback"].run.Status != domainexecution.Queued || len(rollback.state.commands) != 0 {
		t.Fatal("failed transaction did not roll back")
	}
	unknown := newReferenceCommandStore(referenceRun{run: domainexecution.Run{ID: "unknown", Status: domainexecution.Queued}, revision: 1})
	unknown.unknownCommit = true
	unknownCommand := CancelRunCommand{CommandID: "u", RunID: "unknown", ExpectedStatus: domainexecution.Queued, ExpectedRevision: 1, At: 2}
	if _, err := unknown.Cancel(context.Background(), unknownCommand); err == nil {
		t.Fatal("expected unknown result")
	}
	unknown.unknownCommit = false
	if result, err := unknown.Cancel(context.Background(), unknownCommand); err != nil || result.WasApplied {
		t.Fatalf("reconciled=%#v/%v", result, err)
	}
}

func TestReferenceStoreAbortFenceReplayAndCompetingTerminalRaces(t *testing.T) {
	fence := domainexecution.WorkerFence{RunID: "run", ClaimToken: "token"}
	store := newReferenceCommandStore(referenceRun{run: domainexecution.Run{ID: "run", Status: domainexecution.Running}, revision: 1, claimed: true, fence: fence})
	command := AbortRunCommand{CommandID: "abort", RunID: "run", ExpectedRevision: 1, At: 2, Fence: fence}
	first, err := store.Abort(context.Background(), command)
	if err != nil || !first.WasApplied {
		t.Fatalf("abort=%#v/%v", first, err)
	}
	replay, err := store.Abort(context.Background(), command)
	if err != nil || replay.WasApplied {
		t.Fatalf("replay=%#v/%v", replay, err)
	}
	stale := command
	stale.CommandID = "stale"
	if _, err := store.Abort(context.Background(), stale); !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) {
		t.Fatalf("stale=%v", err)
	}

	for _, competing := range []domainexecution.RunStatus{domainexecution.Succeeded, domainexecution.Failed, domainexecution.Canceled} {
		t.Run(string(competing), func(t *testing.T) {
			race := newReferenceCommandStore(referenceRun{run: domainexecution.Run{ID: "run", Status: domainexecution.Running}, revision: 1, claimed: true, fence: fence})
			start := make(chan struct{})
			outcomes := make(chan bool, 2)
			go func() { <-start; _, err := race.Abort(context.Background(), command); outcomes <- err == nil }()
			go func() {
				<-start
				race.mu.Lock()
				next := cloneReferenceState(race.state)
				record := next.runs["run"]
				won := record.run.Status == domainexecution.Running
				if won {
					record.run.Status = competing
					record.revision++
					record.claimed = false
					record.fence = domainexecution.WorkerFence{}
					next.runs["run"] = record
					race.state = next
				}
				race.mu.Unlock()
				outcomes <- won
			}()
			close(start)
			if (<-outcomes) == (<-outcomes) {
				t.Fatal("race must have exactly one winner")
			}
		})
	}
}

func TestReferenceStoreCancelClaimAbortAndDuplicateRaces(t *testing.T) {
	queued := func() *referenceCommandStore {
		return newReferenceCommandStore(referenceRun{run: domainexecution.Run{ID: "run", Status: domainexecution.Queued}, revision: 1, scope: "scope"})
	}
	cancelQueued := CancelRunCommand{CommandID: "cancel", RunID: "run", ExpectedStatus: domainexecution.Queued, ExpectedRevision: 1, At: 2}
	race := queued()
	start := make(chan struct{})
	outcomes := make(chan bool, 2)
	go func() { <-start; _, err := race.Cancel(context.Background(), cancelQueued); outcomes <- err == nil }()
	go func() { <-start; outcomes <- race.claim("run", 0) == nil }()
	close(start)
	if (<-outcomes) == (<-outcomes) {
		t.Fatal("cancel/claim must have exactly one winner")
	}

	fence := domainexecution.WorkerFence{RunID: "run", ClaimToken: "token"}
	active := func() *referenceCommandStore {
		return newReferenceCommandStore(referenceRun{run: domainexecution.Run{ID: "run", Status: domainexecution.Running}, revision: 1, claimed: true, fence: fence})
	}
	cancelActive := CancelRunCommand{CommandID: "cancel", RunID: "run", ExpectedStatus: domainexecution.Running, ExpectedRevision: 1, At: 2}
	abort := AbortRunCommand{CommandID: "abort", RunID: "run", ExpectedRevision: 1, At: 2, Fence: fence}
	race = active()
	start = make(chan struct{})
	outcomes = make(chan bool, 2)
	go func() { <-start; _, err := race.Cancel(context.Background(), cancelActive); outcomes <- err == nil }()
	go func() { <-start; _, err := race.Abort(context.Background(), abort); outcomes <- err == nil }()
	close(start)
	if (<-outcomes) == (<-outcomes) {
		t.Fatal("cancel/abort must have exactly one winner")
	}
	stale := abort
	stale.CommandID = "stale"
	if _, err := race.Abort(context.Background(), stale); err == nil {
		t.Fatal("old fence mutated terminal winner")
	}

	for _, completed := range []domainexecution.RunStatus{domainexecution.Succeeded, domainexecution.Failed} {
		t.Run("cancel-vs-"+string(completed), func(t *testing.T) {
			race := active()
			start := make(chan struct{})
			outcomes := make(chan bool, 2)
			go func() { <-start; _, err := race.Cancel(context.Background(), cancelActive); outcomes <- err == nil }()
			go func() {
				<-start
				race.mu.Lock()
				next := cloneReferenceState(race.state)
				record := next.runs["run"]
				won := record.run.Status == domainexecution.Running && record.revision == 1 && record.fence == fence
				if won {
					record.run.Status, record.revision, record.claimed, record.fence = completed, 2, false, domainexecution.WorkerFence{}
					next.runs["run"] = record
					race.state = next
				}
				race.mu.Unlock()
				outcomes <- won
			}()
			close(start)
			if (<-outcomes) == (<-outcomes) {
				t.Fatal("cancel/complete must have exactly one winner")
			}
		})
	}

	for _, operation := range []string{"cancel", "abort"} {
		t.Run("duplicate-"+operation, func(t *testing.T) {
			store := active()
			start := make(chan struct{})
			results := make(chan RunCommandResult, 2)
			errorsFound := make(chan error, 2)
			invoke := func() {
				<-start
				var result RunCommandResult
				var err error
				if operation == "cancel" {
					result, err = store.Cancel(context.Background(), cancelActive)
				} else {
					result, err = store.Abort(context.Background(), abort)
				}
				results <- result
				errorsFound <- err
			}
			go invoke()
			go invoke()
			close(start)
			first, second := <-results, <-results
			if err := <-errorsFound; err != nil {
				t.Fatal(err)
			}
			if err := <-errorsFound; err != nil {
				t.Fatal(err)
			}
			if first.WasApplied == second.WasApplied || first.Run.Status != second.Run.Status || first.Revision != second.Revision {
				t.Fatalf("duplicate results=%#v/%#v", first, second)
			}
		})
	}
}

func TestReferenceStoreReorderExactPermutationReplayRollbackAndClaimRace(t *testing.T) {
	makeStore := func() *referenceCommandStore {
		return newReferenceCommandStore(referenceRun{run: domainexecution.Run{ID: "a", Status: domainexecution.Queued}, revision: 1, scope: "scope"}, referenceRun{run: domainexecution.Run{ID: "b", Status: domainexecution.Queued}, revision: 1, scope: "scope"})
	}
	command := ReorderQueueCommand{CommandID: "reorder", ScopeID: "scope", ExpectedRevision: 0, RunIDs: []string{"b", "a"}}
	store := makeStore()
	first, err := store.Reorder(context.Background(), command)
	if err != nil || !first.WasApplied || store.state.runs["b"].run.QueuePosition != 0 {
		t.Fatalf("first=%#v/%v", first, err)
	}
	replay, err := store.Reorder(context.Background(), command)
	if err != nil || replay.WasApplied {
		t.Fatalf("replay=%#v/%v", replay, err)
	}
	changed := command
	changed.RunIDs = []string{"a", "b"}
	if _, err := store.Reorder(context.Background(), changed); !fault.IsCode(err, CodeRunCommandIdentityConflict) {
		t.Fatalf("conflict=%v", err)
	}
	for _, invalid := range [][]string{{"a"}, {"a", "foreign"}} {
		bad := makeStore()
		badCommand := command
		badCommand.CommandID = "bad"
		badCommand.RunIDs = invalid
		if _, err := bad.Reorder(context.Background(), badCommand); !fault.IsCode(err, CodeQueueMembershipConflict) {
			t.Fatalf("members %v: %v", invalid, err)
		}
	}
	claimed := makeStore()
	record := claimed.state.runs["a"]
	record.claimed = true
	claimed.state.runs["a"] = record
	if _, err := claimed.Reorder(context.Background(), command); !fault.IsCode(err, CodeQueueMembershipConflict) {
		t.Fatalf("claimed=%v", err)
	}
	nonqueued := makeStore()
	record = nonqueued.state.runs["a"]
	record.run.Status = domainexecution.Succeeded
	nonqueued.state.runs["a"] = record
	if _, err := nonqueued.Reorder(context.Background(), command); !fault.IsCode(err, CodeQueueMembershipConflict) {
		t.Fatalf("nonqueued=%v", err)
	}
	stale := makeStore()
	stale.state.queueRevision["scope"] = 2
	if _, err := stale.Reorder(context.Background(), command); !fault.IsCode(err, CodeQueueRevisionConflict) {
		t.Fatalf("stale=%v", err)
	}
	rollback := makeStore()
	before := cloneReferenceState(rollback.state)
	rollback.failBeforeCommit = true
	_, _ = rollback.Reorder(context.Background(), command)
	if !reflect.DeepEqual(before, rollback.state) {
		t.Fatal("reorder rollback changed state")
	}
	race := makeStore()
	start := make(chan struct{})
	outcomes := make(chan bool, 2)
	go func() { <-start; _, err := race.Reorder(context.Background(), command); outcomes <- err == nil }()
	go func() { <-start; outcomes <- race.claim("a", 0) == nil }()
	close(start)
	if (<-outcomes) == (<-outcomes) {
		t.Fatal("reorder/claim must have exactly one winner")
	}
}
