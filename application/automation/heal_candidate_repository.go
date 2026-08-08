package automation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

const (
	healReviewDigestV1 = "heal-review-v1"

	// CodeHealReviewIdentityConflict 表示审核请求身份与已存在请求不一致。
	CodeHealReviewIdentityConflict fault.Code = "AUTOMATION_HEAL_REVIEW_IDENTITY_CONFLICT"
	// CodeHealReviewDecisionConflict 表示候选已不再处于可审核状态。
	CodeHealReviewDecisionConflict fault.Code = "AUTOMATION_HEAL_REVIEW_DECISION_CONFLICT"
	// CodeHealReviewAuthorityConflict 表示审核期间候选或节点的权威状态发生变化。
	CodeHealReviewAuthorityConflict fault.Code = "AUTOMATION_HEAL_REVIEW_AUTHORITY_CONFLICT"
	// CodeHealReviewContractViolation 表示事务适配器返回的审核结果违反内部契约。
	CodeHealReviewContractViolation fault.Code = "AUTOMATION_HEAL_REVIEW_CONTRACT_VIOLATION"
)

// HealReviewIdentityConflictError 构造审核请求身份冲突错误。
func HealReviewIdentityConflictError() error {
	return newHealReviewFault(
		fault.Conflict,
		CodeHealReviewIdentityConflict,
		"heal review command conflicts with an existing request",
	)
}

// HealReviewDecisionConflictError 构造候选不可再审核的前置条件错误。
func HealReviewDecisionConflictError() error {
	return newHealReviewFault(
		fault.FailedPrecondition,
		CodeHealReviewDecisionConflict,
		"heal candidate is no longer available for review",
	)
}

// HealReviewAuthorityConflictError 构造审核完成前权威状态发生变化的冲突错误。
func HealReviewAuthorityConflictError() error {
	return newHealReviewFault(
		fault.Conflict,
		CodeHealReviewAuthorityConflict,
		"heal review authority changed before the operation completed",
	)
}

// classifyHealReviewCommand 为格式错误的审核命令补上 domain/automation 已发布的调用方错误码，
// 并让已有分类的错误直接通过；尤其保留描述持久化修订这一独立维度的错误码。
//
// 此函数不会复用 AUTOMATION_HEAL_REVIEW_CONTRACT_VIOLATION。该错误码属于 INTERNAL，注册表中
// 明确表示适配器结果格式错误；调用方提供的无效命令应归为 INVALID_ARGUMENT，因为调用方可以纠正它，
// 报告为 INTERNAL 会向宿主传达相反的信息。
func classifyHealReviewCommand(cause error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	err, constructionErr := fault.Wrap(
		cause,
		fault.InvalidArgument,
		domain.CodeHealCandidateReviewCommandInvalid,
		"heal candidate review command is invalid",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// healReviewContractViolationError 将审核事务结果或内部转换失败包装为契约违规错误。
func healReviewContractViolationError(cause error) error {
	err, constructionErr := fault.Wrap(
		cause,
		fault.Internal,
		CodeHealReviewContractViolation,
		"heal review could not be completed",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// newHealReviewFault 构造并返回已注册的审核错误；错误码配置失败表示程序错误。
func newHealReviewFault(kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.New(kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// HealReviewDecision 表示审核者对自愈候选作出的决定。
type HealReviewDecision string

const (
	// HealReviewApprove 表示批准候选并提升为节点版本。
	HealReviewApprove HealReviewDecision = "APPROVE"
	// HealReviewReject 表示拒绝候选并推进拒绝 streak。
	HealReviewReject HealReviewDecision = "REJECT"
)

// HealReviewIntent 描述一次审核事务预期提交的候选、节点或 streak 状态及其权威修订。
type HealReviewIntent struct {
	CommandID                 string
	RequestDigest             string
	Decision                  HealReviewDecision
	ElementTargetID           string
	BaseNodeVersionID         string
	CandidateHash             string
	ExpectedCandidateRevision domain.Revision
	ExpectedNodeRevision      domain.Revision
	ExpectedStreak            *domain.HealStreak
	ExpectedStreakDigest      string
	NextCandidate             domain.HealCandidate
	NextNode                  *domain.ElementTargetAggregate
	NextStreak                *domain.HealStreak
	ReviewedBy                string
	ReviewedAt                int64
}

// HealReviewStatus 表示审核事务结果是首次应用还是幂等重放。
type HealReviewStatus string

const (
	// HealReviewApplied 表示本次审核事务首次应用成功。
	HealReviewApplied HealReviewStatus = "APPLIED"
	// HealReviewReplayed 表示命令已应用过，本次返回已持久化结果。
	HealReviewReplayed HealReviewStatus = "REPLAYED"
)

// HealReviewResult 保存审核决定及其产生的候选、节点和 streak 结果。
type HealReviewResult struct {
	Decision      HealReviewDecision
	Candidate     domain.HealCandidate
	ElementTarget *domain.ElementTargetAggregate
	Streak        *domain.HealStreak
}

// HealReviewOutcome 保存幂等键、请求摘要、状态和审核结果。
type HealReviewOutcome struct {
	Status        HealReviewStatus
	CommandID     string
	RequestDigest string
	Result        HealReviewResult
}

// HealReviewTransaction 定义审核结果的幂等查询和原子提交端口。
type HealReviewTransaction interface {
	// LookupHealReview 按命令 ID 和请求摘要查找既有审核结果；未找到时返回 found=false。
	LookupHealReview(context.Context, string, string) (HealReviewOutcome, bool, error)
	// CommitHealReview 校验并原子提交审核意图；重复命令应返回已持久化结果。
	CommitHealReview(context.Context, HealReviewIntent) (HealReviewOutcome, error)
}

// HealReviewRequest 保存审核命令的不可变身份和候选、节点的期望修订。
type HealReviewRequest struct {
	CommandID                 string
	Decision                  HealReviewDecision
	ElementTargetID           string
	BaseNodeVersionID         string
	CandidateHash             string
	ExpectedCandidateRevision domain.Revision
	ExpectedNodeRevision      domain.Revision
}

// Validate 校验审核请求形状，并将调用方可纠正的输入分类为 INVALID_ARGUMENT。
func (request HealReviewRequest) Validate() error {
	return classifyHealReviewCommand(request.checkShape())
}

// checkShape 校验命令身份、决定以及候选和节点的非零期望修订。
func (request HealReviewRequest) checkShape() error {
	if strings.TrimSpace(request.CommandID) == "" || strings.TrimSpace(request.ElementTargetID) == "" || strings.TrimSpace(request.BaseNodeVersionID) == "" || strings.TrimSpace(request.CandidateHash) == "" {
		return fmt.Errorf("heal review request requires command, node, base version, and candidate identity")
	}
	if request.Decision != HealReviewApprove && request.Decision != HealReviewReject {
		return fmt.Errorf("unsupported heal review decision %q", request.Decision)
	}
	// REQUEST 中的零期望修订可由调用方纠正：调用方读取权威修订后重新提交即可。
	// 若此处改用 ValidatePersisted，会产生 FAILED_PRECONDITION 持久化状态错误，分类器会直接
	// 放行它，让调用方误以为需要调整持久化状态，而实际只需纠正自身参数。
	if request.ExpectedCandidateRevision == 0 {
		return fmt.Errorf("heal review request requires the expected candidate revision")
	}
	if request.ExpectedNodeRevision == 0 {
		return fmt.Errorf("heal review request requires the expected node revision")
	}
	return nil
}

// HealReviewRequestIdentityDigest 为审核命令身份生成带 schema 前缀的 SHA-256 摘要。
func HealReviewRequestIdentityDigest(request HealReviewRequest) (string, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(struct {
		Schema  string
		Request HealReviewRequest
	}{Schema: healReviewDigestV1, Request: request})
	if err != nil {
		return "", fmt.Errorf("encode heal review request identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

// HealReviewStreakDigest 为审核拒绝所依据的 streak 权威快照生成 SHA-256 摘要。
func HealReviewStreakDigest(streak domain.HealStreak) (string, error) {
	encoded, err := json.Marshal(streak)
	if err != nil {
		return "", fmt.Errorf("encode heal review streak authority: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

// Validate 将审核意图失败分类为 INTERNAL 契约违规，而不是调用方参数错误。意图由审核服务
// 根据适配器结果构造，或从持久化数据重放；外部命令调用方无法改变其中的状态转换不变量，
// 因此这些失败归为 INTERNAL。已有分类的错误（如持久化修订错误码）保持不变。
func (intent HealReviewIntent) Validate() error {
	err := intent.checkShape()
	if err == nil {
		return nil
	}
	if _, classified := fault.CodeOf(err); classified {
		return err
	}
	return healReviewContractViolationError(err)
}

// checkShape 校验审核意图的身份、可信审核元数据以及批准或拒绝转换的不变量。
func (intent HealReviewIntent) checkShape() error {
	if strings.TrimSpace(intent.CommandID) == "" || strings.TrimSpace(intent.ElementTargetID) == "" || strings.TrimSpace(intent.BaseNodeVersionID) == "" || strings.TrimSpace(intent.CandidateHash) == "" {
		return fmt.Errorf("heal review intent requires command, node, base version, and candidate identity")
	}
	if strings.TrimSpace(intent.ReviewedBy) == "" || intent.ReviewedAt <= 0 {
		return fmt.Errorf("heal review intent requires trusted reviewer metadata")
	}
	if err := intent.ExpectedCandidateRevision.ValidatePersisted(); err != nil {
		// ValidatePersisted 已返回 AUTOMATION_PERSISTED_REVISION_INVALID。
		return err
	}
	if err := intent.ExpectedNodeRevision.ValidatePersisted(); err != nil {
		return err
	}
	if intent.NextCandidate.Hash != intent.CandidateHash || intent.NextCandidate.ElementTargetID != intent.ElementTargetID || intent.NextCandidate.BaseNodeVersionID != intent.BaseNodeVersionID || intent.NextCandidate.Revision != intent.ExpectedCandidateRevision+1 {
		return fmt.Errorf("heal review candidate transition does not match authority")
	}
	switch intent.Decision {
	case HealReviewApprove:
		if intent.NextCandidate.Status != domain.HealCandidatePromoted || intent.NextNode == nil || intent.ExpectedStreak != nil || intent.NextStreak != nil {
			return fmt.Errorf("approval requires promoted candidate and node only")
		}
		if intent.NextNode.ElementTarget.ID != intent.ElementTargetID || intent.NextNode.ElementTarget.Revision != intent.ExpectedNodeRevision+1 || intent.NextNode.Current.ID == intent.BaseNodeVersionID {
			return fmt.Errorf("approval node transition does not match authority")
		}
	case HealReviewReject:
		if intent.NextCandidate.Status != domain.HealCandidateRejected || intent.NextNode != nil || intent.ExpectedStreak == nil || intent.NextStreak == nil {
			return fmt.Errorf("rejection requires rejected candidate and streak transition only")
		}
		if strings.TrimSpace(intent.ExpectedStreakDigest) == "" || intent.ExpectedStreak.ElementTargetID != intent.ElementTargetID || intent.ExpectedStreak.BaseNodeVersionID != intent.BaseNodeVersionID || intent.ExpectedStreak.CandidateHash != intent.CandidateHash || intent.NextStreak.ElementTargetID != intent.ElementTargetID || intent.NextStreak.BaseNodeVersionID != intent.BaseNodeVersionID || intent.NextStreak.CandidateHash != intent.CandidateHash || intent.NextStreak.Disposition != domain.HealStreakRejected || intent.NextStreak.LastSequence <= intent.ExpectedStreak.LastSequence {
			return fmt.Errorf("rejection streak transition does not match authority")
		}
	default:
		return fmt.Errorf("unsupported heal review decision %q", intent.Decision)
	}
	return nil
}

// HealReviewRequestDigest 从完整审核意图提取不可变请求身份并生成摘要。
func HealReviewRequestDigest(intent HealReviewIntent) (string, error) {
	if err := intent.Validate(); err != nil {
		return "", err
	}
	return HealReviewRequestIdentityDigest(HealReviewRequest{
		CommandID: intent.CommandID, Decision: intent.Decision, ElementTargetID: intent.ElementTargetID,
		BaseNodeVersionID: intent.BaseNodeVersionID, CandidateHash: intent.CandidateHash,
		ExpectedCandidateRevision: intent.ExpectedCandidateRevision, ExpectedNodeRevision: intent.ExpectedNodeRevision,
	})
}

// ValidateHealReviewIntentDigest 校验意图中的请求摘要与其身份字段一致。
func ValidateHealReviewIntentDigest(intent HealReviewIntent) error {
	digest, err := HealReviewRequestDigest(intent)
	if err != nil {
		return err
	}
	if intent.RequestDigest != digest {
		return HealReviewIdentityConflictError()
	}
	return nil
}

// cloneHealReviewIntent 深拷贝审核意图中的候选、节点和 streak，避免跨边界共享可变状态。
func cloneHealReviewIntent(intent HealReviewIntent) HealReviewIntent {
	result := intent
	result.NextCandidate = cloneHealCandidate(intent.NextCandidate)
	result.NextNode = cloneNodePointer(intent.NextNode)
	result.ExpectedStreak = cloneHealStreakPointer(intent.ExpectedStreak)
	result.NextStreak = cloneHealStreakPointer(intent.NextStreak)
	return result
}

// cloneHealReviewOutcome 深拷贝审核结果中的候选、节点和 streak，保持返回值所有权独立。
func cloneHealReviewOutcome(outcome HealReviewOutcome) HealReviewOutcome {
	result := outcome
	result.Result.Candidate = cloneHealCandidate(outcome.Result.Candidate)
	result.Result.ElementTarget = cloneNodePointer(outcome.Result.ElementTarget)
	result.Result.Streak = cloneHealStreakPointer(outcome.Result.Streak)
	return result
}

// cloneHealCandidate 深拷贝候选的选择器和指纹，保留调用方与事务之间的所有权边界。
func cloneHealCandidate(candidate domain.HealCandidate) domain.HealCandidate {
	result := candidate
	result.Selectors = append([]fingerprint.Selector(nil), candidate.Selectors...)
	result.Fingerprint = candidate.Fingerprint.Clone()
	return result
}

// cloneNodePointer 在节点非 nil 时返回独立副本，nil 仍保持 nil。
func cloneNodePointer(node *domain.ElementTargetAggregate) *domain.ElementTargetAggregate {
	if node == nil {
		return nil
	}
	result := node.Clone()
	return &result
}

// cloneHealStreakPointer 在 streak 非 nil 时返回独立副本，nil 仍保持 nil。
func cloneHealStreakPointer(streak *domain.HealStreak) *domain.HealStreak {
	if streak == nil {
		return nil
	}
	result := streak.Clone()
	return &result
}
