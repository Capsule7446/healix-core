package scheduling

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"math"
	"reflect"
	"sort"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

const createInstanceRequestDigestV1 = "create-run-request-v1"

func CreateInstanceRequestDigest(owned CreateInstanceCommand) (string, error) {
	owned = normalizeCreateInstanceCommand(owned)
	if err := preflightCreateInstanceCommand(owned); err != nil {
		return "", err
	}
	if err := validateCreateInstanceCommand(owned); err != nil {
		return "", err
	}
	h := sha256.New()
	writeDigestString(h, createInstanceRequestDigestV1)
	for _, value := range []string{owned.InstanceID.String(), owned.ExecutionFlowID, owned.TestTaskVersionID, owned.EnvironmentID, string(owned.FailurePolicy), owned.ScreenshotPolicy.Destination} {
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

type CreateInstanceService struct{ store CreateInstanceStore }

func NewCreateInstanceService(store CreateInstanceStore) (CreateInstanceService, error) {
	if isNilCreateInstanceStore(store) {
		return CreateInstanceService{}, schedulingDependencyRequiredError()
	}
	return CreateInstanceService{store: store}, nil
}

func isNilCreateInstanceStore(store CreateInstanceStore) bool {
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

func (s CreateInstanceService) CreateInstance(ctx context.Context, command CreateInstanceCommand) (CreateInstanceResult, error) {
	if err := preflightCreateInstanceCommand(command); err != nil {
		return CreateInstanceResult{}, err
	}
	owned := normalizeCreateInstanceCommand(cloneCreateInstanceCommand(command))
	digest, err := CreateInstanceRequestDigest(owned)
	if err != nil {
		return CreateInstanceResult{}, err
	}
	var result CreateInstanceResult
	err = s.store.InTransaction(ctx, func(tx CreateInstanceTx) error {
		attempt := CreateInstanceResult{}
		existing, found, err := tx.FindCommand(ctx, owned.CommandID)
		if err != nil {
			return classifySchedulingAdapterFailure(err)
		}
		if found {
			if existing.RequestDigest != digest {
				return createInstanceCommandConflictError()
			}
			if existing.CommandID != owned.CommandID {
				return createInstanceAdapterContractViolationError(errors.New("stored command identity mismatch"))
			}
			if err := validateStoredCreateInstanceResult(existing.Result, owned); err != nil {
				return err
			}
			attempt = resultFromStored(existing.Result, false)
			result = attempt
			return nil
		}
		resolved, err := tx.ResolveCreateInstance(ctx, cloneCreateInstanceCommand(owned))
		if err != nil {
			return err
		}
		snapshot, err := BuildInstanceSnapshot(owned, resolved)
		if err != nil {
			return err
		}
		run, err := execution.NewInstance(execution.Instance{ID: owned.InstanceID, ExecutionFlowID: owned.ExecutionFlowID, TestTaskVersionID: owned.TestTaskVersionID, Status: execution.Queued, EnvironmentID: owned.EnvironmentID, CreatedAt: owned.CreatedAt, QueuedAt: owned.CreatedAt}, snapshot)
		if err != nil {
			return err // execution.NewInstance's own error is being classified by a parallel domain/execution migration; this boundary neither adds an uncoded layer on top nor buries a code that is already there.
		}
		entries := snapshot.Plan().Entries
		entryIDs := make([]execution.EntryID, len(entries))
		for i := range entries {
			entryIDs[i] = entries[i].ID
		}
		outcome, err := tx.InsertCreateInstance(ctx, CreateInstanceIntent{CommandID: owned.CommandID, RequestDigest: digest, Run: run, Snapshot: snapshot, Entries: entries})
		if err != nil {
			return classifySchedulingAdapterFailure(err)
		}
		if err := validateInsertCreateInstanceOutcome(outcome, owned, digest, run, snapshot, entryIDs); err != nil {
			return err
		}
		attempt = resultFromStored(outcome.Result, outcome.Status == InsertCreateInstanceApplied)
		result = attempt
		return nil
	})
	if err != nil {
		// This is the service's only exit, so wrapping here erased the
		// classification of everything the transaction produced — command
		// validation, catalog resolution, snapshot conflicts, adapter contract
		// violations — and left the host a single unclassified error for all of them.
		return CreateInstanceResult{}, err
	}
	return result, nil
}

func validateStoredCreateInstanceResult(stored StoredCreateInstanceResult, command CreateInstanceCommand) error {
	invalid := func(reason string) error {
		return createInstanceAdapterContractViolationError(errors.New(reason))
	}
	input := stored.Snapshot.Input()
	if stored.SnapshotDigest == "" || stored.SnapshotDigest != stored.Snapshot.Digest() || stored.Run.SnapshotDigest != stored.SnapshotDigest {
		return invalid("stored snapshot digest identity is inconsistent")
	}
	if stored.Run.ID != command.InstanceID || stored.Run.ExecutionFlowID != command.ExecutionFlowID || stored.Run.TestTaskVersionID != command.TestTaskVersionID || stored.Run.EnvironmentID != command.EnvironmentID || stored.Run.CreatedAt != command.CreatedAt {
		return invalid("stored run identity does not match command")
	}
	if stored.Snapshot.InstanceID() != command.InstanceID || stored.Snapshot.ExecutionFlowID() != command.ExecutionFlowID || stored.Snapshot.TestTaskVersionID() != command.TestTaskVersionID || stored.Snapshot.Environment().ID != command.EnvironmentID {
		return invalid("stored snapshot identity does not match command")
	}
	if input.FailurePolicy != command.FailurePolicy || input.ScreenshotPolicy != command.ScreenshotPolicy || input.HealerPolicy != command.HealerPolicy {
		return invalid("stored snapshot policies do not match command")
	}
	if _, err := execution.HydrateInstance(stored.Run, stored.Snapshot); err != nil {
		return invalid("stored run cannot restore snapshot seal: " + err.Error())
	}
	entries := stored.Snapshot.Plan().Entries
	if len(stored.EntryIDs) != len(entries) {
		return invalid("stored entry count mismatch")
	}
	entryByItem := make(map[string]execution.Entry, len(entries))
	for index := range entries {
		if stored.EntryIDs[index] != entries[index].ID {
			return invalid("stored entry identity mismatch")
		}
		entryByItem[entries[index].TestTaskItemID] = entries[index]
	}
	for itemID, requested := range command.Entries {
		entry, exists := entryByItem[itemID]
		if !exists {
			return invalid("command entry is missing from stored plan")
		}
		invocation, exists := stored.Snapshot.Invocation(execution.RootInvocationPath(entry.ID))
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

func equalAppliedInstance(returned, intended execution.Instance, snapshot execution.InstanceSnapshot) bool {
	// QueuePosition is assigned atomically by the adapter; every other persisted field is intent-owned.
	hydrated, err := execution.HydrateInstance(returned, snapshot)
	if err != nil {
		return false
	}
	hydrated.QueuePosition = intended.QueuePosition
	return hydrated == intended
}

func cloneCreateInstanceCommand(command CreateInstanceCommand) CreateInstanceCommand {
	owned := command
	owned.Entries = make(map[string]map[string]parameter.Value, len(command.Entries))
	for itemID, values := range command.Entries {
		owned.Entries[itemID] = cloneParameterValues(values)
	}
	return owned
}

type createInstanceRequestBudget struct {
	remainingBytes      int
	remainingParameters int
	remainingElements   int
}

func newCreateInstanceRequestBudget() createInstanceRequestBudget {
	return createInstanceRequestBudget{remainingBytes: execution.MaxAggregateStringBytes, remainingParameters: execution.MaxAggregateParameters, remainingElements: execution.MaxAggregateCollectionElements}
}

func (b *createInstanceRequestBudget) addString(value string) error {
	if len(value) > execution.MaxStringBytes || len(value) > b.remainingBytes {
		return createInstanceCommandInvalidError(nil)
	}
	b.remainingBytes -= len(value)
	return nil
}

func (b *createInstanceRequestBudget) addStringMetrics(totalBytes, maxItemBytes int) error {
	if totalBytes < 0 || maxItemBytes < 0 || maxItemBytes > execution.MaxStringBytes || totalBytes > b.remainingBytes {
		return createInstanceCommandInvalidError(nil)
	}
	b.remainingBytes -= totalBytes
	return nil
}

func (b *createInstanceRequestBudget) addParameters(count int) error {
	if count < 0 || count > b.remainingParameters || count > b.remainingElements {
		return createInstanceCommandInvalidError(nil)
	}
	b.remainingParameters -= count
	b.remainingElements -= count
	return nil
}

func (b *createInstanceRequestBudget) addElements(count int) error {
	if count < 0 || count > b.remainingElements {
		return createInstanceCommandInvalidError(nil)
	}
	b.remainingElements -= count
	return nil
}

func preflightCreateInstanceCommand(command CreateInstanceCommand) error {
	budget := newCreateInstanceRequestBudget()
	for _, value := range []string{command.CommandID, command.InstanceID.String(), command.ExecutionFlowID, command.TestTaskVersionID, command.EnvironmentID, command.ScreenshotPolicy.Destination} {
		if err := budget.addString(value); err != nil {
			return err
		}
	}
	if err := budget.addElements(len(command.Entries)); err != nil {
		return createInstanceCommandInvalidError(nil)
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
					return createInstanceCommandInvalidError(nil)
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

func validateInsertCreateInstanceOutcome(outcome InsertCreateInstanceOutcome, command CreateInstanceCommand, digest string, intendedInstance execution.Instance, snapshot execution.InstanceSnapshot, entryIDs []execution.EntryID) error {
	invalid := func(reason string) error {
		return createInstanceAdapterContractViolationError(errors.New(reason))
	}
	if outcome.Status != InsertCreateInstanceApplied && outcome.Status != InsertCreateInstanceReplayed {
		return invalid("invalid status")
	}
	if outcome.CommandID != command.CommandID || outcome.RequestDigest != digest {
		return invalid("command identity or request digest mismatch")
	}
	if outcome.Result.Run.ID != command.InstanceID || outcome.Result.Snapshot.Digest() == "" {
		return invalid("run or snapshot is incomplete")
	}
	storedPlan := outcome.Result.Snapshot.Plan()
	if outcome.Result.Snapshot.InstanceID() != outcome.Result.Run.ID || outcome.Result.Snapshot.TestTaskVersionID() != outcome.Result.Run.TestTaskVersionID || len(outcome.Result.EntryIDs) != len(storedPlan.Entries) {
		return invalid("stored result is internally inconsistent")
	}
	for index := range storedPlan.Entries {
		if outcome.Result.EntryIDs[index] != storedPlan.Entries[index].ID {
			return invalid("stored entry identities are inconsistent")
		}
	}
	if outcome.Status == InsertCreateInstanceReplayed {
		if err := validateStoredCreateInstanceResult(outcome.Result, command); err != nil {
			return createInstanceAdapterContractViolationError(err)
		}
		return nil
	}
	if outcome.Result.SnapshotDigest != snapshot.Digest() {
		return invalid("applied snapshot digest does not match submitted intent")
	}
	if outcome.Status == InsertCreateInstanceApplied {
		if !equalAppliedInstance(outcome.Result.Run, intendedInstance, outcome.Result.Snapshot) {
			return invalid("applied run does not match submitted intent")
		}
		if _, err := execution.HydrateInstance(outcome.Result.Run, outcome.Result.Snapshot); err != nil {
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

func resultFromStored(stored StoredCreateInstanceResult, applied bool) CreateInstanceResult {
	return CreateInstanceResult{Run: stored.Run, Snapshot: stored.Snapshot, EntryIDs: append([]execution.EntryID(nil), stored.EntryIDs...), WasApplied: applied}
}
