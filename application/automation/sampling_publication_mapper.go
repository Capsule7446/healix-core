package automation

import (
	"fmt"

	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/sampling"
)

type SamplingNodeAuthority struct {
	TemporaryElementTargetID string
	ElementTargetID          string
	ElementTargetVersionID   string
	Current                  *domainautomation.ElementTargetAggregate
	ExpectedRevision         domainautomation.Revision
	ExpectedCurrentVersionID string
}

type SamplingPublicationRequest struct {
	FlowFragmentID    string
	WorkflowVersionID string
	PublishedAt       int64
	Workspace         sampling.UnpublishedFlowFragment
	Nodes             []SamplingNodeAuthority
}

func MapSamplingPublication(request SamplingPublicationRequest) (domainautomation.SamplingPublication, error) {
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
		return domainautomation.SamplingPublication{}, fmt.Errorf("rewrite sampled workflow references: %w", err)
	}
	workflow, err := domainautomation.NewFlowFragment(
		domainautomation.FlowFragment{ID: request.FlowFragmentID, DisplayName: request.Workspace.DisplayName, Properties: request.Workspace.Properties.Clone(), CreatedAt: request.PublishedAt, UpdatedAt: request.PublishedAt},
		domainautomation.FlowFragmentVersion{ID: request.WorkflowVersionID, Definition: domainautomation.FlowFragmentContent{Steps: steps, Parameters: append([]domainautomation.ParameterDefinition(nil), request.Workspace.Parameters...)}, CreatedAt: request.PublishedAt},
	)
	if err != nil {
		return domainautomation.SamplingPublication{}, fmt.Errorf("build sampled workflow: %w", err)
	}
	result := domainautomation.SamplingPublication{Nodes: publications, FlowFragment: workflow}
	if err := result.Validate(); err != nil {
		return domainautomation.SamplingPublication{}, err
	}
	return result, nil
}

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
			return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("build sampled node %q: %w", temporary.ID, err)
		}
		publication.Aggregate, publication.PublishVersion = aggregate, true
	case sampling.ResolutionModeMerge:
		if authority.Current == nil || authority.Current.ElementTarget.ID != authority.ElementTargetID || authority.ExpectedRevision == 0 || authority.ExpectedCurrentVersionID == "" || authority.Current.ElementTarget.Revision != authority.ExpectedRevision || authority.Current.ElementTarget.CurrentVersionID != authority.ExpectedCurrentVersionID {
			return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("sampling node %q merge requires exact current node authority", temporary.ID)
		}
		aggregate, err := authority.Current.PublishVersion(authority.ElementTargetVersionID, temporary.PageURL, temporary.Origin, temporary.Selectors, temporary.Fingerprint, domainautomation.SourceSampling, at)
		if err != nil {
			return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("merge sampled node %q: %w", temporary.ID, err)
		}
		publication.Aggregate, publication.ExpectedRevision, publication.ExpectedCurrentVersionID, publication.PublishVersion = aggregate, authority.ExpectedRevision, authority.ExpectedCurrentVersionID, true
	case sampling.ResolutionModeReuse:
		if authority.Current == nil || authority.Current.ElementTarget.ID != authority.ElementTargetID || authority.Current.Current.ID != authority.ElementTargetVersionID || authority.ExpectedRevision == 0 || authority.Current.ElementTarget.Revision != authority.ExpectedRevision || authority.Current.ElementTarget.CurrentVersionID != authority.ExpectedCurrentVersionID || authority.ExpectedCurrentVersionID != authority.ElementTargetVersionID {
			return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("sampling node %q reuse requires exact current node authority", temporary.ID)
		}
		publication.Aggregate = authority.Current.Clone()
		publication.ExpectedRevision, publication.ExpectedCurrentVersionID = authority.ExpectedRevision, authority.ExpectedCurrentVersionID
	case sampling.ResolutionModeUndecided:
		return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("sampling node %q resolution is undecided", temporary.ID)
	default:
		return domainautomation.SamplingElementTargetPublication{}, fmt.Errorf("sampling node %q has unsupported resolution mode %q", temporary.ID, mode)
	}
	return publication, nil
}
