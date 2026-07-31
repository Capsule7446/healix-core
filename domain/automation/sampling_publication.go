package automation

import (
	"errors"
	"fmt"
	"strings"
)

type SamplingElementTargetPublication struct {
	TemporaryElementTargetID string
	ResolutionMode           string
	Aggregate                ElementTargetAggregate
	ExpectedRevision         Revision
	ExpectedCurrentVersionID string
	PublishVersion           bool
}

type SamplingPublication struct {
	Nodes        []SamplingElementTargetPublication
	FlowFragment FlowFragmentAggregate
}

type SamplingNodeMapping struct {
	TemporaryElementTargetID string
	ElementTargetID          string
	ElementTargetVersionID   string
	ResolutionMode           string
}

type SamplingPublicationResult struct {
	FlowFragmentID    string
	WorkflowVersionID string
	VersionNumber     int
	Nodes             []SamplingNodeMapping
}

func (p SamplingPublication) Clone() SamplingPublication {
	cloned := SamplingPublication{FlowFragment: cloneWorkflowAggregate(p.FlowFragment)}
	cloned.Nodes = make([]SamplingElementTargetPublication, len(p.Nodes))
	for index, node := range p.Nodes {
		cloned.Nodes[index] = node
		cloned.Nodes[index].Aggregate = cloneNodeAggregate(node.Aggregate)
	}
	return cloned
}

func containsReferenceableElementTargetVersion(aggregate ElementTargetAggregate, versionID string) bool {
	if aggregate.Current.ID == versionID && aggregate.Current.DeletedAt == 0 {
		return true
	}
	for _, version := range aggregate.Versions {
		if version.ID == versionID && version.DeletedAt == 0 {
			return true
		}
	}
	return false
}

// Validate classifies at this single exported boundary. The checks below stay
// ordinary Go errors and travel on as a private cause; identities never reach
// public text.
func (p SamplingPublication) Validate() error {
	return classifySamplingPublicationContent(p.validateContent())
}

func (p SamplingPublication) validateContent() error {
	if err := p.FlowFragment.Validate(); err != nil {
		return fmt.Errorf("sampled workflow: %w", err)
	}
	seen := make(map[string]struct{}, len(p.Nodes))
	formalNodes := make(map[string]struct{}, len(p.Nodes))
	formalVersions := make(map[string]struct{}, len(p.Nodes))
	decisions := make(map[string]struct{}, len(p.Nodes))
	// Failures address a node by its 0-based position in the slice the caller
	// passed. Temporary and formal element target identities, selected version
	// identities, and the resolution mode are all caller data and stay out of the
	// message; the caller can index its own input.
	for index, node := range p.Nodes {
		if strings.TrimSpace(node.TemporaryElementTargetID) == "" {
			return fmt.Errorf("sampled node %d temporary id is required", index)
		}
		switch node.ResolutionMode {
		case "CREATE", "MERGE", "REUSE":
		default:
			return fmt.Errorf("sampled node %d has an unsupported resolution mode", index)
		}
		if _, ok := seen[node.TemporaryElementTargetID]; ok {
			return fmt.Errorf("duplicate sampled node at %d", index)
		}
		seen[node.TemporaryElementTargetID] = struct{}{}
		if node.ResolutionMode != "REUSE" {
			if err := node.Aggregate.Validate(); err != nil {
				return fmt.Errorf("sampled node %d: %w", index, err)
			}
		}
		if strings.TrimSpace(node.Aggregate.ElementTarget.ID) == "" || node.Aggregate.Current.ElementTargetID != node.Aggregate.ElementTarget.ID || node.Aggregate.Current.DeletedAt != 0 {
			return fmt.Errorf("sampled node %d selected version is not referenceable", index)
		}
		if node.ResolutionMode != "REUSE" && !containsReferenceableElementTargetVersion(node.Aggregate, node.Aggregate.Current.ID) {
			return fmt.Errorf("sampled node %d selected version is not referenceable", index)
		}
		switch node.ResolutionMode {
		case "CREATE":
			if node.ExpectedRevision != 0 || node.ExpectedCurrentVersionID != "" || !node.PublishVersion || node.Aggregate.Current.VersionNumber != 1 {
				return fmt.Errorf("sampled node %d new ownership must publish version 1 without current-node authority", index)
			}
		case "MERGE":
			expectedNextRevision, err := node.ExpectedRevision.Next()
			if err != nil {
				// Revision.Next already returns AUTOMATION_REVISION_EXHAUSTED; wrapping
				// it here would bury that code under an unclassified layer.
				return err
			}
			if node.ExpectedRevision == 0 || node.ExpectedCurrentVersionID == "" || !node.PublishVersion || node.Aggregate.ElementTarget.Revision != expectedNextRevision {
				return fmt.Errorf("sampled node %d merge requires current revision and version authority", index)
			}
			if node.ExpectedCurrentVersionID == node.Aggregate.Current.ID {
				return fmt.Errorf("sampled node %d cannot publish the expected current version again", index)
			}
			if node.Aggregate.Current.VersionNumber < 2 {
				return fmt.Errorf("sampled node %d merge must publish version 2 or later", index)
			}
		case "REUSE":
			if node.ExpectedRevision == 0 || node.PublishVersion || node.ExpectedCurrentVersionID == "" || node.Aggregate.ElementTarget.CurrentVersionID != node.ExpectedCurrentVersionID || node.Aggregate.ElementTarget.Revision != node.ExpectedRevision || !containsReferenceableElementTargetVersion(node.Aggregate, node.Aggregate.Current.ID) {
				return fmt.Errorf("sampled node %d reuse must keep current aggregate authority and select a referenceable version", index)
			}
		}
		if _, ok := formalNodes[node.Aggregate.ElementTarget.ID]; ok {
			return fmt.Errorf("duplicate formal sampled node at %d", index)
		}
		formalNodes[node.Aggregate.ElementTarget.ID] = struct{}{}
		if _, ok := formalVersions[node.Aggregate.Current.ID]; ok {
			return fmt.Errorf("duplicate formal sampled node version at %d", index)
		}
		formalVersions[node.Aggregate.Current.ID] = struct{}{}
		decisions[node.Aggregate.ElementTarget.ID+"\x00"+node.Aggregate.Current.ID] = struct{}{}
	}
	var validateReferences func([]FlowFragmentStep) error
	validateReferences = func(steps []FlowFragmentStep) error {
		for _, step := range steps {
			if step.ElementTargetID != "" {
				if _, exists := decisions[step.ElementTargetID+"\x00"+step.ElementTargetVersionID]; !exists {
					// The step display name is author-written text and the two ids are
					// caller data. The recursive walk carries no flat index, so the
					// message stays positionless rather than echoing any of the three.
					return errors.New("a sampled workflow step has no matching node decision")
				}
			}
			if err := validateReferences(step.Children); err != nil {
				return err
			}
			if step.ValidationGroup != nil {
				for _, branch := range step.ValidationGroup.Branches {
					if err := validateReferences(branch.Steps); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := validateReferences(p.FlowFragment.Current.Definition.Steps); err != nil {
		return err
	}
	return nil
}
