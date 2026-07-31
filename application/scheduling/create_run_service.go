package scheduling

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math"
	"reflect"
	"sort"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

const createRunRequestDigestV1 = "create-run-request-v1"

func CreateRunRequestDigest(owned CreateRunCommand) (string, error) {
	owned = normalizeCreateRunCommand(owned)
	if err := preflightCreateRunCommand(owned); err != nil {
		return "", err
	}
	if err := validateCreateRunCommand(owned); err != nil {
		return "", err
	}
	h := sha256.New()
	writeDigestString(h, createRunRequestDigestV1)
	for _, value := range []string{owned.RunID, owned.ExecutionFlowID, owned.TestTaskVersionID, owned.EnvironmentID, string(owned.FailurePolicy), owned.ScreenshotPolicy.Destination} {
		writeDigestString(h, value)
	}
	writeDigestUint64(h, uint64(owned.CreatedAt))
	writeDigestUint64(h, uint64(owned.ScreenshotPolicy.Version))
	writeDigestBool(h, owned.ScreenshotPolicy.Enabled)
	encodeHealerDigest(h, owned.HealerPolicy)
	itemIDs := make([]string, 0, len(owned.Entries))
	for itemID := range owned.Entries {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Strings(itemIDs)
	writeDigestUint64(h, uint64(len(itemIDs)))
	for _, itemID := range itemIDs {
		writeDigestString(h, itemID)
		values := owned.Entries[itemID]
		names := make([]string, 0, len(values))
		for name := range values {
			names = append(names, name)
		}
		sort.Strings(names)
		writeDigestUint64(h, uint64(len(names)))
		for _, name := range names {
			writeDigestString(h, name)
			encodeParameterDigest(h, values[name])
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func encodeParameterDigest(h hash.Hash, value parameter.Value) {
	writeDigestString(h, string(value.Type()))
	switch value.Type() {
	case parameter.Text:
		writeDigestString(h, value.Text())
	case parameter.Number:
		writeDigestString(h, value.Number())
	case parameter.Boolean:
		writeDigestBool(h, value.Boolean())
	case parameter.SingleSelect:
		writeDigestString(h, value.SingleSelect())
	case parameter.MultiSelect:
		items := value.MultiSelect()
		writeDigestUint64(h, uint64(len(items)))
		for _, item := range items {
			writeDigestString(h, item)
		}
	}
}

func encodeHealerDigest(h hash.Hash, policy execution.HealerPolicySnapshot) {
	writeDigestUint64(h, uint64(policy.Version))
	for _, value := range []float64{policy.ReviewCap, policy.AppliedCap, policy.Weights.Tag, policy.Weights.ID, policy.Weights.RoleName, policy.Weights.Class, policy.Weights.Attrs, policy.Weights.Text, policy.Weights.Index, policy.Weights.Neighbor, policy.Weights.LabelText, policy.Weights.Container} {
		writeDigestUint64(h, math.Float64bits(value))
	}
}

func writeDigestString(h hash.Hash, value string) {
	writeDigestUint64(h, uint64(len(value)))
	_, _ = h.Write([]byte(value))
}
func writeDigestUint64(h hash.Hash, value uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], value)
	_, _ = h.Write(b[:])
}
func writeDigestBool(h hash.Hash, value bool) {
	if value {
		_, _ = h.Write([]byte{1})
	} else {
		_, _ = h.Write([]byte{0})
	}
}

type CreateRunService struct{ store CreateRunStore }

func NewCreateRunService(store CreateRunStore) (CreateRunService, error) {
	if isNilCreateRunStore(store) {
		return CreateRunService{}, schedulingDependencyRequiredError()
	}
	return CreateRunService{store: store}, nil
}

func isNilCreateRunStore(store CreateRunStore) bool {
	if store == nil {
		return true
	}
	reflected := reflect.ValueOf(store)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (s CreateRunService) CreateRun(ctx context.Context, command CreateRunCommand) (CreateRunResult, error) {
	if err := preflightCreateRunCommand(command); err != nil {
		return CreateRunResult{}, err
	}
	owned := normalizeCreateRunCommand(cloneCreateRunCommand(command))
	digest, err := CreateRunRequestDigest(owned)
	if err != nil {
		return CreateRunResult{}, err
	}
	var result CreateRunResult
	err = s.store.InTransaction(ctx, func(tx CreateRunTx) error {
		attempt := CreateRunResult{}
		existing, found, err := tx.FindCommand(ctx, owned.CommandID)
		if err != nil {
			return fmt.Errorf("find create-run owned: %w", err)
		}
		if found {
			if existing.RequestDigest != digest {
				return createRunCommandConflictError()
			}
			if existing.CommandID != owned.CommandID {
				return createRunAdapterContractViolationError(errors.New("stored command identity mismatch"))
			}
			if err := validateStoredCreateRunResult(existing.Result, owned); err != nil {
				return err
			}
			attempt = resultFromStored(existing.Result, false)
			result = attempt
			return nil
		}
		resolved, err := tx.ResolveCreateRun(ctx, cloneCreateRunCommand(owned))
		if err != nil {
			return err
		}
		snapshot, err := BuildRunSnapshot(owned, resolved)
		if err != nil {
			return err
		}
		run, err := execution.NewRun(execution.Run{ID: owned.RunID, ExecutionFlowID: owned.ExecutionFlowID, TestTaskVersionID: owned.TestTaskVersionID, Status: execution.Queued, EnvironmentID: owned.EnvironmentID, CreatedAt: owned.CreatedAt, QueuedAt: owned.CreatedAt}, snapshot)
		if err != nil {
			return fmt.Errorf("create queued run: %w", err)
		}
		entries := snapshot.Plan().Entries
		entryIDs := make([]string, len(entries))
		for i := range entries {
			entryIDs[i] = entries[i].ExecutionID
		}
		outcome, err := tx.InsertCreateRun(ctx, CreateRunIntent{CommandID: owned.CommandID, RequestDigest: digest, Run: run, Snapshot: snapshot, Entries: entries})
		if err != nil {
			return fmt.Errorf("insert create run: %w", err)
		}
		if err := validateInsertCreateRunOutcome(outcome, owned, digest, run, snapshot, entryIDs); err != nil {
			return err
		}
		attempt = resultFromStored(outcome.Result, outcome.Status == InsertCreateRunApplied)
		result = attempt
		return nil
	})
	if err != nil {
		// This is the service's only exit, so wrapping here erased the
		// classification of everything the transaction produced — command
		// validation, catalog resolution, snapshot conflicts, adapter contract
		// violations — and left the host a single unclassified error for all of them.
		return CreateRunResult{}, err
	}
	return result, nil
}

func validateStoredCreateRunResult(stored StoredCreateRunResult, command CreateRunCommand) error {
	invalid := func(reason string) error {
		return createRunAdapterContractViolationError(errors.New(reason))
	}
	input := stored.Snapshot.Input()
	if stored.SnapshotDigest == "" || stored.SnapshotDigest != stored.Snapshot.Digest() || stored.Run.SnapshotDigest != stored.SnapshotDigest {
		return invalid("stored snapshot digest identity is inconsistent")
	}
	if stored.Run.ID != command.RunID || stored.Run.ExecutionFlowID != command.ExecutionFlowID || stored.Run.TestTaskVersionID != command.TestTaskVersionID || stored.Run.EnvironmentID != command.EnvironmentID || stored.Run.CreatedAt != command.CreatedAt {
		return invalid("stored run identity does not match command")
	}
	if stored.Snapshot.RunID() != command.RunID || stored.Snapshot.ExecutionFlowID() != command.ExecutionFlowID || stored.Snapshot.TestTaskVersionID() != command.TestTaskVersionID || stored.Snapshot.Environment().ID != command.EnvironmentID {
		return invalid("stored snapshot identity does not match command")
	}
	if input.FailurePolicy != command.FailurePolicy || input.ScreenshotPolicy != command.ScreenshotPolicy || input.HealerPolicy != command.HealerPolicy {
		return invalid("stored snapshot policies do not match command")
	}
	if _, err := execution.HydrateRun(stored.Run, stored.Snapshot); err != nil {
		return invalid("stored run cannot restore snapshot seal: " + err.Error())
	}
	entries := stored.Snapshot.Plan().Entries
	if len(stored.EntryIDs) != len(entries) {
		return invalid("stored entry count mismatch")
	}
	entryByItem := make(map[string]execution.WorkflowEntry, len(entries))
	for index := range entries {
		if stored.EntryIDs[index] != entries[index].ExecutionID {
			return invalid("stored entry identity mismatch")
		}
		entryByItem[entries[index].TestTaskItemID] = entries[index]
	}
	for itemID, requested := range command.Entries {
		entry, exists := entryByItem[itemID]
		if !exists {
			return invalid("command entry is missing from stored plan")
		}
		invocation, exists := stored.Snapshot.Invocation(entry.ExecutionID)
		if !exists {
			return invalid("stored root invocation is missing")
		}
		for name, value := range requested {
			resolved, exists := invocation.Values[name]
			if !exists || !equalParameterValues(map[string]parameter.Value{name: value}, map[string]parameter.Value{name: resolved}) {
				return invalid("stored root parameters do not bind command values")
			}
		}
	}
	return nil
}

func equalAppliedRun(returned, intended execution.Run, snapshot execution.RunSnapshot) bool {
	// QueuePosition is assigned atomically by the adapter; every other persisted field is intent-owned.
	hydrated, err := execution.HydrateRun(returned, snapshot)
	if err != nil {
		return false
	}
	hydrated.QueuePosition = intended.QueuePosition
	return hydrated == intended
}

func cloneCreateRunCommand(command CreateRunCommand) CreateRunCommand {
	owned := command
	owned.Entries = make(map[string]map[string]parameter.Value, len(command.Entries))
	for itemID, values := range command.Entries {
		owned.Entries[itemID] = cloneParameterValues(values)
	}
	return owned
}

type createRunRequestBudget struct {
	remainingBytes      int
	remainingParameters int
	remainingElements   int
}

func newCreateRunRequestBudget() createRunRequestBudget {
	return createRunRequestBudget{remainingBytes: execution.MaxAggregateStringBytes, remainingParameters: execution.MaxAggregateParameters, remainingElements: execution.MaxAggregateCollectionElements}
}

func (b *createRunRequestBudget) addString(value string) error {
	if len(value) > execution.MaxStringBytes || len(value) > b.remainingBytes {
		return createRunCommandInvalidError(nil)
	}
	b.remainingBytes -= len(value)
	return nil
}

func (b *createRunRequestBudget) addStringMetrics(totalBytes, maxItemBytes int) error {
	if totalBytes < 0 || maxItemBytes < 0 || maxItemBytes > execution.MaxStringBytes || totalBytes > b.remainingBytes {
		return createRunCommandInvalidError(nil)
	}
	b.remainingBytes -= totalBytes
	return nil
}

func (b *createRunRequestBudget) addParameters(count int) error {
	if count < 0 || count > b.remainingParameters || count > b.remainingElements {
		return createRunCommandInvalidError(nil)
	}
	b.remainingParameters -= count
	b.remainingElements -= count
	return nil
}

func (b *createRunRequestBudget) addElements(count int) error {
	if count < 0 || count > b.remainingElements {
		return createRunCommandInvalidError(nil)
	}
	b.remainingElements -= count
	return nil
}

func preflightCreateRunCommand(command CreateRunCommand) error {
	budget := newCreateRunRequestBudget()
	for _, value := range []string{command.CommandID, command.RunID, command.ExecutionFlowID, command.TestTaskVersionID, command.EnvironmentID, command.ScreenshotPolicy.Destination} {
		if err := budget.addString(value); err != nil {
			return err
		}
	}
	if err := budget.addElements(len(command.Entries)); err != nil {
		return createRunCommandInvalidError(nil)
	}
	for itemID, values := range command.Entries {
		if err := budget.addString(itemID); err != nil {
			return err
		}
		if err := budget.addParameters(len(values)); err != nil {
			return err
		}
		for name, value := range values {
			if err := budget.addString(name); err != nil {
				return err
			}
			switch value.Type() {
			case parameter.Text:
				if err := budget.addString(value.Text()); err != nil {
					return err
				}
			case parameter.Number:
				if err := budget.addString(value.Number()); err != nil {
					return err
				}
			case parameter.SingleSelect:
				if err := budget.addString(value.SingleSelect()); err != nil {
					return err
				}
			case parameter.MultiSelect:
				count, totalBytes, maxItemBytes, ok := value.MultiSelectMetrics()
				if !ok {
					return createRunCommandInvalidError(nil)
				}
				if err := budget.addElements(count); err != nil {
					return err
				}
				if err := budget.addStringMetrics(totalBytes, maxItemBytes); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateInsertCreateRunOutcome(outcome InsertCreateRunOutcome, command CreateRunCommand, digest string, intendedRun execution.Run, snapshot execution.RunSnapshot, entryIDs []string) error {
	invalid := func(reason string) error {
		return createRunAdapterContractViolationError(errors.New(reason))
	}
	if outcome.Status != InsertCreateRunApplied && outcome.Status != InsertCreateRunReplayed {
		return invalid("invalid status")
	}
	if outcome.CommandID != command.CommandID || outcome.RequestDigest != digest {
		return invalid("command identity or request digest mismatch")
	}
	if outcome.Result.Run.ID != command.RunID || outcome.Result.Snapshot.Digest() == "" {
		return invalid("run or snapshot is incomplete")
	}
	storedPlan := outcome.Result.Snapshot.Plan()
	if outcome.Result.Snapshot.RunID() != outcome.Result.Run.ID || outcome.Result.Snapshot.TestTaskVersionID() != outcome.Result.Run.TestTaskVersionID || len(outcome.Result.EntryIDs) != len(storedPlan.Entries) {
		return invalid("stored result is internally inconsistent")
	}
	for index := range storedPlan.Entries {
		if outcome.Result.EntryIDs[index] != storedPlan.Entries[index].ExecutionID {
			return invalid("stored entry identities are inconsistent")
		}
	}
	if outcome.Status == InsertCreateRunReplayed {
		if err := validateStoredCreateRunResult(outcome.Result, command); err != nil {
			return createRunAdapterContractViolationError(err)
		}
		return nil
	}
	if outcome.Result.SnapshotDigest != snapshot.Digest() {
		return invalid("applied snapshot digest does not match submitted intent")
	}
	if outcome.Status == InsertCreateRunApplied {
		if !equalAppliedRun(outcome.Result.Run, intendedRun, outcome.Result.Snapshot) {
			return invalid("applied run does not match submitted intent")
		}
		if _, err := execution.HydrateRun(outcome.Result.Run, outcome.Result.Snapshot); err != nil {
			return invalid("applied run cannot restore snapshot seal")
		}
		if outcome.Result.Snapshot.Digest() != snapshot.Digest() || len(outcome.Result.EntryIDs) != len(entryIDs) {
			return invalid("applied result does not match submitted intent")
		}
		for index := range entryIDs {
			if outcome.Result.EntryIDs[index] != entryIDs[index] {
				return invalid("applied entry identities do not match submitted intent")
			}
		}
	}
	return nil
}

func resultFromStored(stored StoredCreateRunResult, applied bool) CreateRunResult {
	return CreateRunResult{Run: stored.Run, Snapshot: stored.Snapshot, EntryIDs: append([]string(nil), stored.EntryIDs...), WasApplied: applied}
}
