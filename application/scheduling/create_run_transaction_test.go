package scheduling

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

type conformanceStage string

const (
	stageRun      conformanceStage = "run"
	stageEntries  conformanceStage = "entries"
	stageSnapshot conformanceStage = "snapshot"
	stageQueue    conformanceStage = "queue"
	stageCommand  conformanceStage = "command"
)

type conformanceState struct {
	commands  map[string]StoredCreateRunCommand
	runs      map[string]execution.Run
	entries   map[string][]execution.WorkflowEntry
	inputs    map[string]execution.RunSnapshotInput
	digests   map[string]string
	queue     []string
	positions map[string]int
}

func emptyConformanceState() conformanceState {
	return conformanceState{commands: map[string]StoredCreateRunCommand{}, runs: map[string]execution.Run{}, entries: map[string][]execution.WorkflowEntry{}, inputs: map[string]execution.RunSnapshotInput{}, digests: map[string]string{}, positions: map[string]int{}}
}
func cloneConformanceState(source conformanceState) conformanceState {
	out := emptyConformanceState()
	for key, value := range source.commands {
		out.commands[key] = value
	}
	for key, value := range source.runs {
		out.runs[key] = value
	}
	for key, value := range source.entries {
		out.entries[key] = append([]execution.WorkflowEntry(nil), value...)
	}
	for key, value := range source.inputs {
		out.inputs[key] = value
	}
	for key, value := range source.digests {
		out.digests[key] = value
	}
	out.queue = append([]string(nil), source.queue...)
	for key, value := range source.positions {
		out.positions[key] = value
	}
	return out
}

type conformanceStore struct {
	mu                sync.Mutex
	state             conformanceState
	resolved          ResolvedCreateRun
	fault             conformanceStage
	commitErr         error
	retryOnce         bool
	unknownCommitOnce bool
	attempts          int
}

type conformanceTx struct {
	store *conformanceStore
	state conformanceState
	dirty bool
}

func newConformanceStore(resolved ResolvedCreateRun) *conformanceStore {
	return &conformanceStore{state: emptyConformanceState(), resolved: resolved}
}
func (s *conformanceStore) InTransaction(ctx context.Context, callback func(CreateRunTx) error) (err error) {
transactionAttempt:
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		attempt := cloneConformanceState(s.state)
		s.attempts++
		attemptNumber := s.attempts
		s.mu.Unlock()
		tx := &conformanceTx{store: s, state: attempt}
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					panic(recovered)
				}
			}()
			err = callback(tx)
		}()
		if err != nil {
			return err
		}
		if s.retryOnce && attemptNumber == 1 {
			continue
		}
		if s.commitErr != nil {
			return s.commitErr
		}
		if !tx.dirty {
			return nil
		}
		s.mu.Lock()
		// Reconcile a concurrent winner before publishing the attempt.
		for commandID, command := range tx.state.commands {
			if winner, exists := s.state.commands[commandID]; exists {
				if winner.RequestDigest != command.RequestDigest {
					s.mu.Unlock()
					return createRunCommandConflictError()
				}
				s.mu.Unlock()
				continue transactionAttempt
			}
			if winner, exists := s.state.runs[command.Result.Run.ID]; exists && winner.SnapshotDigest != command.Result.Run.SnapshotDigest {
				s.mu.Unlock()
				return createRunSnapshotConflictError()
			}
		}
		s.state = tx.state
		if s.unknownCommitOnce {
			s.unknownCommitOnce = false
			s.mu.Unlock()
			continue transactionAttempt
		}
		s.mu.Unlock()
		return nil
	}
}
func (tx *conformanceTx) FindCommand(_ context.Context, id string) (StoredCreateRunCommand, bool, error) {
	value, ok := tx.state.commands[id]
	return value, ok, nil
}
func (tx *conformanceTx) ResolveCreateRun(context.Context, CreateRunCommand) (ResolvedCreateRun, error) {
	return tx.store.resolved, nil
}
func (tx *conformanceTx) InsertCreateRun(_ context.Context, intent CreateRunIntent) (InsertCreateRunOutcome, error) {
	if existing, ok := tx.state.commands[intent.CommandID]; ok {
		if existing.RequestDigest != intent.RequestDigest {
			return InsertCreateRunOutcome{}, createRunCommandConflictError()
		}
		return InsertCreateRunOutcome{Status: InsertCreateRunReplayed, CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: existing.Result}, nil
	}
	if existing, ok := tx.state.runs[intent.Run.ID]; ok && existing.SnapshotDigest != intent.Run.SnapshotDigest {
		return InsertCreateRunOutcome{}, createRunSnapshotConflictError()
	}
	fail := func(stage conformanceStage) error {
		if tx.store.fault == stage {
			return fmt.Errorf("fault at %s", stage)
		}
		return nil
	}
	if err := fail(stageRun); err != nil {
		return InsertCreateRunOutcome{}, err
	}
	tx.state.runs[intent.Run.ID] = intent.Run
	if err := fail(stageEntries); err != nil {
		return InsertCreateRunOutcome{}, err
	}
	tx.state.entries[intent.Run.ID] = append([]execution.WorkflowEntry(nil), intent.Entries...)
	if err := fail(stageSnapshot); err != nil {
		return InsertCreateRunOutcome{}, err
	}
	tx.state.inputs[intent.Run.ID], tx.state.digests[intent.Run.ID] = intent.Snapshot.Input(), intent.Snapshot.Digest()
	if err := fail(stageQueue); err != nil {
		return InsertCreateRunOutcome{}, err
	}
	if _, exists := tx.state.positions[intent.Run.ID]; !exists {
		tx.state.positions[intent.Run.ID] = len(tx.state.queue) + 1
		tx.state.queue = append(tx.state.queue, intent.Run.ID)
	}
	if err := fail(stageCommand); err != nil {
		return InsertCreateRunOutcome{}, err
	}
	entryIDs := make([]string, len(intent.Entries))
	for index := range intent.Entries {
		entryIDs[index] = intent.Entries[index].ExecutionID
	}
	stored := StoredCreateRunResult{Run: intent.Run, Snapshot: intent.Snapshot, SnapshotDigest: intent.Snapshot.Digest(), EntryIDs: entryIDs}
	tx.state.commands[intent.CommandID] = StoredCreateRunCommand{CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: stored}
	tx.dirty = true
	return InsertCreateRunOutcome{Status: InsertCreateRunApplied, CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: stored}, nil
}
func (s *conformanceStore) isEmpty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.state.commands)+len(s.state.runs)+len(s.state.entries)+len(s.state.inputs)+len(s.state.queue) == 0
}

func TestCopyOnWriteStoreRollsBackEveryAtomicWriteStage(t *testing.T) {
	for _, stage := range []conformanceStage{stageRun, stageEntries, stageSnapshot, stageQueue, stageCommand} {
		t.Run(string(stage), func(t *testing.T) {
			command := validCreateRunCommand()
			store := newConformanceStore(validResolvedCreateRun(t, command))
			store.fault = stage
			if _, err := mustCreateRunService(t, store).CreateRun(context.Background(), command); err == nil {
				t.Fatal("fault accepted")
			}
			if !store.isEmpty() {
				t.Fatalf("stage %s published partial state: %#v", stage, store.state)
			}
		})
	}
}

func TestCopyOnWriteStoreRollsBackCallbackCommitCancelAndPanic(t *testing.T) {
	store := newConformanceStore(ResolvedCreateRun{})
	if err := store.InTransaction(context.Background(), func(tx CreateRunTx) error {
		tx.(*conformanceTx).state.queue = append(tx.(*conformanceTx).state.queue, "run")
		return errors.New("callback")
	}); err == nil || !store.isEmpty() {
		t.Fatal("callback error published")
	}
	store.commitErr = errors.New("commit")
	if err := store.InTransaction(context.Background(), func(tx CreateRunTx) error {
		tx.(*conformanceTx).state.queue = append(tx.(*conformanceTx).state.queue, "run")
		return nil
	}); err == nil || !store.isEmpty() {
		t.Fatal("commit error published")
	}
	store.commitErr = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.InTransaction(ctx, func(CreateRunTx) error { t.Fatal("canceled callback executed"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("panic was swallowed")
		}
		if !store.isEmpty() {
			t.Fatal("panic published")
		}
	}()
	_ = store.InTransaction(context.Background(), func(tx CreateRunTx) error {
		tx.(*conformanceTx).state.queue = append(tx.(*conformanceTx).state.queue, "run")
		panic("boom")
	})
}

func TestCopyOnWriteStoreRetriesWithFreshAttemptAndReturnsSuccessfulResult(t *testing.T) {
	command := validCreateRunCommand()
	store := newConformanceStore(validResolvedCreateRun(t, command))
	store.retryOnce = true
	result, err := mustCreateRunService(t, store).CreateRun(context.Background(), command)
	if err != nil || !result.WasApplied || store.attempts != 2 || len(store.state.queue) != 1 {
		t.Fatalf("result=%#v attempts=%d state=%#v err=%v", result, store.attempts, store.state, err)
	}
}

func TestCopyOnWriteStoreReconcilesUnknownCommittedOutcome(t *testing.T) {
	command := validCreateRunCommand()
	store := newConformanceStore(validResolvedCreateRun(t, command))
	store.unknownCommitOnce = true
	result, err := mustCreateRunService(t, store).CreateRun(context.Background(), command)
	if err != nil || result.WasApplied || store.attempts != 2 || len(store.state.queue) != 1 || len(store.state.commands) != 1 {
		t.Fatalf("result=%#v attempts=%d state=%#v err=%v", result, store.attempts, store.state, err)
	}
}

func TestCopyOnWriteStoreConcurrentEqualCommandHasOneWinner(t *testing.T) {
	command := validCreateRunCommand()
	store := newConformanceStore(validResolvedCreateRun(t, command))
	service := mustCreateRunService(t, store)
	results := make(chan CreateRunResult, 2)
	errorsChannel := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := service.CreateRun(context.Background(), command)
			results <- result
			errorsChannel <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errorsChannel)
	applied := 0
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	for result := range results {
		if result.WasApplied {
			applied++
		}
	}
	if applied != 1 || len(store.state.queue) != 1 || len(store.state.positions) != 1 || len(store.state.commands) != 1 {
		t.Fatalf("applied=%d state=%#v", applied, store.state)
	}
}

func TestCopyOnWriteStoreConcurrentConflictsAreTyped(t *testing.T) {
	base := validCreateRunCommand()
	store := newConformanceStore(validResolvedCreateRun(t, base))
	service := mustCreateRunService(t, store)
	if _, err := service.CreateRun(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	changedCommand := base
	changedCommand.EnvironmentID = "different"
	if _, err := service.CreateRun(context.Background(), changedCommand); !fault.IsCode(err, CodeCreateInstanceCommandConflict) {
		t.Fatalf("command conflict=%v", err)
	}
	sameRun := base
	sameRun.CommandID = "command-2"
	sameRun.ScreenshotPolicy.Destination = "other"
	result, err := service.CreateRun(context.Background(), sameRun)
	if !fault.IsCode(err, CodeCreateInstanceSnapshotConflict) ||
		strings.Contains(err.Error(), sameRun.RunID) ||
		!isZeroCreateRunResult(result) {
		t.Fatalf("snapshot conflict result/error=%#v/%v", result, err)
	}
	if len(store.state.queue) != 1 || len(store.state.positions) != 1 {
		t.Fatalf("duplicate queue state=%#v", store.state)
	}
}

func TestCopyOnWriteStoreConflictErrorsAreTyped(t *testing.T) {
	commandErr := createRunCommandConflictError()
	if !fault.IsCode(commandErr, CodeCreateInstanceCommandConflict) {
		t.Fatalf("command conflict classification = %v", commandErr)
	}
	snapshotErr := createRunSnapshotConflictError()
	if !fault.IsCode(snapshotErr, CodeCreateInstanceSnapshotConflict) {
		t.Fatalf("snapshot conflict classification = %v", snapshotErr)
	}
}

var _ CreateRunStore = (*conformanceStore)(nil)
var _ CreateRunTx = (*conformanceTx)(nil)
