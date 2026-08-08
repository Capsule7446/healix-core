package automation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
)

const samplingPublicationDigestV1 = "sampling-publication-v1"

const (
	// CodeSamplingPublicationIdentityConflict 表示同一发布 ID 对应了不同请求身份或摘要。
	CodeSamplingPublicationIdentityConflict fault.Code = "SAMPLING_PUBLICATION_IDENTITY_CONFLICT"
	// CodeSamplingPublicationDigestMismatch 表示请求负载与提供的摘要不一致。
	CodeSamplingPublicationDigestMismatch fault.Code = "AUTOMATION_SAMPLING_PUBLICATION_DIGEST_MISMATCH"
	// CodeSamplingPublicationUnavailable 表示采样发布事务依赖不可用。
	CodeSamplingPublicationUnavailable fault.Code = "AUTOMATION_SAMPLING_PUBLICATION_UNAVAILABLE"
	// CodeSamplingPublicationContractViolation 表示事务适配器返回结果违反发布契约。
	CodeSamplingPublicationContractViolation fault.Code = "AUTOMATION_SAMPLING_PUBLICATION_ADAPTER_CONTRACT_VIOLATION"
	// CodeSamplingPublicationAuthorityConflict 表示发布期间节点或流程权威发生变化。
	CodeSamplingPublicationAuthorityConflict fault.Code = "AUTOMATION_SAMPLING_PUBLICATION_AUTHORITY_CONFLICT"
	// CodeSamplingPublicationCommandInvalid 是 validateSamplingPublicationCommand 及其调用方的边界码，
	// 表示发布 ID 为空或发布内容自身无效；已分类的内容错误
	// AUTOMATION_SAMPLING_PUBLICATION_CONTENT_INVALID 保持原样，不再包裹第二个错误码。
	CodeSamplingPublicationCommandInvalid fault.Code = "SAMPLING_PUBLICATION_COMMAND_INVALID"
)

// classifySamplingPublicationCommand 是 validateSamplingPublicationCommand 和
// ValidatePublishSamplingIntentDigest 共用的边界分类器。命令细节（空 ID 或被包装 cause 携带的
// 校验文本）保持私有，仅可通过 errors.Unwrap 访问。
func classifySamplingPublicationCommand(cause error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	err, constructionErr := fault.Wrap(
		cause,
		fault.InvalidArgument,
		CodeSamplingPublicationCommandInvalid,
		"sampling publication command is invalid",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// SamplingPublicationIdentityConflictError 构造发布身份冲突错误。
func SamplingPublicationIdentityConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeSamplingPublicationIdentityConflict,
		"sampling publication identity conflicts with an existing request",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// SamplingPublicationDigestMismatchError 构造发布摘要与请求负载不一致的错误。
func SamplingPublicationDigestMismatchError() error {
	err, constructionErr := fault.New(
		fault.InvalidArgument,
		CodeSamplingPublicationDigestMismatch,
		"sampling publication digest does not match the request payload",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// SamplingPublicationAuthorityConflictError 构造发布期间权威状态变化的冲突错误。
func SamplingPublicationAuthorityConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeSamplingPublicationAuthorityConflict,
		"sampling publication authority changed before the publication could be applied",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// SamplingPublicationUnavailableError 构造采样发布事务不可用错误。
func SamplingPublicationUnavailableError() error {
	err, constructionErr := fault.New(
		fault.Unavailable,
		CodeSamplingPublicationUnavailable,
		"sampling publication service is unavailable",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// samplingPublicationContractViolationError 将适配器返回的非法结果包装为内部契约违规错误。
func samplingPublicationContractViolationError(cause error) error {
	err, constructionErr := fault.Wrap(
		cause,
		fault.Internal,
		CodeSamplingPublicationContractViolation,
		"sampling publication adapter returned an invalid outcome",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// SamplingPublicationCommand 保存发布 ID 及待持久化的领域发布内容。
type SamplingPublicationCommand struct {
	PublicationID string
	Publication   domain.SamplingPublication
}

// PublishSamplingIntent 携带发布事务提交所需的幂等身份、请求摘要和发布内容。
type PublishSamplingIntent struct {
	PublicationID string
	RequestDigest string
	Publication   domain.SamplingPublication
}

// PublishSamplingStatus 表示发布事务是首次应用还是幂等重放。
type PublishSamplingStatus string

const (
	// PublishSamplingApplied 表示发布事务首次应用成功。
	PublishSamplingApplied PublishSamplingStatus = "APPLIED"
	// PublishSamplingReplayed 表示发布已应用过，本次返回已持久化结果。
	PublishSamplingReplayed PublishSamplingStatus = "REPLAYED"
)

// PublishSamplingOutcome 保存发布事务状态、身份、摘要和领域结果。
type PublishSamplingOutcome struct {
	Status        PublishSamplingStatus
	PublicationID string
	RequestDigest string
	Result        domain.SamplingPublicationResult
}

// SamplingPublicationTransaction 定义发布结果的幂等查询和原子提交端口。
type SamplingPublicationTransaction interface {
	// LookupSamplingPublication 按发布 ID 和请求摘要查找既有结果；未找到时返回 found=false。
	LookupSamplingPublication(context.Context, string, string) (PublishSamplingOutcome, bool, error)
	// PublishSampling 校验并原子提交发布意图；重复请求应返回已持久化结果。
	PublishSampling(context.Context, PublishSamplingIntent) (PublishSamplingOutcome, error)
}

// SamplingPublicationService 编排采样发布的摘要校验、幂等重放和事务提交。
type SamplingPublicationService struct {
	transaction SamplingPublicationTransaction
}

// NewSamplingPublicationService 构造采样发布服务；事务为 nil 时由 Publish 返回不可用错误。
func NewSamplingPublicationService(transaction SamplingPublicationTransaction) SamplingPublicationService {
	return SamplingPublicationService{transaction: transaction}
}

// ValidatePublishSamplingIntentDigest 校验发布意图中的请求摘要与其 ID 和内容一致。
func ValidatePublishSamplingIntentDigest(intent PublishSamplingIntent) error {
	digest, err := SamplingPublicationRequestDigest(SamplingPublicationCommand{
		PublicationID: intent.PublicationID,
		Publication:   intent.Publication,
	})
	if err != nil {
		return classifySamplingPublicationCommand(err)
	}
	if intent.RequestDigest != digest {
		return SamplingPublicationDigestMismatchError()
	}
	return nil
}

// SamplingPublicationRequestDigest 为发布 ID 和完整内容生成带 schema 前缀的 SHA-256 摘要。
func SamplingPublicationRequestDigest(command SamplingPublicationCommand) (string, error) {
	owned := command
	owned.Publication = command.Publication.Clone()
	if err := validateSamplingPublicationCommand(owned); err != nil {
		return "", err
	}
	h := sha256.New()
	writeDigestString(h, samplingPublicationDigestV1)
	writeDigestString(h, owned.PublicationID)
	encodeCanonicalPayload(h, reflect.ValueOf(owned.Publication))
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// Publish 校验并提交采样发布；已存在的同身份请求返回独立的幂等结果副本。
func (s SamplingPublicationService) Publish(ctx context.Context, command SamplingPublicationCommand) (domain.SamplingPublicationResult, error) {
	if isNilDependency(s.transaction) {
		return domain.SamplingPublicationResult{}, SamplingPublicationUnavailableError()
	}
	owned := command
	owned.Publication = command.Publication.Clone()
	digest, err := SamplingPublicationRequestDigest(owned)
	if err != nil {
		return domain.SamplingPublicationResult{}, err
	}
	if s.transaction == nil {
		return domain.SamplingPublicationResult{}, SamplingPublicationUnavailableError()
	}
	replay, found, err := s.transaction.LookupSamplingPublication(ctx, owned.PublicationID, digest)
	if err != nil {
		return domain.SamplingPublicationResult{}, fmt.Errorf("lookup sampling publication: %w", err)
	}
	if found {
		if err := validatePublishSamplingOutcome(owned, digest, replay); err != nil {
			return domain.SamplingPublicationResult{}, samplingPublicationContractViolationError(err)
		}
		return cloneSamplingPublicationResult(replay.Result), nil
	}
	if s.transaction == nil {
		return domain.SamplingPublicationResult{}, SamplingPublicationUnavailableError()
	}
	transactionPublication := owned.Publication.Clone()
	outcome, err := s.transaction.PublishSampling(ctx, PublishSamplingIntent{
		PublicationID: owned.PublicationID,
		RequestDigest: digest,
		Publication:   transactionPublication,
	})
	if err != nil {
		return domain.SamplingPublicationResult{}, fmt.Errorf("publish sampling result: %w", err)
	}
	if err := validatePublishSamplingOutcome(owned, digest, outcome); err != nil {
		return domain.SamplingPublicationResult{}, samplingPublicationContractViolationError(err)
	}
	return cloneSamplingPublicationResult(outcome.Result), nil
}

// cloneSamplingPublicationResult 复制节点映射切片，返回调用方独立拥有的结果。
func cloneSamplingPublicationResult(result domain.SamplingPublicationResult) domain.SamplingPublicationResult {
	result.Nodes = append([]domain.SamplingNodeMapping(nil), result.Nodes...)
	return result
}

// validateSamplingPublicationCommand 校验发布 ID 和领域发布内容，并分类调用方输入错误。
func validateSamplingPublicationCommand(command SamplingPublicationCommand) error {
	if strings.TrimSpace(command.PublicationID) == "" {
		return classifySamplingPublicationCommand(errors.New("sampling publication id is required"))
	}
	if err := command.Publication.Validate(); err != nil {
		return classifySamplingPublicationCommand(err)
	}
	return nil
}

// validatePublishSamplingOutcome 校验适配器结果的状态、身份、流程版本和节点映射完整匹配。
func validatePublishSamplingOutcome(command SamplingPublicationCommand, digest string, outcome PublishSamplingOutcome) error {
	if outcome.Status != PublishSamplingApplied && outcome.Status != PublishSamplingReplayed {
		return fmt.Errorf("unsupported status %q", outcome.Status)
	}
	if outcome.PublicationID != command.PublicationID || outcome.RequestDigest != digest {
		return errors.New("outcome identity does not match request")
	}
	workflow := command.Publication.FlowFragment
	if outcome.Result.FlowFragmentID != workflow.FlowFragment.ID || outcome.Result.WorkflowVersionID != workflow.Current.ID || outcome.Result.VersionNumber != workflow.Current.VersionNumber {
		return errors.New("outcome workflow does not match publication")
	}
	if len(outcome.Result.Nodes) != len(command.Publication.Nodes) {
		return errors.New("outcome mappings do not exactly match publication")
	}
	for index, node := range command.Publication.Nodes {
		mapping := outcome.Result.Nodes[index]
		if mapping.TemporaryElementTargetID != node.TemporaryElementTargetID || mapping.ElementTargetID != node.Aggregate.ElementTarget.ID || mapping.ElementTargetVersionID != node.Aggregate.Current.ID || mapping.ResolutionMode != node.ResolutionMode {
			return errors.New("outcome mapping does not match publication")
		}
	}
	return nil
}
