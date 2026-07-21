package execution

import "github.com/Capsule7446/healix-core/domain/fingerprint"

type StepKind string

const (
	ActionStep          StepKind = "ACTION"
	WaitStep            StepKind = "WAIT"
	RepeatStep          StepKind = "REPEAT"
	WorkflowReference   StepKind = "WORKFLOW_REF"
	ValidationStep      StepKind = "VALIDATION"
	ValidationGroupStep StepKind = "VALIDATION_GROUP"
)

type Step struct {
	ID                string
	DisplayName       string
	Kind              StepKind
	CaptureScreenshot bool
	Action            string
	NodeID            string
	NodeVersionID     string
	Value             string
	Values            []string
	WaitKind          string
	WaitMS            int
	RepeatCount       int
	Optional          bool
	Children          []Step
	Reference         *Reference
	Validation        *Validation
	ValidationGroup   *ValidationGroup
}

type Reference struct {
	WorkflowID        string
	WorkflowVersionID string
	ParameterBindings map[string]string
}

type Validation struct {
	Kind           string
	Expected       string
	ExpectedValues []string
	Attribute      string
	IgnoreCase     bool
	MaxWaitMS      int
	StabilityMS    int
}

type ValidationGroup struct {
	Branches    []ValidationBranch
	MaxWaitMS   int
	StabilityMS int
}

type ValidationBranch struct {
	ID    string
	Name  string
	Steps []Step
}

type WorkflowSnapshot struct {
	ID            string
	VersionID     string
	WorkflowID    string
	DisplayName   string
	VersionNumber int
	Steps         []Step
}

type NodeSnapshot struct {
	NodeID      string
	VersionID   string
	DisplayName string
	PageURL     string
	Origin      string
	Selectors   []fingerprint.Selector
	Fingerprint fingerprint.Fingerprint
}

type ReferenceResolution struct {
	ParentVersionID   string
	StepID            string
	WorkflowID        string
	WorkflowVersionID string
}

type Plan struct {
	RunID         string
	RootVersionID string
	Workflows     []WorkflowSnapshot
	Nodes         []NodeSnapshot
	References    []ReferenceResolution
}
