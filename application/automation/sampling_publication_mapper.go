package automation

import (
	"fmt"

	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/sampling"
)

type SamplingNodeAuthority struct {
	TemporaryNodeID          string
	NodeID                   string
	NodeVersionID            string
	Current                  *domainautomation.NodeAggregate
	ExpectedRevision         domainautomation.Revision
	ExpectedCurrentVersionID string
	ForceCreateAuthorized    bool
}

type SamplingPublicationRequest struct {
	FlowFragmentID    string
	WorkflowVersionID string
	PublishedAt       int64
	Workspace         sampling.TemporarySamplingWorkflow
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
		if authority.TemporaryNodeID == "" || authority.NodeID == "" || authority.NodeVersionID == "" {
			return domainautomation.SamplingPublication{}, fmt.Errorf("sampling node authority requires temporary and formal identity")
		}
		if _, exists := authorityByTemporaryID[authority.TemporaryNodeID]; exists {
			return domainautomation.SamplingPublication{}, fmt.Errorf("duplicate sampling node authority %q", authority.TemporaryNodeID)
		}
		if _, exists := formalNodeIDs[authority.NodeID]; exists {
			return domainautomation.SamplingPublication{}, fmt.Errorf("duplicate formal sampling node %q", authority.NodeID)
		}
		if _, exists := formalVersionIDs[authority.NodeVersionID]; exists {
			return domainautomation.SamplingPublication{}, fmt.Errorf("duplicate formal sampling node version %q", authority.NodeVersionID)
		}
		authorityByTemporaryID[authority.TemporaryNodeID] = authority
		formalNodeIDs[authority.NodeID] = struct{}{}
		formalVersionIDs[authority.NodeVersionID] = struct{}{}
	}
	publications := make([]domainautomation.SamplingNodePublication, len(request.Workspace.Nodes))
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
		mappings[index] = domainautomation.SamplingNodeMapping{TemporaryNodeID: temporary.ID, NodeID: publication.Aggregate.Node.ID, NodeVersionID: publication.Aggregate.Current.ID, ResolutionMode: publication.ResolutionMode}
	}
	if len(authorityByTemporaryID) != 0 {
		return domainautomation.SamplingPublication{}, fmt.Errorf("sampling node authority must exactly match temporary nodes")
	}
	steps, err := sampling.RewriteTemporaryNodeReferences(request.Workspace.Steps, mappings)
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

func mapSamplingNode(temporary sampling.TemporarySamplingNode, authority SamplingNodeAuthority, at int64) (domainautomation.SamplingNodePublication, error) {
	mode := temporary.ResolutionMode
	publication := domainautomation.SamplingNodePublication{TemporaryNodeID: temporary.ID, ResolutionMode: string(mode)}
	switch mode {
	case sampling.SamplingResolutionCreate, sampling.SamplingResolutionForceCreate:
		if authority.Current != nil || authority.ExpectedRevision != 0 || authority.ExpectedCurrentVersionID != "" {
			return domainautomation.SamplingNodePublication{}, fmt.Errorf("sampling node %q create cannot carry current-node authority", temporary.ID)
		}
		if mode == sampling.SamplingResolutionForceCreate && !authority.ForceCreateAuthorized {
			return domainautomation.SamplingNodePublication{}, fmt.Errorf("sampling node %q force create is not authorized", temporary.ID)
		}
		aggregate, err := domainautomation.NewNode(
			domainautomation.Node{ID: authority.NodeID, DisplayName: temporary.DisplayName, Properties: temporary.Properties.Clone(), CreatedAt: at, UpdatedAt: at},
			domainautomation.NodeVersion{ID: authority.NodeVersionID, PageURL: temporary.PageURL, Origin: temporary.Origin, Selectors: temporary.Selectors, Fingerprint: temporary.Fingerprint, Source: domainautomation.SourceSampling, CreatedAt: at},
		)
		if err != nil {
			return domainautomation.SamplingNodePublication{}, fmt.Errorf("build sampled node %q: %w", temporary.ID, err)
		}
		publication.Aggregate, publication.PublishVersion = aggregate, true
	case sampling.SamplingResolutionMerge:
		if authority.Current == nil || authority.Current.Node.ID != authority.NodeID || authority.ExpectedRevision == 0 || authority.ExpectedCurrentVersionID == "" || authority.Current.Node.Revision != authority.ExpectedRevision || authority.Current.Node.CurrentVersionID != authority.ExpectedCurrentVersionID {
			return domainautomation.SamplingNodePublication{}, fmt.Errorf("sampling node %q merge requires exact current node authority", temporary.ID)
		}
		aggregate, err := authority.Current.PublishVersion(authority.NodeVersionID, temporary.PageURL, temporary.Origin, temporary.Selectors, temporary.Fingerprint, domainautomation.SourceSampling, at)
		if err != nil {
			return domainautomation.SamplingNodePublication{}, fmt.Errorf("merge sampled node %q: %w", temporary.ID, err)
		}
		publication.Aggregate, publication.ExpectedRevision, publication.ExpectedCurrentVersionID, publication.PublishVersion = aggregate, authority.ExpectedRevision, authority.ExpectedCurrentVersionID, true
	case sampling.SamplingResolutionReuse:
		if authority.Current == nil || authority.Current.Node.ID != authority.NodeID || authority.Current.Current.ID != authority.NodeVersionID || authority.ExpectedRevision == 0 || authority.Current.Node.Revision != authority.ExpectedRevision || authority.Current.Node.CurrentVersionID != authority.ExpectedCurrentVersionID || authority.ExpectedCurrentVersionID != authority.NodeVersionID {
			return domainautomation.SamplingNodePublication{}, fmt.Errorf("sampling node %q reuse requires exact current node authority", temporary.ID)
		}
		publication.Aggregate = authority.Current.Clone()
		publication.ExpectedRevision, publication.ExpectedCurrentVersionID = authority.ExpectedRevision, authority.ExpectedCurrentVersionID
	case sampling.SamplingResolutionUndecided:
		return domainautomation.SamplingNodePublication{}, fmt.Errorf("sampling node %q resolution is undecided", temporary.ID)
	default:
		return domainautomation.SamplingNodePublication{}, fmt.Errorf("sampling node %q has unsupported resolution mode %q", temporary.ID, mode)
	}
	return publication, nil
}
