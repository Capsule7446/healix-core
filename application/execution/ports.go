package execution

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Capsule7446/healix-core/domain/evidence"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// stepTransitionCommitPayloadTooLargeError 将提交负载超限包装为范围错误。
func stepTransitionCommitPayloadTooLargeError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.OutOfRange, CodeStepTransitionCommitPayloadTooLarge, "step transition commit payload is too large")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// stepTransitionCommitInstanceMismatchError 将提交事实与领取实例不一致包装为前置条件错误。
func stepTransitionCommitInstanceMismatchError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.FailedPrecondition, CodeStepTransitionCommitRunMismatch, "step transition commit does not match the claimed run")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// StepRevisionConflictError 构造步骤转换修订冲突错误。
func StepRevisionConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeStepRevisionConflict,
		"step transition revision conflicts with current state",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// CommitIdentityConflictError 构造步骤转换提交身份冲突错误。
func CommitIdentityConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeCommitIdentityConflict,
		"step transition commit identity conflicts with the previously accepted commit",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// FactCommitterRequiredError 构造缺少事实提交器的前置条件错误。
func FactCommitterRequiredError() error {
	err, constructionErr := fault.New(
		fault.FailedPrecondition,
		CodeFactCommitterRequired,
		"execution fact committer is required",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

const (
	// MaxStepTransitionPayloadBytes 以遍历得到的内容字节数表示。
	//
	// 当度量方式为 len(json.Marshal) 时，该数值相同；随后曾短暂缩放为 1<<18，依据是旧 framing 约为
	// 内容的 4.5 倍，以为四分之一的限制会保留原边界。该推理不成立：比例并非恒定。Framing 按字段
	// 收费而非按字节收费，因此少量长字符串几乎没有 framing，而大量短字段会产生很多 framing。五个
	// 各 60 KiB 的字符串分别通过单字符串上限，总内容为 300 KiB；在旧的 1 MiB JSON 限制下会接受，
	// 在四分之一 MiB 内容限制下会拒绝。
	//
	// 没有单一倍数可以保留依赖形状的边界，因此需要决定错误方向。拒绝宿主昨日成功发送的负载是更差
	// 的失败，而实际膨胀已经由另外两条规则限制：maxStepTransitionStringBytes 将单个字符串限制为
	// 64 KiB，maxStepTransitionFacts 将事实数限制为 10,000。
	MaxStepTransitionPayloadBytes = 1 << 20
	// maxStepTransitionStringBytes 限制单个步骤转换字符串的字节数。
	maxStepTransitionStringBytes = 64 << 10
)

// ValidateStepTransitionPayloadSize 校验步骤转换提交的字符串和总负载大小。
func ValidateStepTransitionPayloadSize(commit evidence.StepTransitionCommit) error {
	_, err := ownStepTransitionCommit(commit)
	return err
}

// ownStepTransitionCommit 校验并克隆步骤转换提交，使返回值由调用方独立拥有。
func ownStepTransitionCommit(commit evidence.StepTransitionCommit) (evidence.StepTransitionCommit, error) {
	if err := validateStepTransitionStringBounds(reflect.ValueOf(commit)); err != nil {
		return evidence.StepTransitionCommit{}, err
	}
	// 大小通过遍历值测量，复制由拥有该类型的 Clone 完成。此前二者曾共用一次 json.Marshal 往返，
	// 既错误度量也错误复制：执行坐标是唯一字段未导出的结构体，会编码为 {}，预算只计两个字节而非
	// 真实长度，解码回来的值也会变成零值。
	if size := stepTransitionPayloadBytes(reflect.ValueOf(commit)); size > MaxStepTransitionPayloadBytes {
		return evidence.StepTransitionCommit{}, stepTransitionCommitPayloadTooLargeError(fmt.Errorf("step transition commit exceeds byte limit %d", MaxStepTransitionPayloadBytes))
	}
	return commit.Clone(), nil
}

// stepTransitionPayloadBytes 测量提交携带的内容：字符串字节数加上每个定长字段的固定宽度。
// 它取代 len(json.Marshal)，后者两次产生错误度量：计算了某种编码的 framing，并把每个执行坐标
// 计为 {} 的两个字节而非真实长度。
//
// 度量单位已变化，任何标量都无法跨越旧边界：旧度量按字段收取 framing，因此与内容的比例取决于提交
// 形状。边界取舍及原因见 MaxStepTransitionPayloadBytes。
//
// 未导出的字符串字段也会计入：reflect 可以读取其长度，即使不能将字段值交给调用方。
func stepTransitionPayloadBytes(value reflect.Value) int {
	return walkStepTransitionBytes(value, 0)
}

// maxStepTransitionWalkDepth 限制遍历深度。提交树当前是有限的，但 json.Marshal 在遇到循环时会返回
// 错误，而替代它的遍历若无上限会递归直到栈耗尽；观测结构新增一个指针字段就可能触发这一点。
const maxStepTransitionWalkDepth = 64

// walkStepTransitionBytes 递归遍历反射值并计算内容字节数，超过深度上限时停止。
func walkStepTransitionBytes(value reflect.Value, depth int) int {
	if !value.IsValid() || depth > maxStepTransitionWalkDepth {
		return 0
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return 1
		}
		return walkStepTransitionBytes(value.Elem(), depth+1)
	case reflect.String:
		return value.Len()
	case reflect.Struct:
		total := 0
		for index := 0; index < value.NumField(); index++ {
			total += walkStepTransitionBytes(value.Field(index), depth+1)
		}
		return total
	case reflect.Slice, reflect.Array:
		total := 0
		for index := 0; index < value.Len(); index++ {
			total += walkStepTransitionBytes(value.Index(index), depth+1)
		}
		return total
	case reflect.Map:
		total := 0
		iterator := value.MapRange()
		for iterator.Next() {
			total += walkStepTransitionBytes(iterator.Key(), depth+1)
			total += walkStepTransitionBytes(iterator.Value(), depth+1)
		}
		return total
	default:
		return 8
	}
}

// validateStepTransitionStringBounds 递归校验所有字符串字段不超过单字符串字节上限。
func validateStepTransitionStringBounds(value reflect.Value) error {
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return validateStepTransitionStringBounds(value.Elem())
	case reflect.String:
		if value.Len() > maxStepTransitionStringBytes {
			return stepTransitionCommitPayloadTooLargeError(fmt.Errorf("step transition string exceeds byte limit %d", maxStepTransitionStringBytes))
		}
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			if err := validateStepTransitionStringBounds(value.Field(index)); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if err := validateStepTransitionStringBounds(value.Index(index)); err != nil {
				return err
			}
		}
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			if err := validateStepTransitionStringBounds(iterator.Key()); err != nil {
				return err
			}
			if err := validateStepTransitionStringBounds(iterator.Value()); err != nil {
				return err
			}
		}
	}
	return nil
}

// StepTransitionTransaction 定义一次步骤转换提交的原子端口，负责最终事实、自愈治理、效果、outbox
// 记录和权威重放结果。
type StepTransitionTransaction interface {
	// CommitStepTransition 在工作线程 fence 下原子提交步骤转换及其自愈治理计划。
	CommitStepTransition(context.Context, domainexecution.WorkerFence, evidence.StepTransitionCommit, HealGovernancePlanner) (evidence.StepTransitionCommitResult, error)
}

// FactCommitter 持有步骤转换事务和自愈治理计划器。
type FactCommitter struct {
	transaction StepTransitionTransaction
	planner     HealGovernancePlanner
}

// NewFactCommitter 构造事实提交器；依赖缺失时由提交操作返回配置错误。
func NewFactCommitter(transaction StepTransitionTransaction, planner HealGovernancePlanner) FactCommitter {
	return FactCommitter{transaction: transaction, planner: planner}
}

// CommitStepTransition 校验依赖并将步骤转换提交委托给事务端口。
func (c FactCommitter) CommitStepTransition(ctx context.Context, fence domainexecution.WorkerFence, commit evidence.StepTransitionCommit) (evidence.StepTransitionCommitResult, error) {
	if isNilInterface(c.transaction) {
		return evidence.StepTransitionCommitResult{}, FactCommitterRequiredError()
	}
	if isNilInterface(c.planner) {
		// 与上方分支相同的依赖缺失错误已有注册码；保持裸错误会使两个相同条件中有一个无法分类。
		return evidence.StepTransitionCommitResult{}, FactCommitterRequiredError()
	}
	return c.transaction.CommitStepTransition(ctx, fence, commit, c.planner)
}

// StepTransitionService 编排 fence、提交内容、实例绑定和所有权校验后执行事实提交。
type StepTransitionService struct {
	committer FactCommitter
}

// NewStepTransitionService 构造步骤转换服务。
func NewStepTransitionService(committer FactCommitter) StepTransitionService {
	return StepTransitionService{committer: committer}
}

// Commit 校验 fence、提交内容及实例绑定，复制提交值后委托事实提交器并复制返回的提升切片。
func (s StepTransitionService) Commit(ctx context.Context, fence domainexecution.WorkerFence, commit evidence.StepTransitionCommit) (evidence.StepTransitionCommitResult, error) {
	if isNilInterface(s.committer.transaction) || isNilInterface(s.committer.planner) {
		return evidence.StepTransitionCommitResult{}, FactCommitterRequiredError()
	}
	// 两个校验器各自返回分类错误；此处不使用未分类 fmt.Errorf 包装，避免在公共边界的已编码错误外层
	// 添加未分类层，使宿主退回笼统 INTERNAL 响应。
	if err := fence.Validate(); err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	if err := commit.Validate(); err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	if err := validateCommitInstanceBinding(fence.InstanceID, commit); err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	owned, err := ownStepTransitionCommit(commit)
	if err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	result, err := s.committer.CommitStepTransition(ctx, fence, owned)
	if err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	result.Promotions = append([]evidence.NodeVersionPromotion(nil), result.Promotions...)
	return result, nil
}

// validateCommitInstanceBinding 校验最终验证、分组验证和自愈观测的 InstanceID 与 worker fence 一致。
func validateCommitInstanceBinding(instanceID domainexecution.InstanceID, commit evidence.StepTransitionCommit) error {
	for _, observation := range commit.FinalValidations {
		if observation.InstanceID != instanceID {
			return stepTransitionCommitInstanceMismatchError(fmt.Errorf("validation observation instance %q does not match worker fence instance %q", observation.InstanceID, instanceID))
		}
	}
	for _, group := range commit.FinalValidationGroups {
		if group.InstanceID != instanceID {
			return stepTransitionCommitInstanceMismatchError(fmt.Errorf("validation group instance %q does not match worker fence instance %q", group.InstanceID, instanceID))
		}
	}
	for _, observation := range commit.HealObservations {
		if observation.InstanceID != instanceID {
			return stepTransitionCommitInstanceMismatchError(fmt.Errorf("heal observation instance %q does not match worker fence instance %q", observation.InstanceID, instanceID))
		}
	}
	return nil
}

// isNilInterface 识别直接为 nil 或承载 typed nil 的接口值。
func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// ProgressWriter 在活动 worker claim 下持久化非终态执行观测。
type ProgressWriter interface {
	// RecordStepProgress 记录步骤进度事件。
	RecordStepProgress(context.Context, domainexecution.WorkerFence, evidence.StepProgressEvent) error
	// RecordValidationProgress 记录验证进度观测。
	RecordValidationProgress(context.Context, domainexecution.WorkerFence, evidence.ValidationProgressObservation) error
}
