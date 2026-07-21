package workspace

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type Recording struct {
	ID          string
	ExecutionID string
	StorageURI  string
	Checksum    string
	Bytes       int64
	EventCount  int
	CreatedAt   int64
}

type HealObservation struct {
	ID                string
	RunID             string
	ExecutionID       string
	StepExecutionID   string
	NodeID            string
	BaseNodeVersionID string
	CandidateHash     string
	Selector          fingerprint.Selector
	Fingerprint       fingerprint.Fingerprint
	Confidence        float64
	DecisionBand      HealDecisionBand
	Succeeded         bool
	ObservedAt        int64
}

func (observation HealObservation) Validate() error {
	if strings.TrimSpace(observation.ID) == "" || strings.TrimSpace(observation.RunID) == "" ||
		strings.TrimSpace(observation.ExecutionID) == "" || strings.TrimSpace(observation.StepExecutionID) == "" ||
		strings.TrimSpace(observation.NodeID) == "" || strings.TrimSpace(observation.BaseNodeVersionID) == "" {
		return errors.New("heal observation is missing a required identity")
	}
	if observation.ObservedAt <= 0 {
		return errors.New("heal observation observed time must be positive")
	}
	if err := ValidateHealConfidence(observation.Confidence); err != nil {
		return err
	}
	return ValidateHealDecisionBand(observation.CandidateHash, observation.DecisionBand)
}

// ValidationObservation 是验证轮询中保留的一个更改点。它属于执行证据生命周期，而不属于 WorkflowVersion。当断言输入敏感时，执行适配器必须已对实际和预期进行了编辑。
type ValidationObservation struct {
	ID                    string
	RunID                 string
	ExecutionID           string
	StepExecutionID       string
	ValidationStepID      string
	ValidationGroupStepID string
	ValidationBranchID    string
	NodeID                string
	NodeVersionID         string
	AssertionKind         string
	Expected              string
	Actual                string
	Passed                bool
	Reason                string
	Selector              fingerprint.Selector
	Healed                bool
	HealConfidence        float64
	HealReviewStatus      string
	ObservedAt            int64
	Final                 bool
}

func (observation ValidationObservation) Validate() error {
	if strings.TrimSpace(observation.ID) == "" || strings.TrimSpace(observation.RunID) == "" ||
		strings.TrimSpace(observation.ExecutionID) == "" || strings.TrimSpace(observation.StepExecutionID) == "" ||
		strings.TrimSpace(observation.ValidationStepID) == "" || strings.TrimSpace(observation.NodeID) == "" ||
		strings.TrimSpace(observation.NodeVersionID) == "" || strings.TrimSpace(observation.AssertionKind) == "" ||
		strings.TrimSpace(observation.Reason) == "" {
		return errors.New("validation observation is missing a required field")
	}
	if observation.ObservedAt <= 0 {
		return errors.New("validation observation observed time must be positive")
	}
	if err := ValidateHealConfidence(observation.HealConfidence); err != nil {
		return fmt.Errorf("validation observation: %w", err)
	}
	switch observation.HealReviewStatus {
	case "not_attempted", "auto_applied", "review_pending", "no_candidate":
		return nil
	default:
		return fmt.Errorf("validation observation has unsupported heal review status %q", observation.HealReviewStatus)
	}
}

type HealCandidateEvidence struct {
	ObservationID   string
	CandidateHash   string
	FingerprintHash string
	Score           float64
	Rank            int
	Eligible        bool
	Selected        bool
	Status          string
	ObservedAt      int64
}

func (e HealCandidateEvidence) Validate() error {
	if strings.TrimSpace(e.ObservationID) == "" || strings.TrimSpace(e.CandidateHash) == "" || strings.TrimSpace(e.FingerprintHash) == "" || e.Rank <= 0 || e.ObservedAt <= 0 {
		return errors.New("heal candidate evidence is missing identity, rank, or time")
	}
	if err := ValidateHealConfidence(e.Score); err != nil {
		return err
	}
	return nil
}

// HealObservationDetail enriches immutable healing facts for replay and review.
type HealObservationDetail struct {
	Observation       HealObservation
	Candidates        []HealCandidateEvidence
	OldSelectors      []fingerprint.Selector
	CountedInStreak   bool
	StreakCount       int
	PromotedVersionID string
}

type HealCandidateRecord struct {
	NodeID                string
	NodeDisplayName       string
	BaseNodeVersionID     string
	BaseVersionNumber     int
	CandidateHash         string
	Selector              fingerprint.Selector
	Fingerprint           fingerprint.Fingerprint
	Streak                int
	DecisionBand          HealDecisionBand
	Status                HealCandidateStatus
	ApprovalStatus        HealApprovalStatus
	ReviewedBy            string
	ReviewedAt            int64
	PromotedVersionID     string
	LastObservedAt        int64
	FirstObservedAt       int64
	LatestRunID           string
	LatestExecutionID     string
	LatestStepExecutionID string
	ObservationCount      int
}

type HTTPRequestEvidence struct {
	Domain        string
	URL           string
	Method        string
	Query         string
	Headers       map[string]string
	BodyMIME      string
	BodyURI       string
	BodyChecksum  string
	BodyBytes     int64
	BodyOmitted   string
	UploadFields  []UploadFieldEvidence
	RedirectChain []string
}

type UploadFieldEvidence struct {
	Name        string
	FileName    string
	ContentType string
	Bytes       int64
	Checksum    string
}

type HTTPResponseEvidence struct {
	Status            int
	StatusText        string
	Headers           map[string]string
	MIME              string
	FileName          string
	BodyURI           string
	BodyChecksum      string
	BodyBytes         int64
	BodyOmitted       string
	FailureReason     string
	FromDiskCache     bool
	FromServiceWorker bool
}

type NetworkEvidence struct {
	ID              string
	ExecutionID     string
	StepExecutionID string
	Request         HTTPRequestEvidence
	Response        HTTPResponseEvidence
	StartedAt       int64
	FinishedAt      int64
	DurationMS      int64
	Bytes           int64
}

type WorkflowExecutionRecord struct {
	Plan           WorkflowExecutionPlan
	RunID          string
	Status         ExecutionStatus
	SucceededSteps int
	FailedSteps    int
	StartedAt      int64
	FinishedAt     int64
	NetworkCount   int
	HealCount      int
	HasRecording   bool
}

type StepExecutionRecord struct {
	ID             string
	ExecutionID    string
	WorkflowStepID string
	DisplayName    string
	Kind           string
	Phase          string
	Occurrence     int
	HierarchyPath  string
	StartedAt      int64
	FinishedAt     int64
	ErrorMessage   string
	NetworkCount   int
	Healed         bool
}

type TestTaskRunDetail struct {
	Plan       TestTaskRunPlan
	Executions []WorkflowExecutionRecord
}

type ExecutionDetail struct {
	RunID       string
	Execution   WorkflowExecutionRecord
	Environment EnvironmentSnapshot
	Workflows   []WorkflowDependencySnapshot
	Nodes       []NodeDependencySnapshot
	Steps       []StepExecutionRecord
	Recording   *Recording
	Requests    []NetworkEvidence
	Heals       []HealObservationDetail
	Validations []ValidationObservation
}

type TestTaskRunPlan struct {
	Run         TestTaskRun
	Task        TestTask
	Version     TestTaskVersion
	Environment EnvironmentSnapshot
	Workflows   []WorkflowDependencySnapshot
	Nodes       []NodeDependencySnapshot
	References  []WorkflowReferenceResolution
	Parameters  []WorkflowParameterScope
	Executions  []WorkflowExecutionPlan
}
