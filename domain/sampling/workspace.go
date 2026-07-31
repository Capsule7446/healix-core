package sampling

import (
	"errors"
	"fmt"

	"github.com/Capsule7446/healix-core/domain/automation"
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

// SamplingLifecycle 属于每个临时工作流程。  CaptureStatus 是控件的工作区投影，而此值在另一个浏览器会话启动后可以区分已结束/中断的草稿。
type SamplingLifecycle string

const (
	SamplingLifecycleRecording   SamplingLifecycle = "RECORDING"
	SamplingLifecyclePaused      SamplingLifecycle = "PAUSED"
	SamplingLifecycleEnded       SamplingLifecycle = "ENDED"
	SamplingLifecycleInterrupted SamplingLifecycle = "INTERRUPTED"
)

type PublicationStatus string

const (
	PublicationStatusUnpublished PublicationStatus = "UNSAVED"
	PublicationStatusPublishing  PublicationStatus = "SAVING"
	PublicationStatusPublished   PublicationStatus = "SAVED"
	PublicationStatusFailed      PublicationStatus = "FAILED"
)

type ResolutionMode string

const (
	ResolutionModeUndecided ResolutionMode = "UNDECIDED"
	ResolutionModeCreate    ResolutionMode = "CREATE"
	ResolutionModeMerge     ResolutionMode = "MERGE"
	ResolutionModeReuse     ResolutionMode = "REUSE"
)

type ElementTargetCandidate struct {
	ElementTargetID string
	DisplayName     string
	VersionID       string
	VersionNumber   int
	Similarity      float64
	SelectorOverlap int
	Exact           bool
}

type UnpublishedElementTarget struct {
	ID             string
	DisplayName    string
	Properties     automation.Properties
	PageURL        string
	Origin         string
	Selectors      []fingerprint.Selector
	Fingerprint    fingerprint.Fingerprint
	StepIDs        []string
	ResolutionMode ResolutionMode
	ExistingNodeID string
	Candidates     []ElementTargetCandidate
}

type UnpublishedFlowFragment struct {
	ID            string
	SessionID     string
	DisplayName   string
	Properties    automation.Properties
	StartedAt     int64
	PausedAt      int64
	EndedAt       int64
	InterruptedAt int64
	Lifecycle     SamplingLifecycle
	// 验证插入是一个可选的暂停编辑器选择。  它只影响下一次验证捕获；普通操作始终保留根步骤，并且应用程序层清除无效/已删除的分支。
	ValidationInsertGroupID     string
	ValidationInsertBranchID    string
	ValidationCapturedActionIDs []string
	Status                      PublicationStatus
	ErrorMessage                string
	Steps                       []automation.FlowFragmentStep
	Parameters                  []automation.ParameterDefinition
	Nodes                       []UnpublishedElementTarget
	SavedWorkflowID             string
	SavedVersionID              string
	SavedVersionNumber          int
}

// RebuildUnpublishedElementTargetReferences 从可编辑工作流树中派生临时 ElementTarget -> Step 投影。临时采样数据有意仅存储在内存中，因此这是任何捕获、编辑、删除或重新排序操作后的唯一事实来源。
func RebuildUnpublishedElementTargetReferences(workflow *UnpublishedFlowFragment) error {
	if workflow == nil {
		return errors.New("temporary sampling workflow is required")
	}
	stepIDsByNode := make(map[string][]string, len(workflow.Nodes))
	for _, node := range workflow.Nodes {
		stepIDsByNode[node.ID] = nil
	}
	var walk func([]automation.FlowFragmentStep) error
	walk = func(steps []automation.FlowFragmentStep) error {
		for _, step := range steps {
			if step.ElementTargetID != "" {
				stepIDs, ok := stepIDsByNode[step.ElementTargetID]
				if !ok {
					return fmt.Errorf("sampling step %s references unknown temporary node %s", step.ID, step.ElementTargetID)
				}
				stepIDsByNode[step.ElementTargetID] = append(stepIDs, step.ID)
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
	if err := walk(workflow.Steps); err != nil {
		return err
	}
	for index := range workflow.Nodes {
		workflow.Nodes[index].StepIDs = stepIDsByNode[workflow.Nodes[index].ID]
	}
	return nil
}

type SamplingSessionState struct {
	BrowserStatus     SamplingBrowserStatus
	CaptureStatus     SamplingCaptureStatus
	ValidationArmed   bool
	BrowserSessionID  string
	CurrentWorkflowID string
	Workflows         []UnpublishedFlowFragment
}
