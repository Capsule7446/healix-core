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

// createInstanceRequestDigestV1 是摘要域分离标签而非 Go 名称。现有每条幂等记录都使用这些精确字节
// 计算摘要，因此术语重命名不得修改它们。
const createInstanceRequestDigestV1 = "create-run-request-v1"

// CreateInstanceRequestDigest 为规范化创建命令生成稳定的 SHA-256 请求摘要。
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

// encodeParameterDigest 以稳定类型和值编码参数摘要；多选值保留其顺序。
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

// encodeHealerDigest 以版本、阈值和权重浮点位模式编码自愈策略摘要。
func encodeHealerDigest(h hash.Hash, policy execution.HealerPolicySnapshot) {
	writeDigestUint64(h, uint64(policy.Version))
	for _, value := range []float64{policy.ReviewCap, policy.AppliedCap, policy.Weights.Tag, policy.Weights.ID, policy.Weights.RoleName, policy.Weights.Class, policy.Weights.Attrs, policy.Weights.Text, policy.Weights.Index, policy.Weights.Neighbor, policy.Weights.LabelText, policy.Weights.Container, policy.Weights.Framework} {
		writeDigestUint64(h, math.Float64bits(value))
	}
}

// writeDigestString 以长度前缀写入摘要字符串。
func writeDigestString(h hash.Hash, value string) {
	writeDigestUint64(h, uint64(len(value)))
	_, _ = h.Write([]byte(value))
}

// writeDigestUint64 以固定的大端字节序写入摘要整数。
func writeDigestUint64(h hash.Hash, value uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], value)
	_, _ = h.Write(b[:])
}

// writeDigestBool 写入单字节布尔摘要值。
func writeDigestBool(h hash.Hash, value bool) {
	if value {
		_, _ = h.Write([]byte{1})
	} else {
		_, _ = h.Write([]byte{0})
	}
}

// CreateInstanceService 编排创建命令摘要、目录解析、快照封存和原子实例插入。
type CreateInstanceService struct{ store CreateInstanceStore }

// NewCreateInstanceService 校验存储依赖并构造创建实例服务。
func NewCreateInstanceService(store CreateInstanceStore) (CreateInstanceService, error) {
	if isNilCreateInstanceStore(store) {
		return CreateInstanceService{}, schedulingDependencyRequiredError()
	}
	return CreateInstanceService{store: store}, nil
}

// isNilCreateInstanceStore 识别直接为 nil 或承载 typed nil 的创建存储。
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

// CreateInstance 复制并规范化命令，计算摘要，在单一事务中解析目录、封存快照、创建运行并原子插入；
// 相同命令 ID 和摘要的重放返回权威存储结果。
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
			// execution.NewInstance 已返回自身分类的领域错误；此边界不添加未分类外层，也不掩盖已有错误码。
			return err
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
		// 这是服务的唯一出口；此处不包裹，以保留事务产生的命令校验、目录解析、快照冲突和适配器
		// 契约违规分类，避免宿主只得到一个无法区分的未分类错误。
		return CreateInstanceResult{}, err
	}
	return result, nil
}

// validateStoredCreateInstanceResult 校验重放结果的快照摘要、运行身份、策略、入口和参数绑定。
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
	for _, itemID := range sortedKeys(command.Entries) {
		requested := command.Entries[itemID]
		entry, exists := entryByItem[itemID]
		if !exists {
			return invalid("command entry is missing from stored plan")
		}
		invocation, exists := stored.Snapshot.Invocation(execution.RootInvocationPath(entry.ID))
		if !exists {
			return invalid("stored root invocation is missing")
		}
		for _, name := range sortedKeys(requested) {
			resolved, exists := invocation.Values[name]
			if !exists || !equalParameterValues(map[string]parameter.Value{name: requested[name]}, map[string]parameter.Value{name: resolved}) {
				return invalid("stored root parameters do not bind command values")
			}
		}
	}
	return nil
}

// equalAppliedInstance 比较适配器返回运行与提交意图；QueuePosition 由适配器原子分配，其余字段须一致。
func equalAppliedInstance(returned, intended execution.Instance, snapshot execution.InstanceSnapshot) bool {
	hydrated, err := execution.HydrateInstance(returned, snapshot)
	if err != nil {
		return false
	}
	hydrated.QueuePosition = intended.QueuePosition
	return hydrated == intended
}

// cloneCreateInstanceCommand 深拷贝命令中的参数映射，返回服务独立拥有的命令。
func cloneCreateInstanceCommand(command CreateInstanceCommand) CreateInstanceCommand {
	owned := command
	owned.Entries = make(map[string]map[string]parameter.Value, len(command.Entries))
	for itemID, values := range command.Entries {
		owned.Entries[itemID] = cloneParameterValues(values)
	}
	return owned
}

// createInstanceRequestBudget 跟踪创建请求剩余字节、参数和元素预算。
type createInstanceRequestBudget struct {
	remainingBytes      int
	remainingParameters int
	remainingElements   int
}

// newCreateInstanceRequestBudget 初始化创建请求的聚合限制预算。
func newCreateInstanceRequestBudget() createInstanceRequestBudget {
	return createInstanceRequestBudget{remainingBytes: execution.MaxAggregateStringBytes, remainingParameters: execution.MaxAggregateParameters, remainingElements: execution.MaxAggregateCollectionElements}
}

// addString 从预算中扣除字符串字节数，超限时返回命令无效错误。
func (b *createInstanceRequestBudget) addString(value string) error {
	if len(value) > execution.MaxStringBytes || len(value) > b.remainingBytes {
		return createInstanceCommandInvalidError(nil)
	}
	b.remainingBytes -= len(value)
	return nil
}

// addStringMetrics 从预算中扣除多选总字节数及单项最大字节数。
func (b *createInstanceRequestBudget) addStringMetrics(totalBytes, maxItemBytes int) error {
	if totalBytes < 0 || maxItemBytes < 0 || maxItemBytes > execution.MaxStringBytes || totalBytes > b.remainingBytes {
		return createInstanceCommandInvalidError(nil)
	}
	b.remainingBytes -= totalBytes
	return nil
}

// addParameters 从预算中扣除参数及其元素数量。
func (b *createInstanceRequestBudget) addParameters(count int) error {
	if count < 0 || count > b.remainingParameters || count > b.remainingElements {
		return createInstanceCommandInvalidError(nil)
	}
	b.remainingParameters -= count
	b.remainingElements -= count
	return nil
}

// addElements 从预算中扣除聚合元素数量。
func (b *createInstanceRequestBudget) addElements(count int) error {
	if count < 0 || count > b.remainingElements {
		return createInstanceCommandInvalidError(nil)
	}
	b.remainingElements -= count
	return nil
}

// preflightCreateInstanceCommand 在读取目录前校验创建命令的字符串、参数和集合预算。
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

// validateInsertCreateInstanceOutcome 校验插入适配器返回的身份、快照、运行和入口结果。
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

// resultFromStored 将存储结果转换为调用方结果，并复制入口 ID 切片所有权。
func resultFromStored(stored StoredCreateInstanceResult, applied bool) CreateInstanceResult {
	return CreateInstanceResult{Run: stored.Run, Snapshot: stored.Snapshot, EntryIDs: append([]execution.EntryID(nil), stored.EntryIDs...), WasApplied: applied}
}
