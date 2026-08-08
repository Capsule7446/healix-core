package automation

import (
	"fmt"

	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/sampling"
)

// CodeSamplingPublicationAuthorityInvalid 是 MapSamplingPublication 和 mapSamplingNode 中所有调用方
// 形状错误的边界错误码，涵盖节点权威缺失或重复、权威与临时节点不匹配、解析模式未决或不受支持，
// 以及与当前聚合不一致的合并/复用权威。它区别于 AUTOMATION_SAMPLING_PUBLICATION_CONTENT_INVALID
// （由 domain/automation 分类的发布内容形状错误）和 SAMPLING_PUBLICATION_MAPPING_INVALID（元素目标
// 引用重写错误）。
const CodeSamplingPublicationAuthorityInvalid fault.Code = "SAMPLING_PUBLICATION_AUTHORITY_INVALID"

// classifySamplingPublicationAuthority 是 MapSamplingPublication 的唯一导出边界分类器。
// mapSamplingPublication 和 mapSamplingNode 内部校验保持普通 Go 错误；契约允许内部不变量以私有
// cause 形式传到此边界。它们用 %q 格式化的身份（临时/正式元素目标 ID、解析模式）不会对外暴露，
// 因为 fault 的公共 Error() 文本仅包含错误码和安全消息，不包含 cause。已分类的错误（元素目标
// 引用重写及下方流程片段/发布验证已返回各自领域错误码）保持原样，不再包裹第二个错误码。
func classifySamplingPublicationAuthority(cause error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	err, constructionErr := fault.Wrap(
		cause,
		fault.InvalidArgument,
		CodeSamplingPublicationAuthorityInvalid,
		"sampling publication authority is invalid",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// SamplingNodeAuthority 保存临时节点映射到正式元素目标及其版本所需的权威快照和 CAS 修订。
type SamplingNodeAuthority struct {
	TemporaryElementTargetID string
	ElementTargetID          string
	ElementTargetVersionID   string
	Current                  *domainautomation.ElementTargetAggregate
	ExpectedRevision         domainautomation.Revision
	ExpectedCurrentVersionID string
}

// SamplingPublicationRequest 携带采样工作区、发布身份、时间和每个节点的权威信息。
type SamplingPublicationRequest struct {
	FlowFragmentID    string
	WorkflowVersionID string
	PublishedAt       int64
	Workspace         sampling.UnpublishedFlowFragment
	Nodes             []SamplingNodeAuthority
}

// MapSamplingPublication 将采样工作区及其发布权威转换为领域 SamplingPublication；所有跨越该
// 边界的错误都由 classifySamplingPublicationAuthority 恰好分类一次。
func MapSamplingPublication(request SamplingPublicationRequest) (domainautomation.SamplingPublication, error) {
	publication, err := mapSamplingPublication(request)
	if err != nil {
		return domainautomation.SamplingPublication{}, classifySamplingPublicationAuthority(err)
	}
	return publication, nil
}

// mapSamplingPublication 校验发布身份和节点权威，重写工作区引用并构造领域发布聚合。
func mapSamplingPublication(request SamplingPublicationRequest) (domainautomation.SamplingPublication, error) {
	if request.PublishedAt <= 0 || request.FlowFragmentID == "" || request.WorkflowVersionID == "" {
		return domainautomation.SamplingPublication{}, fmt.Errorf("sampling publication requires workflow identity and publication time")
	}
	authorityByTemporaryID := make(map[string]SamplingNodeAuthority, len(request.Nodes))
	formalNodeIDs := make(map[string]struct{}, len(request.Nodes))
	formalVersionIDs := make(map[string]struct{}, len(request.Nodes))
	for _, authority := range request.Nodes {
		if authority.TemporaryElementTargetID == "" || authority.ElementTargetID == "" || authority.ElementTargetVersionID == "" {
			return domainautomation.SamplingPublication{}, fmt.Errorf("sampling node authority requires temporary and formal identity")
		}
		if _, exists := authorityByTemporaryID[authority.TemporaryElementTargetID]; exists {
			return domainautomation.SamplingPublication{}, fmt.Errorf("duplicate sampling node authority %q", authority.TemporaryElementTargetID)
		}
		if _, exists := formalNodeIDs[authority.ElementTargetID]; exists {
			return domainautomation.SamplingPublication{}, fmt.Errorf("duplicate formal sampling node %q", authority.ElementTargetID)
		}
		if _, exists := formalVersionIDs[authority.ElementTargetVersionID]; exists {
			return domainautomation.SamplingPublication{}, fmt.Errorf("duplicate formal sampling node version %q", authority.ElementTargetVersionID)
		}
		authorityByTemporaryID[authority.TemporaryElementTargetID] = authority
		formalNodeIDs[authority.ElementTargetID] = struct{}{}
		formalVersionIDs[authority.ElementTargetVersionID] = struct{}{}
	}
	publications := make([]domainautomation.SamplingElementTargetPublication, len(request.Workspace.Nodes))
	mappings := make([]domainautomation.SamplingNodeMapping, len(request.Workspace.Nodes))
	for index, temporary := range request.Workspace.Nodes {
		authority, exists := authorityByTemporaryID[temporary.ID]
		if !exists {
			return domainautomation.SamplingPublication{}, fmt.Errorf("sampling node %q has no publication authority", temporary.ID)
		}
		delete(authorityByTemporaryID, temporary.ID)
		publication, err := mapSamplingNode(temporary, authority, request.PublishedAt)
		if err != nil {
			return domainautomation.SamplingPublication{}, err
		}
		publications[index] = publication
		mappings[index] = domainautomation.SamplingNodeMapping{TemporaryElementTargetID: temporary.ID, ElementTargetID: publication.Aggregate.ElementTarget.ID, ElementTargetVersionID: publication.Aggregate.Current.ID, ResolutionMode: publication.ResolutionMode}
	}
	if len(authorityByTemporaryID) != 0 {
		return domainautomation.SamplingPublication{}, fmt.Errorf("sampling node authority must exactly match temporary nodes")
	}
	steps, err := sampling.RewriteUnpublishedElementTargetReferences(request.Workspace.Steps, mappings)
	if err != nil {
		// 重写函数已返回带有有序违规明细的 SAMPLING_PUBLICATION_MAPPING_INVALID；此处不包裹，
		// 避免在公共边界的已编码错误外层再增加未分类层。
		return domainautomation.SamplingPublication{}, err
	}
	workflow, err := domainautomation.NewFlowFragment(
		domainautomation.FlowFragment{ID: request.FlowFragmentID, DisplayName: request.Workspace.DisplayName, Properties: request.Workspace.Properties.Clone(), CreatedAt: request.PublishedAt, UpdatedAt: request.PublishedAt},
		domainautomation.FlowFragmentVersion{ID: request.WorkflowVersionID, Definition: domainautomation.FlowFragmentContent{Steps: steps, Parameters: domainautomation.CloneParameterDefinitions(request.Workspace.Parameters)}, CreatedAt: request.PublishedAt},
	)
	if err != nil {
		// domainautomation.NewFlowFragment 在自身包边界分类错误；此处既不在其外层添加未分类层，
		// 也不掩盖已有错误码，而是原样交给外层 classifySamplingPublicationAuthority 处理，
		// 无论原错误已分类与否。
		return domainautomation.SamplingPublication{}, err
	}
	result := domainautomation.SamplingPublication{Nodes: publications, FlowFragment: workflow}
	if err := result.Validate(); err != nil {
		return domainautomation.SamplingPublication{}, err
	}
	return result, nil
}

// referenceableElementTargetVersion 在聚合历史中查找指定的未删除元素目标版本。
func referenceableElementTargetVersion(aggregate domainautomation.ElementTargetAggregate, versionID string) (domainautomation.ElementTargetVersion, bool) {
	for _, version := range aggregate.Versions {
		if version.ID == versionID && version.DeletedAt == 0 {
			return version, true
		}
	}
	return domainautomation.ElementTargetVersion{}, false
}

// mapSamplingNode 按解析模式将临时节点与创建、合并或复用的正式聚合映射，并保留权威修订。
func mapSamplingNode(temporary sampling.UnpublishedElementTarget, authority SamplingNodeAuthority, at int64) (domainautomation.SamplingElementTargetPublication, error) {
	mode := temporary.ResolutionMode
	publication := domainautomation.SamplingElementTargetPublication{TemporaryElementTargetID: temporary.ID, ResolutionMode: string(mode)}
	switch mode {
	case sampling.ResolutionModeCreate:
		if authority.Current != nil || authority.ExpectedRevision != 0 || authority.ExpectedCurrentVersionID != "" {
			return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("sampling node %q create cannot carry current-node authority", temporary.ID)
		}
		aggregate, err := domainautomation.NewElementTarget(
			domainautomation.ElementTarget{ID: authority.ElementTargetID, DisplayName: temporary.DisplayName, Properties: temporary.Properties.Clone(), CreatedAt: at, UpdatedAt: at},
			domainautomation.ElementTargetVersion{ID: authority.ElementTargetVersionID, PageURL: temporary.PageURL, Origin: temporary.Origin, Selectors: temporary.Selectors, Fingerprint: temporary.Fingerprint, Source: domainautomation.SourceSampling, CreatedAt: at},
		)
		if err != nil {
			// 此处包装不包含 %q 身份，使 domain/automation.NewElementTarget 返回已分类 cause 时，
			// 边界原样传递错误也不会泄露临时 ID。
			return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("build sampled node: %w", err)
		}
		publication.Aggregate, publication.PublishVersion = aggregate, true
	case sampling.ResolutionModeMerge:
		if authority.Current == nil || authority.Current.ElementTarget.ID != authority.ElementTargetID || authority.ExpectedRevision == 0 || authority.ExpectedCurrentVersionID == "" || authority.Current.ElementTarget.Revision != authority.ExpectedRevision || authority.Current.ElementTarget.CurrentVersionID != authority.ExpectedCurrentVersionID {
			return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("sampling node %q merge requires exact current node authority", temporary.ID)
		}
		aggregate, err := authority.Current.PublishVersion(authority.ElementTargetVersionID, temporary.PageURL, temporary.Origin, temporary.Selectors, temporary.Fingerprint, domainautomation.SourceSampling, at)
		if err != nil {
			return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("merge sampled node: %w", err)
		}
		publication.Aggregate, publication.ExpectedRevision, publication.ExpectedCurrentVersionID, publication.PublishVersion = aggregate, authority.ExpectedRevision, authority.ExpectedCurrentVersionID, true
	case sampling.ResolutionModeReuse:
		if authority.Current == nil || authority.Current.ElementTarget.ID != authority.ElementTargetID || authority.ExpectedRevision == 0 || authority.Current.ElementTarget.Revision != authority.ExpectedRevision || authority.Current.ElementTarget.CurrentVersionID != authority.ExpectedCurrentVersionID || authority.ExpectedCurrentVersionID != authority.Current.Current.ID {
			return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("sampling node %q reuse requires exact current aggregate authority", temporary.ID)
		}
		if err := authority.Current.ValidateLoadedHistory(); err != nil {
			return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("sampling node reuse requires valid loaded authority history: %w", err)
		}
		publication.Aggregate = authority.Current.Clone()
		selected, ok := referenceableElementTargetVersion(publication.Aggregate, authority.ElementTargetVersionID)
		if !ok {
			return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("sampling node %q reuse requires a referenceable selected version", temporary.ID)
		}
		publication.Aggregate.Current = selected
		publication.ExpectedRevision, publication.ExpectedCurrentVersionID = authority.ExpectedRevision, authority.ExpectedCurrentVersionID
	case sampling.ResolutionModeUndecided:
		return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("sampling node %q resolution is undecided", temporary.ID)
	default:
		return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("sampling node %q has unsupported resolution mode %q", temporary.ID, mode)
	}
	return publication, nil
}
