package workspace

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type SamplingBrowserStatus string

const (
	SamplingBrowserClosed SamplingBrowserStatus = "CLOSED"
	SamplingBrowserOpen   SamplingBrowserStatus = "OPEN"
)

type SamplingCaptureStatus string

const (
	SamplingCaptureIdle        SamplingCaptureStatus = "IDLE"
	SamplingCaptureRecording   SamplingCaptureStatus = "RECORDING"
	SamplingCapturePaused      SamplingCaptureStatus = "PAUSED"
	SamplingCaptureEnded       SamplingCaptureStatus = "ENDED"
	SamplingCaptureInterrupted SamplingCaptureStatus = "INTERRUPTED"
)

// SamplingLifecycle is owned by each temporary workflow.  CaptureStatus is a
// workspace projection for controls, while this value keeps ended/interrupted
// drafts distinguishable after another browser session starts.
type SamplingLifecycle string

const (
	SamplingLifecycleRecording   SamplingLifecycle = "RECORDING"
	SamplingLifecyclePaused      SamplingLifecycle = "PAUSED"
	SamplingLifecycleEnded       SamplingLifecycle = "ENDED"
	SamplingLifecycleInterrupted SamplingLifecycle = "INTERRUPTED"
)

type SamplingWorkflowStatus string

const (
	SamplingWorkflowUnsaved SamplingWorkflowStatus = "UNSAVED"
	SamplingWorkflowSaving  SamplingWorkflowStatus = "SAVING"
	SamplingWorkflowSaved   SamplingWorkflowStatus = "SAVED"
	SamplingWorkflowFailed  SamplingWorkflowStatus = "FAILED"
)

type SamplingResolutionMode string

const (
	SamplingResolutionUndecided   SamplingResolutionMode = "UNDECIDED"
	SamplingResolutionCreate      SamplingResolutionMode = "CREATE"
	SamplingResolutionMerge       SamplingResolutionMode = "MERGE"
	SamplingResolutionReuse       SamplingResolutionMode = "REUSE"
	SamplingResolutionForceCreate SamplingResolutionMode = "FORCE_CREATE"
)

type SamplingCandidate struct {
	NodeID          string
	DisplayName     string
	VersionID       string
	VersionNumber   int
	Similarity      float64
	SelectorOverlap int
	Exact           bool
}

type TemporarySamplingNode struct {
	ID             string
	DisplayName    string
	Properties     Properties
	PageURL        string
	Origin         string
	Selectors      []fingerprint.Selector
	Fingerprint    fingerprint.Fingerprint
	StepIDs        []string
	ResolutionMode SamplingResolutionMode
	ExistingNodeID string
	Candidates     []SamplingCandidate
}

type TemporarySamplingWorkflow struct {
	ID            string
	SessionID     string
	DisplayName   string
	Properties    Properties
	StartedAt     int64
	PausedAt      int64
	EndedAt       int64
	InterruptedAt int64
	Lifecycle     SamplingLifecycle
	// Validation insertion is an optional paused-editor choice.  It affects
	// only the next validation capture; ordinary actions always remain root
	// steps and an invalid/deleted branch is cleared by the application layer.
	ValidationInsertGroupID     string
	ValidationInsertBranchID    string
	ValidationCapturedActionIDs []string
	Status                      SamplingWorkflowStatus
	ErrorMessage                string
	Steps                       []WorkflowStep
	Parameters                  []ParameterDefinition
	Nodes                       []TemporarySamplingNode
	SavedWorkflowID             string
	SavedVersionID              string
	SavedVersionNumber          int
}

// RebuildTemporaryNodeReferences derives the temporary Node -> Step projection
// from the editable workflow tree. Temporary sampling data is intentionally
// in-memory only, therefore this is the single source of truth after any
// capture, edit, delete, or reorder operation.
func RebuildTemporaryNodeReferences(workflow *TemporarySamplingWorkflow) error {
	if workflow == nil {
		return errors.New("temporary sampling workflow is required")
	}
	nodes := make(map[string]*TemporarySamplingNode, len(workflow.Nodes))
	for index := range workflow.Nodes {
		workflow.Nodes[index].StepIDs = nil
		nodes[workflow.Nodes[index].ID] = &workflow.Nodes[index]
	}
	var walk func([]WorkflowStep) error
	walk = func(steps []WorkflowStep) error {
		for _, step := range steps {
			if step.NodeID != "" {
				node, ok := nodes[step.NodeID]
				if !ok {
					return fmt.Errorf("sampling step %s references unknown temporary node %s", step.ID, step.NodeID)
				}
				node.StepIDs = append(node.StepIDs, step.ID)
			}
			if err := walk(step.Children); err != nil {
				return err
			}
			if step.ValidationGroup != nil {
				for _, branch := range step.ValidationGroup.Branches {
					if err := walk(branch.Steps); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	return walk(workflow.Steps)
}

type SamplingWorkspace struct {
	BrowserStatus     SamplingBrowserStatus
	CaptureStatus     SamplingCaptureStatus
	ValidationArmed   bool
	BrowserSessionID  string
	CurrentWorkflowID string
	Workflows         []TemporarySamplingWorkflow
}

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
