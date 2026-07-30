package automation

import (
	"errors"
	"fmt"
	"strings"
)

type SamplingNodePublication struct {
	TemporaryNodeID          string
	ResolutionMode           string
	Aggregate                NodeAggregate
	ExpectedRevision         Revision
	ExpectedCurrentVersionID string
	PublishVersion           bool
}

type SamplingPublication struct {
	Nodes        []SamplingNodePublication
	FlowFragment FlowFragmentAggregate
}

type SamplingNodeMapping struct {
	TemporaryNodeID string
	NodeID          string
	NodeVersionID   string
	ResolutionMode  string
}

type SamplingPublicationResult struct {
	FlowFragmentID    string
	WorkflowVersionID string
	VersionNumber     int
	Nodes             []SamplingNodeMapping
}

func (p SamplingPublication) Clone() SamplingPublication {
	cloned := SamplingPublication{FlowFragment: cloneWorkflowAggregate(p.FlowFragment)}
	cloned.Nodes = make([]SamplingNodePublication, len(p.Nodes))
	for index, node := range p.Nodes {
		cloned.Nodes[index] = node
		cloned.Nodes[index].Aggregate = cloneNodeAggregate(node.Aggregate)
	}
	return cloned
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
		if strings.TrimSpace(node.TemporaryNodeID) == "" {
			return errors.New("sampled node temporary id is required")
		}
		switch node.ResolutionMode {
		case "CREATE", "FORCE_CREATE", "MERGE", "REUSE":
		default:
			return fmt.Errorf("sampled node %s has unsupported resolution mode %q", node.TemporaryNodeID, node.ResolutionMode)
		}
		if _, ok := seen[node.TemporaryNodeID]; ok {
			return fmt.Errorf("duplicate sampled node %q", node.TemporaryNodeID)
		}
		seen[node.TemporaryNodeID] = struct{}{}
		if err := node.Aggregate.Validate(); err != nil {
			return fmt.Errorf("sampled node %s: %w", node.TemporaryNodeID, err)
		}
		switch node.ResolutionMode {
		case "CREATE", "FORCE_CREATE":
			if node.ExpectedRevision != 0 || node.ExpectedCurrentVersionID != "" || !node.PublishVersion || node.Aggregate.Current.VersionNumber != 1 {
				return fmt.Errorf("sampled node %s new ownership must publish version 1 without current-node authority", node.TemporaryNodeID)
			}
		case "MERGE":
			expectedNextRevision, err := node.ExpectedRevision.Next()
			if err != nil {
				return fmt.Errorf("sampled node %s merge revision: %w", node.TemporaryNodeID, err)
			}
			if node.ExpectedRevision == 0 || node.ExpectedCurrentVersionID == "" || !node.PublishVersion || node.Aggregate.Node.Revision != expectedNextRevision {
				return fmt.Errorf("sampled node %s merge requires current revision and version authority", node.TemporaryNodeID)
			}
			if node.ExpectedCurrentVersionID == node.Aggregate.Current.ID {
				return fmt.Errorf("sampled node %s cannot publish the expected current version again", node.TemporaryNodeID)
			}
			if node.Aggregate.Current.VersionNumber < 2 {
				return fmt.Errorf("sampled node %s merge must publish version 2 or later", node.TemporaryNodeID)
			}
		case "REUSE":
			if node.ExpectedRevision == 0 || node.PublishVersion || node.ExpectedCurrentVersionID == "" || node.ExpectedCurrentVersionID != node.Aggregate.Current.ID || node.Aggregate.Node.Revision != node.ExpectedRevision {
				return fmt.Errorf("sampled node %s reuse must keep the expected current version and revision", node.TemporaryNodeID)
			}
		}
		if _, ok := formalNodes[node.Aggregate.Node.ID]; ok {
			return fmt.Errorf("duplicate formal sampled node %q", node.Aggregate.Node.ID)
		}
		formalNodes[node.Aggregate.Node.ID] = struct{}{}
		if _, ok := formalVersions[node.Aggregate.Current.ID]; ok {
			return fmt.Errorf("duplicate formal sampled node version %q", node.Aggregate.Current.ID)
		}
		formalVersions[node.Aggregate.Current.ID] = struct{}{}
		decisions[node.Aggregate.Node.ID+"\x00"+node.Aggregate.Current.ID] = struct{}{}
	}
	var validateReferences func([]FlowFragmentStep) error
	validateReferences = func(steps []FlowFragmentStep) error {
		for _, step := range steps {
			if step.NodeID != "" {
				if _, exists := decisions[step.NodeID+"\x00"+step.NodeVersionID]; !exists {
					return fmt.Errorf("sampled workflow step %q has no matching node decision for %s/%s",
						step.DisplayName, step.NodeID, step.NodeVersionID)
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
