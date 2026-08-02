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
	stageInstance conformanceStage = "run"
	stageEntries  conformanceStage = "entries"
	stageSnapshot conformanceStage = "snapshot"
	stageQueue    conformanceStage = "queue"
	stageCommand  conformanceStage = "command"
)

type conformanceState struct {
	commands  map[string]StoredCreateInstanceCommand
	runs      map[string]execution.Instance
	entries   map[string][]execution.Entry
	inputs    map[string]execution.InstanceSnapshotInput
	digests   map[string]string
	queue     []string
	positions map[string]int
}

func emptyConformanceState() conformanceState {
	return conformanceState{commands: map[string]StoredCreateInstanceCommand{}, runs: map[string]execution.Instance{}, entries: map[string][]execution.Entry{}, inputs: map[string]execution.InstanceSnapshotInput{}, digests: map[string]string{}, positions: map[string]int{}}
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
		out.entries[key] = append([]execution.Entry(nil), value...)
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
	resolved          ResolvedCreateInstance
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

func newConformanceStore(resolved ResolvedCreateInstance) *conformanceStore {
	return &conformanceStore{state: emptyConformanceState(), resolved: resolved}
}
func (s *conformanceStore) InTransaction(ctx context.Context, callback func(CreateInstanceTx) error) (err error) {
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
					return createInstanceCommandConflictError()
				}
				s.mu.Unlock()
				continue transactionAttempt
			}
			if winner, exists := s.state.runs[command.Result.Run.ID.String()]; exists && winner.SnapshotDigest != command.Result.Run.SnapshotDigest {
				s.mu.Unlock()
				return createInstanceSnapshotConflictError()
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
func (tx *conformanceTx) FindCommand(_ context.Context, id string) (StoredCreateInstanceCommand, bool, error) {
	value, ok := tx.state.commands[id]
	return value, ok, nil
}
func (tx *conformanceTx) ResolveCreateInstance(context.Context, CreateInstanceCommand) (ResolvedCreateInstance, error) {
	return tx.store.resolved, nil
}
func (tx *conformanceTx) InsertCreateInstance(_ context.Context, intent CreateInstanceIntent) (InsertCreateInstanceOutcome, error) {
	if existing, ok := tx.state.commands[intent.CommandID]; ok {
		if existing.RequestDigest != intent.RequestDigest {
			return InsertCreateInstanceOutcome{}, createInstanceCommandConflictError()
		}
		return InsertCreateInstanceOutcome{Status: InsertCreateInstanceReplayed, CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: existing.Result}, nil
	}
	if existing, ok := tx.state.runs[intent.Run.ID.String()]; ok && existing.SnapshotDigest != intent.Run.SnapshotDigest {
		return InsertCreateInstanceOutcome{}, createInstanceSnapshotConflictError()
	}
	fail := func(stage conformanceStage) error {
		if tx.store.fault == stage {
			return fmt.Errorf("fault at %s", stage)
		}
		return nil
	}
	if err := fail(stageInstance); err != nil {
		return InsertCreateInstanceOutcome{}, err
	}
	tx.state.runs[intent.Run.ID.String()] = intent.Run
	if err := fail(stageEntries); err != nil {
		return InsertCreateInstanceOutcome{}, err
	}
	tx.state.entries[intent.Run.ID.String()] = append([]execution.Entry(nil), intent.Entries...)
	if err := fail(stageSnapshot); err != nil {
		return InsertCreateInstanceOutcome{}, err
	}
	tx.state.inputs[intent.Run.ID.String()], tx.state.digests[intent.Run.ID.String()] = intent.Snapshot.Input(), intent.Snapshot.Digest()
	if err := fail(stageQueue); err != nil {
		return InsertCreateInstanceOutcome{}, err
	}
	if _, exists := tx.state.positions[intent.Run.ID.String()]; !exists {
		tx.state.positions[intent.Run.ID.String()] = len(tx.state.queue) + 1
		tx.state.queue = append(tx.state.queue, intent.Run.ID.String())
	}
	if err := fail(stageCommand); err != nil {
		return InsertCreateInstanceOutcome{}, err
	}
	entryIDs := make([]execution.EntryID, len(intent.Entries))
	for index := range intent.Entries {
		entryIDs[index] = intent.Entries[index].ID
	}
	stored := StoredCreateInstanceResult{Run: intent.Run, Snapshot: intent.Snapshot, SnapshotDigest: intent.Snapshot.Digest(), EntryIDs: entryIDs}
	tx.state.commands[intent.CommandID] = StoredCreateInstanceCommand{CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: stored}
	tx.dirty = true
	return InsertCreateInstanceOutcome{Status: InsertCreateInstanceApplied, CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: stored}, nil
}
func (s *conformanceStore) isEmpty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.state.commands)+len(s.state.runs)+len(s.state.entries)+len(s.state.inputs)+len(s.state.queue) == 0
}

func TestCopyOnWriteStoreRollsBackEveryAtomicWriteStage(t *testing.T) {
	for _, stage := range []conformanceStage{stageInstance, stageEntries, stageSnapshot, stageQueue, stageCommand} {
		t.Run(string(stage), func(t *testing.T) {
			command := validCreateInstanceCommand()
			store := newConformanceStore(validResolvedCreateInstance(t, command))
			store.fault = stage
			if _, err := mustCreateInstanceService(t, store).CreateInstance(context.Background(), command); err == nil {
				t.Fatal("fault accepted")
			}
			if !store.isEmpty() {
				t.Fatalf("stage %s published partial state: %#v", stage, store.state)
			}
		})
	}
}

func TestCopyOnWriteStoreRollsBackCallbackCommitCancelAndPanic(t *testing.T) {
	store := newConformanceStore(ResolvedCreateInstance{})
	if err := store.InTransaction(context.Background(), func(tx CreateInstanceTx) error {
		tx.(*conformanceTx).state.queue = append(tx.(*conformanceTx).state.queue, "run")
		return errors.New("callback")
	}); err == nil || !store.isEmpty() {
		t.Fatal("callback error published")
	}
	store.commitErr = errors.New("commit")
	if err := store.InTransaction(context.Background(), func(tx CreateInstanceTx) error {
		tx.(*conformanceTx).state.queue = append(tx.(*conformanceTx).state.queue, "run")
		return nil
	}); err == nil || !store.isEmpty() {
		t.Fatal("commit error published")
	}
	store.commitErr = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.InTransaction(ctx, func(CreateInstanceTx) error { t.Fatal("canceled callback executed"); return nil }); !errors.Is(err, context.Canceled) {
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
	_ = store.InTransaction(context.Background(), func(tx CreateInstanceTx) error {
		tx.(*conformanceTx).state.queue = append(tx.(*conformanceTx).state.queue, "run")
		panic("boom")
	})
}

func TestCopyOnWriteStoreRetriesWithFreshAttemptAndReturnsSuccessfulResult(t *testing.T) {
	command := validCreateInstanceCommand()
	store := newConformanceStore(validResolvedCreateInstance(t, command))
	store.retryOnce = true
	result, err := mustCreateInstanceService(t, store).CreateInstance(context.Background(), command)
	if err != nil || !result.WasApplied || store.attempts != 2 || len(store.state.queue) != 1 {
		t.Fatalf("result=%#v attempts=%d state=%#v err=%v", result, store.attempts, store.state, err)
	}
}

func TestCopyOnWriteStoreReconcilesUnknownCommittedOutcome(t *testing.T) {
	command := validCreateInstanceCommand()
	store := newConformanceStore(validResolvedCreateInstance(t, command))
	store.unknownCommitOnce = true
	result, err := mustCreateInstanceService(t, store).CreateInstance(context.Background(), command)
	if err != nil || result.WasApplied || store.attempts != 2 || len(store.state.queue) != 1 || len(store.state.commands) != 1 {
		t.Fatalf("result=%#v attempts=%d state=%#v err=%v", result, store.attempts, store.state, err)
	}
}

func TestCopyOnWriteStoreConcurrentEqualCommandHasOneWinner(t *testing.T) {
	command := validCreateInstanceCommand()
	store := newConformanceStore(validResolvedCreateInstance(t, command))
	service := mustCreateInstanceService(t, store)
	results := make(chan CreateInstanceResult, 2)
	errorsChannel := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := service.CreateInstance(context.Background(), command)
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
	base := validCreateInstanceCommand()
	store := newConformanceStore(validResolvedCreateInstance(t, base))
	service := mustCreateInstanceService(t, store)
	if _, err := service.CreateInstance(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	changedCommand := base
	changedCommand.EnvironmentID = "different"
	if _, err := service.CreateInstance(context.Background(), changedCommand); !fault.IsCode(err, CodeCreateInstanceCommandConflict) {
		t.Fatalf("command conflict=%v", err)
	}
	sameInstance := base
	sameInstance.CommandID = "command-2"
	sameInstance.ScreenshotPolicy.Destination = "other"
	result, err := service.CreateInstance(context.Background(), sameInstance)
	if !fault.IsCode(err, CodeCreateInstanceSnapshotConflict) ||
		strings.Contains(err.Error(), sameInstance.InstanceID.String()) ||
		!isZeroCreateInstanceResult(result) {
		t.Fatalf("snapshot conflict result/error=%#v/%v", result, err)
	}
	if len(store.state.queue) != 1 || len(store.state.positions) != 1 {
		t.Fatalf("duplicate queue state=%#v", store.state)
	}
}

func TestCopyOnWriteStoreConflictErrorsAreTyped(t *testing.T) {
	commandErr := createInstanceCommandConflictError()
	if !fault.IsCode(commandErr, CodeCreateInstanceCommandConflict) {
		t.Fatalf("command conflict classification = %v", commandErr)
	}
	snapshotErr := createInstanceSnapshotConflictError()
	if !fault.IsCode(snapshotErr, CodeCreateInstanceSnapshotConflict) {
		t.Fatalf("snapshot conflict classification = %v", snapshotErr)
	}
}

var _ CreateInstanceStore = (*conformanceStore)(nil)
var _ CreateInstanceTx = (*conformanceTx)(nil)
