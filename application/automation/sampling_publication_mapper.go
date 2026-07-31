package automation

import (
	"fmt"

	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/sampling"
)

// CodeSamplingPublicationAuthorityInvalid is the boundary code for every
// caller-supplied shape failure in MapSamplingPublication and mapSamplingNode:
// missing or duplicate node authority, an authority/temporary-node mismatch, an
// undecided or unsupported resolution mode, and a merge/reuse authority that
// does not match the current aggregate. It is distinct from
// AUTOMATION_SAMPLING_PUBLICATION_CONTENT_INVALID (the publication's own content
// shape, classified in domain/automation) and from
// SAMPLING_PUBLICATION_MAPPING_INVALID (the element-target reference rewrite).
const CodeSamplingPublicationAuthorityInvalid fault.Code = "SAMPLING_PUBLICATION_AUTHORITY_INVALID"

// classifySamplingPublicationAuthority is the single exported-boundary
// classifier for MapSamplingPublication. The checks inside mapSamplingPublication
// and mapSamplingNode stay ordinary Go errors — the contract permits that for
// internal invariants — and travel to this boundary as a private cause; their
// %q-formatted identities (temporary/formal element target ids, resolution
// modes) never surface because a fault's public Error() text is its code and
// safe message only, never its cause. An already-classified failure (the
// element-target reference rewrite and the flow fragment/publication validation
// below already return their own domain codes) passes through unchanged rather
// than being buried under a second code.
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

// MapSamplingPublication is the single exported boundary that turns a sampling
// workspace and its publication authority into a domain SamplingPublication.
// Every error crossing it is classified exactly once by
// classifySamplingPublicationAuthority.
func MapSamplingPublication(request SamplingPublicationRequest) (domainautomation.SamplingPublication, error) {
	publication, err := mapSamplingPublication(request)
	if err != nil {
		return domainautomation.SamplingPublication{}, classifySamplingPublicationAuthority(err)
	}
	return publication, nil
}

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
		// The rewrite already returns SAMPLING_PUBLICATION_MAPPING_INVALID with its
		// own ordered violations. Wrapping it in an unclassified error would put an
		// uncoded layer on the outside of a coded fault at the public boundary.
		return domainautomation.SamplingPublication{}, err
	}
	workflow, err := domainautomation.NewFlowFragment(
		domainautomation.FlowFragment{ID: request.FlowFragmentID, DisplayName: request.Workspace.DisplayName, Properties: request.Workspace.Properties.Clone(), CreatedAt: request.PublishedAt, UpdatedAt: request.PublishedAt},
		domainautomation.FlowFragmentVersion{ID: request.WorkflowVersionID, Definition: domainautomation.FlowFragmentContent{Steps: steps, Parameters: append([]domainautomation.ParameterDefinition(nil), request.Workspace.Parameters...)}, CreatedAt: request.PublishedAt},
	)
	if err != nil {
		// domainautomation.NewFlowFragment's own errors are classified at its own
		// package boundary (a parallel migration); this boundary neither adds an
		// uncoded layer on top nor buries a code that is already there — it passes
		// through unclassified or classified alike, and the outer
		// classifySamplingPublicationAuthority call resolves whichever it is.
		return domainautomation.SamplingPublication{}, err
	}
	result := domainautomation.SamplingPublication{Nodes: publications, FlowFragment: workflow}
	if err := result.Validate(); err != nil {
		return domainautomation.SamplingPublication{}, err
	}
	return result, nil
}

func referenceableElementTargetVersion(aggregate domainautomation.ElementTargetAggregate, versionID string) (domainautomation.ElementTargetVersion, bool) {
	for _, version := range aggregate.Versions {
		if version.ID == versionID && version.DeletedAt == 0 {
			return version, true
		}
	}
	return domainautomation.ElementTargetVersion{}, false
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
			// No %q identity in this wrap: if domain/automation's NewElementTarget is
			// later classified by a parallel migration, an outer fmt-wrapper that still
			// echoed the temporary id would leak it even after the boundary classifier
			// passes an already-classified cause through unchanged.
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
