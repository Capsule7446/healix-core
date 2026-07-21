package automation

import (
	"errors"
	"fmt"
	"strings"
)

type SamplingNodePublication struct {
	TemporaryNodeID          string
	Aggregate                NodeAggregate
	ExpectedCurrentVersionID string
	PublishVersion           bool
}

type SamplingPublication struct {
	Nodes    []SamplingNodePublication
	Workflow WorkflowAggregate
}

type SamplingNodeMapping struct {
	TemporaryNodeID string
	NodeID          string
	NodeVersionID   string
}

type SamplingPublicationResult struct {
	WorkflowID        string
	WorkflowVersionID string
	VersionNumber     int
	Nodes             []SamplingNodeMapping
}

func (p SamplingPublication) Validate() error {
	if err := p.Workflow.Validate(); err != nil {
		return fmt.Errorf("sampled workflow: %w", err)
	}
	seen := make(map[string]struct{}, len(p.Nodes))
	decisions := make(map[string]struct{}, len(p.Nodes))
	for _, node := range p.Nodes {
		if strings.TrimSpace(node.TemporaryNodeID) == "" {
			return errors.New("sampled node temporary id is required")
		}
		if _, ok := seen[node.TemporaryNodeID]; ok {
			return fmt.Errorf("duplicate sampled node %q", node.TemporaryNodeID)
		}
		seen[node.TemporaryNodeID] = struct{}{}
		if err := node.Aggregate.Validate(); err != nil {
			return fmt.Errorf("sampled node %s: %w", node.TemporaryNodeID, err)
		}
		switch {
		case node.ExpectedCurrentVersionID == "":
			if !node.PublishVersion || node.Aggregate.Current.VersionNumber != 1 {
				return fmt.Errorf("sampled node %s new ownership must publish version 1", node.TemporaryNodeID)
			}
		case !node.PublishVersion:
			if node.ExpectedCurrentVersionID != node.Aggregate.Current.ID {
				return fmt.Errorf("sampled node %s reuse must keep the expected current version", node.TemporaryNodeID)
			}
		case node.ExpectedCurrentVersionID == node.Aggregate.Current.ID:
			return fmt.Errorf("sampled node %s cannot publish the expected current version again", node.TemporaryNodeID)
		case node.Aggregate.Current.VersionNumber < 2:
			return fmt.Errorf("sampled node %s merge must publish version 2 or later", node.TemporaryNodeID)
		}
		decisions[node.Aggregate.Node.ID+"\x00"+node.Aggregate.Current.ID] = struct{}{}
	}
	var validateReferences func([]WorkflowStep) error
	validateReferences = func(steps []WorkflowStep) error {
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
	if err := validateReferences(p.Workflow.Current.Definition.Steps); err != nil {
		return err
	}
	return nil
}
