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

func (p SamplingPublication) Validate() error {
	if err := p.FlowFragment.Validate(); err != nil {
		return fmt.Errorf("sampled workflow: %w", err)
	}
	seen := make(map[string]struct{}, len(p.Nodes))
	formalNodes := make(map[string]struct{}, len(p.Nodes))
	formalVersions := make(map[string]struct{}, len(p.Nodes))
	decisions := make(map[string]struct{}, len(p.Nodes))
	for _, node := range p.Nodes {
		if strings.TrimSpace(node.TemporaryElementTargetID) == "" {
			return errors.New("sampled node temporary id is required")
		}
		switch node.ResolutionMode {
		case "CREATE", "MERGE", "REUSE":
		default:
			return fmt.Errorf("sampled node %s has unsupported resolution mode %q", node.TemporaryElementTargetID, node.ResolutionMode)
		}
		if _, ok := seen[node.TemporaryElementTargetID]; ok {
			return fmt.Errorf("duplicate sampled node %q", node.TemporaryElementTargetID)
		}
		seen[node.TemporaryElementTargetID] = struct{}{}
		if node.ResolutionMode != "REUSE" {
			if err := node.Aggregate.Validate(); err != nil {
				return fmt.Errorf("sampled node %s: %w", node.TemporaryElementTargetID, err)
			}
		}
		if strings.TrimSpace(node.Aggregate.ElementTarget.ID) == "" || node.Aggregate.Current.ElementTargetID != node.Aggregate.ElementTarget.ID || node.Aggregate.Current.DeletedAt != 0 {
			return fmt.Errorf("sampled node %s selected version is not referenceable", node.TemporaryElementTargetID)
		}
		if node.ResolutionMode != "REUSE" && !containsReferenceableElementTargetVersion(node.Aggregate, node.Aggregate.Current.ID) {
			return fmt.Errorf("sampled node %s selected version is not referenceable", node.TemporaryElementTargetID)
		}
		switch node.ResolutionMode {
		case "CREATE":
			if node.ExpectedRevision != 0 || node.ExpectedCurrentVersionID != "" || !node.PublishVersion || node.Aggregate.Current.VersionNumber != 1 {
				return fmt.Errorf("sampled node %s new ownership must publish version 1 without current-node authority", node.TemporaryElementTargetID)
			}
		case "MERGE":
			expectedNextRevision, err := node.ExpectedRevision.Next()
			if err != nil {
				return fmt.Errorf("sampled node %s merge revision: %w", node.TemporaryElementTargetID, err)
			}
			if node.ExpectedRevision == 0 || node.ExpectedCurrentVersionID == "" || !node.PublishVersion || node.Aggregate.ElementTarget.Revision != expectedNextRevision {
				return fmt.Errorf("sampled node %s merge requires current revision and version authority", node.TemporaryElementTargetID)
			}
			if node.ExpectedCurrentVersionID == node.Aggregate.Current.ID {
				return fmt.Errorf("sampled node %s cannot publish the expected current version again", node.TemporaryElementTargetID)
			}
			if node.Aggregate.Current.VersionNumber < 2 {
				return fmt.Errorf("sampled node %s merge must publish version 2 or later", node.TemporaryElementTargetID)
			}
		case "REUSE":
			if node.ExpectedRevision == 0 || node.PublishVersion || node.ExpectedCurrentVersionID == "" || node.Aggregate.ElementTarget.CurrentVersionID != node.ExpectedCurrentVersionID || node.Aggregate.ElementTarget.Revision != node.ExpectedRevision || !containsReferenceableElementTargetVersion(node.Aggregate, node.Aggregate.Current.ID) {
				return fmt.Errorf("sampled node %s reuse must keep current aggregate authority and select a referenceable version", node.TemporaryElementTargetID)
			}
		}
		if _, ok := formalNodes[node.Aggregate.ElementTarget.ID]; ok {
			return fmt.Errorf("duplicate formal sampled node %q", node.Aggregate.ElementTarget.ID)
		}
		formalNodes[node.Aggregate.ElementTarget.ID] = struct{}{}
		if _, ok := formalVersions[node.Aggregate.Current.ID]; ok {
			return fmt.Errorf("duplicate formal sampled node version %q", node.Aggregate.Current.ID)
		}
		formalVersions[node.Aggregate.Current.ID] = struct{}{}
		decisions[node.Aggregate.ElementTarget.ID+"\x00"+node.Aggregate.Current.ID] = struct{}{}
	}
	var validateReferences func([]FlowFragmentStep) error
	validateReferences = func(steps []FlowFragmentStep) error {
		for _, step := range steps {
			if step.ElementTargetID != "" {
				if _, exists := decisions[step.ElementTargetID+"\x00"+step.ElementTargetVersionID]; !exists {
					return fmt.Errorf("sampled workflow step %q has no matching node decision for %s/%s",
						step.DisplayName, step.ElementTargetID, step.ElementTargetVersionID)
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
