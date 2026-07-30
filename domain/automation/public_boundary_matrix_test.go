package automation

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestPrimitiveAssetValidatorsRejectBusinessBoundaries(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{name: "blank property key", run: func() error { return Properties{" \t": "value"}.Validate() }, want: "property key"},
		{name: "unknown version source", run: func() error { return VersionSource("UNKNOWN").Validate() }, want: "unsupported version source"},
		{name: "unknown workflow version policy", run: func() error { return FlowFragmentVersionPolicy("UNKNOWN").Validate() }, want: "unsupported workflow version policy"},
		{name: "non select options", run: func() error {
			return (ParameterDefinition{Name: "value", DisplayName: "Value", Type: parameter.Text, Required: true, Options: []string{"forbidden"}}).Validate()
		}, want: "cannot declare options"},
		{name: "invalid parameter value", run: func() error {
			return (ParameterDefinition{Type: parameter.Text}).ValidateValue(parameter.Value{})
		}, want: "unsupported parameter value type"},
		{name: "unknown multi select option", run: func() error {
			return (ParameterDefinition{Type: parameter.MultiSelect, Options: []string{"east"}}).ValidateValue(parameter.MultiSelectValue([]string{"west"}))
		}, want: "unknown option"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestNodeAggregateValidateSingleFactorRuleMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*NodeAggregate)
		want   string
	}{
		{name: "invalid properties", mutate: func(a *NodeAggregate) { a.Node.Properties = Properties{" ": "value"} }, want: "property key"},
		{name: "missing version id", mutate: func(a *NodeAggregate) { a.Current.ID, a.Node.CurrentVersionID = "", "" }, want: "version id"},
		{name: "version number below boundary", mutate: func(a *NodeAggregate) { a.Current.VersionNumber = 0 }, want: "version number"},
		{name: "invalid selector", mutate: func(a *NodeAggregate) { a.Current.Selectors[0].Type = fingerprint.SelectorType("UNKNOWN") }, want: "selector 1"},
		{name: "missing fingerprint tag", mutate: func(a *NodeAggregate) { a.Current.Fingerprint.Tag = " " }, want: "fingerprint tag"},
		{name: "nil fingerprint attributes", mutate: func(a *NodeAggregate) { a.Current.Fingerprint.Attributes = nil }, want: "fingerprint attributes"},
		{name: "unknown source", mutate: func(a *NodeAggregate) { a.Current.Source = "UNKNOWN" }, want: "unsupported version source"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := versionedNodeAggregate()
			test.mutate(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestEnvironmentValidateSingleFactorRuleMatrix(t *testing.T) {
	valid := Environment{ID: "env", DisplayName: "Environment", BaseURL: "https://example.test", Variables: EnvironmentVariables{}}
	tests := []struct {
		name   string
		mutate func(*Environment)
		want   string
	}{
		{name: "missing id", mutate: func(e *Environment) { e.ID = " " }, want: "environment id"},
		{name: "missing display name", mutate: func(e *Environment) { e.DisplayName = "\n" }, want: "display name"},
		{name: "relative base url", mutate: func(e *Environment) { e.BaseURL = "/relative" }, want: "absolute HTTP"},
		{name: "unsupported base url scheme", mutate: func(e *Environment) { e.BaseURL = "ftp://example.test" }, want: "HTTP or HTTPS"},
		{name: "invalid variables", mutate: func(e *Environment) {
			e.Variables = EnvironmentVariables{" ": parameter.TextValue("value")}
		}, want: "environment variable name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := valid
			test.mutate(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestWorkflowAggregateValidateSingleFactorRuleMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*FlowFragmentAggregate)
		want   string
	}{
		{name: "missing workflow id", mutate: func(a *FlowFragmentAggregate) { a.FlowFragment.ID = " " }, want: "workflow id"},
		{name: "missing display name", mutate: func(a *FlowFragmentAggregate) { a.FlowFragment.DisplayName = "\t" }, want: "display name"},
		{name: "invalid properties", mutate: func(a *FlowFragmentAggregate) { a.FlowFragment.Properties = Properties{" ": "value"} }, want: "property key"},
		{name: "version number below boundary", mutate: func(a *FlowFragmentAggregate) { a.Current.VersionNumber = 0 }, want: "version number"},
		{name: "missing step identity", mutate: func(a *FlowFragmentAggregate) { a.Current.Definition.Steps[0].ID = " " }, want: "step id and display name"},
		{name: "action carries validation", mutate: func(a *FlowFragmentAggregate) {
			a.Current.Definition.Steps[0] = FlowFragmentStep{ID: "action", DisplayName: "Action", Kind: StepAction, Action: "navigate", Value: "https://example.test", Validation: &ValidationConfig{}}
		}, want: "ACTION cannot carry validation"},
		{name: "wait carries validation", mutate: func(a *FlowFragmentAggregate) { a.Current.Definition.Steps[0].Validation = &ValidationConfig{} }, want: "WAIT cannot carry validation"},
		{name: "repeat carries validation", mutate: func(a *FlowFragmentAggregate) {
			a.Current.Definition.Steps[0] = FlowFragmentStep{ID: "repeat", DisplayName: "Repeat", Kind: StepRepeat, RepeatCount: 1, Validation: &ValidationConfig{}, Children: []FlowFragmentStep{{ID: "child", DisplayName: "Child", Kind: StepAction, Action: "navigate", Value: "https://example.test"}}}
		}, want: "REPEAT cannot carry validation"},
		{name: "reference carries validation", mutate: func(a *FlowFragmentAggregate) {
			a.Current.Definition.Steps[0] = FlowFragmentStep{ID: "reference", DisplayName: "Reference", Kind: StepFlowFragmentRef, Reference: &FlowFragmentReference{FlowFragmentID: "child", LatestPublished: true}, Validation: &ValidationConfig{}}
		}, want: "WORKFLOW_REF cannot carry validation"},
		{name: "nested validation group", mutate: func(a *FlowFragmentAggregate) {
			a.Current.Definition.Steps[0] = FlowFragmentStep{ID: "repeat", DisplayName: "Repeat", Kind: StepRepeat, RepeatCount: 1, Children: []FlowFragmentStep{{ID: "group", DisplayName: "Group", Kind: StepValidationGroup}}}
		}, want: "must be a root step"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := versionedWorkflowAggregate()
			test.mutate(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestHealPublicRulesAndErrorBoundaries(t *testing.T) {
	if _, err := NextVersionNumber([]VersionMeta{{ID: "max", VersionNumber: math.MaxInt}}); !errors.Is(err, ErrVersionNumberOverflow) {
		t.Fatalf("NextVersionNumber() error = %v", err)
	}
	if err := ValidateHealDecisionBand("", HealDecisionBandApplied); err == nil {
		t.Fatal("candidate-free applied band accepted")
	}
	for _, confidence := range []float64{0, 0.5, 1} {
		if err := ValidateHealConfidence(confidence); err != nil {
			t.Fatalf("ValidateHealConfidence(%v) error = %v", confidence, err)
		}
	}

	validCandidate := HealCandidate{Hash: "candidate", NodeID: "node", BaseNodeVersionID: "base", Status: HealCandidateAwaitingApproval, Revision: 1}
	invalidCandidate := validCandidate
	invalidCandidate.Hash = " "
	if err := invalidCandidate.ValidateReviewed(); err == nil {
		t.Fatal("ValidateReviewed accepted invalid identity")
	}
	if _, err := invalidCandidate.Review(HealCandidatePromoted); err == nil {
		t.Fatal("Review accepted invalid candidate")
	}

	validCommand := HealCandidateReviewCommand{CommandID: "command", NodeID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", ExpectedCandidateRevision: 1, ExpectedNodeRevision: 1}
	commands := []struct {
		name     string
		command  HealCandidateReviewCommand
		approval HealApprovalStatus
	}{
		{name: "missing identity", command: func() HealCandidateReviewCommand { value := validCommand; value.CommandID = " "; return value }(), approval: HealApprovalApproved},
		{name: "missing candidate revision", command: func() HealCandidateReviewCommand {
			value := validCommand
			value.ExpectedCandidateRevision = 0
			return value
		}(), approval: HealApprovalApproved},
		{name: "unsupported approval", command: validCommand, approval: HealApprovalPending},
	}
	for _, test := range commands {
		t.Run(test.name, func(t *testing.T) {
			if err := test.command.Validate(test.approval); err == nil {
				t.Fatal("invalid review command accepted")
			}
		})
	}
}

func TestHealStreakObserveRejectsInvalidInputsAndStateBoundaries(t *testing.T) {
	valid := acceptedHealObservation("run-2", 2, "candidate", HealDecisionBandApplied)
	invalidStreak := HealStreak{Observing: true, Disposition: HealStreakObserving}
	if _, err := invalidStreak.Observe(valid); err == nil {
		t.Fatal("invalid persisted streak accepted")
	}
	invalidObservation := valid
	invalidObservation.FactID = " "
	if _, err := (HealStreak{}).Observe(invalidObservation); err == nil {
		t.Fatal("invalid observation accepted")
	}

	base := HealStreak{NodeID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandApplied, Contributions: contributions("run-1"), LastSequence: 1, Observing: true, Disposition: HealStreakObserving}
	ordering := base
	ordering.LastSequence = 5
	staleSequence := valid
	staleSequence.Sequence = 4
	staleSequence.FactID, staleSequence.CommitID, staleSequence.RunID = "fact-new", "commit-new", "run-new"
	if _, err := ordering.Observe(staleSequence); err == nil {
		t.Fatal("non-increasing observation sequence accepted")
	}

	staleBase := valid
	staleBase.BaseIsCurrent = false
	decision, err := base.Observe(staleBase)
	if err != nil || decision.Next.Disposition != HealStreakStale {
		t.Fatalf("stale current base decision = %#v, %v", decision.Next, err)
	}
	newStale, err := (HealStreak{}).Observe(staleBase)
	if err != nil || newStale.Next.Disposition != HealStreakStale {
		t.Fatalf("new stale terminal = %#v, %v", newStale.Next, err)
	}
	unknown := valid
	unknown.CandidateHash = ""
	unknown.Band = HealDecisionBandUnknown
	unchanged, err := base.Observe(unknown)
	if err != nil || len(unchanged.Next.Contributions) != len(base.Contributions) {
		t.Fatalf("unknown-band observation = %#v, %v", unchanged.Next, err)
	}
}

func TestHealStreakRejectRejectsInvalidReceiverAndDisposition(t *testing.T) {
	if _, err := (HealStreak{Observing: true, Disposition: HealStreakObserving}).Reject(1); err == nil {
		t.Fatal("invalid streak accepted")
	}
	observing := HealStreak{NodeID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandApplied, Contributions: contributions("run-1"), LastSequence: 1, Observing: true, Disposition: HealStreakObserving}
	if _, err := observing.Reject(2); err == nil {
		t.Fatal("non-awaiting streak rejection accepted")
	}
}

func TestLifecyclePublicMethodsRejectSingleFactorFailures(t *testing.T) {
	validEnvironment := Environment{ID: "env", DisplayName: "Environment", BaseURL: "https://example.test", Variables: EnvironmentVariables{}, CreatedAt: 2, UpdatedAt: 2}
	if _, err := NewEnvironment(func() Environment { value := validEnvironment; value.UpdatedAt = 1; return value }()); err == nil {
		t.Fatal("NewEnvironment accepted unequal timestamps")
	}
	if _, err := NewEnvironment(func() Environment { value := validEnvironment; value.ID = " "; return value }()); err == nil {
		t.Fatal("NewEnvironment accepted invalid environment")
	}
	createdEnvironment, err := NewEnvironment(validEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	deletedEnvironment := createdEnvironment
	deletedEnvironment.DeletedAt = 2
	if _, err := deletedEnvironment.UpdateMetadata("Environment", "", EnvironmentVariables{}, 3); !errors.Is(err, ErrDeletedAggregate) {
		t.Fatalf("deleted environment update error = %v", err)
	}
	for _, test := range []struct {
		name    string
		value   Environment
		at      int64
		display string
	}{
		{name: "stale time", value: createdEnvironment, at: 1, display: "Environment"},
		{name: "revision overflow", value: func() Environment {
			value := createdEnvironment
			value.Revision = Revision(math.MaxUint64)
			return value
		}(), at: 3, display: "Environment"},
		{name: "invalid result", value: createdEnvironment, at: 3, display: " "},
	} {
		t.Run("environment "+test.name, func(t *testing.T) {
			if _, err := test.value.UpdateMetadata(test.display, "", EnvironmentVariables{}, test.at); err == nil {
				t.Fatal("invalid environment update accepted")
			}
		})
	}

	node := versionedNodeAggregate()
	if _, err := NewNode(func() Node { value := node.Node; value.UpdatedAt = 0; return value }(), node.Current); err == nil {
		t.Fatal("NewNode accepted invalid timestamps")
	}
	invalidNode := node.Node
	invalidNode.UpdatedAt = invalidNode.CreatedAt
	invalidVersion := node.Current
	invalidVersion.CreatedAt = invalidNode.CreatedAt
	invalidVersion.Fingerprint.Tag = ""
	if _, err := NewNode(invalidNode, invalidVersion); err == nil {
		t.Fatal("NewNode accepted invalid aggregate")
	}
	for _, test := range []struct {
		name  string
		value NodeAggregate
		label string
	}{
		{name: "revision overflow", value: func() NodeAggregate { value := node; value.Node.Revision = Revision(math.MaxUint64); return value }(), label: "Node"},
		{name: "invalid result", value: node, label: " "},
	} {
		t.Run("node "+test.name, func(t *testing.T) {
			if _, err := test.value.UpdateMetadata(test.label, "", Properties{}, 3); err == nil {
				t.Fatal("invalid node update accepted")
			}
		})
	}

	workflow := versionedWorkflowAggregate()
	if _, err := NewFlowFragment(func() FlowFragment { value := workflow.FlowFragment; value.UpdatedAt = 0; return value }(), workflow.Current); err == nil {
		t.Fatal("NewFlowFragment accepted invalid timestamps")
	}
	invalidWorkflow := workflow.FlowFragment
	invalidWorkflow.UpdatedAt = invalidWorkflow.CreatedAt
	invalidWorkflowVersion := workflow.Current
	invalidWorkflowVersion.CreatedAt = invalidWorkflow.CreatedAt
	invalidWorkflowVersion.Definition.Steps = nil
	if _, err := NewFlowFragment(invalidWorkflow, invalidWorkflowVersion); err == nil {
		t.Fatal("NewFlowFragment accepted invalid aggregate")
	}
	deletedWorkflow := workflow
	deletedWorkflow.FlowFragment.DeletedAt = 2
	if _, err := deletedWorkflow.UpdateMetadata("FlowFragment", "", Properties{}, 3); !errors.Is(err, ErrDeletedAggregate) {
		t.Fatalf("deleted workflow update error = %v", err)
	}
	for _, test := range []struct {
		name  string
		value FlowFragmentAggregate
		label string
	}{
		{name: "revision overflow", value: func() FlowFragmentAggregate {
			value := workflow
			value.FlowFragment.Revision = Revision(math.MaxUint64)
			return value
		}(), label: "FlowFragment"},
		{name: "invalid result", value: workflow, label: " "},
	} {
		t.Run("workflow "+test.name, func(t *testing.T) {
			if _, err := test.value.UpdateMetadata(test.label, "", Properties{}, 3); err == nil {
				t.Fatal("invalid workflow update accepted")
			}
		})
	}
}

func TestTestTaskLifecyclePublicFailureMatrix(t *testing.T) {
	plan := validTestTaskVersionPlan()
	created, err := NewExecutionFlow(plan.Task, plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	publication := ExecutionFlowVersionPublication{ID: "task-v2", Items: cloneTestTaskVersion(plan.Version).Items, FailurePolicy: plan.Version.FailurePolicy, CreatedAt: 2}
	tests := []struct {
		name        string
		aggregate   ExecutionFlowAggregate
		publication ExecutionFlowVersionPublication
	}{
		{name: "invalid aggregate", aggregate: func() ExecutionFlowAggregate { value := created; value.Task.ID = " "; return value }(), publication: publication},
		{name: "deleted aggregate", aggregate: func() ExecutionFlowAggregate { value := created; value.Task.DeletedAt = 1; return value }(), publication: publication},
		{name: "blank publication id", aggregate: created, publication: func() ExecutionFlowVersionPublication { value := publication; value.ID = " "; return value }()},
		{name: "duplicate publication id", aggregate: created, publication: func() ExecutionFlowVersionPublication {
			value := publication
			value.ID = created.Current.ID
			return value
		}()},
		{name: "revision overflow", aggregate: func() ExecutionFlowAggregate {
			value := created
			value.Task.Revision = Revision(math.MaxUint64)
			return value
		}(), publication: publication},
		{name: "invalid published version", aggregate: created, publication: func() ExecutionFlowVersionPublication {
			value := publication
			value.FailurePolicy = "UNKNOWN"
			return value
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.aggregate.PublishVersion(test.publication); err == nil {
				t.Fatal("invalid publication accepted")
			}
		})
	}
	invalidInitial := plan.Version
	invalidInitial.FailurePolicy = "UNKNOWN"
	if _, err := NewExecutionFlow(plan.Task, invalidInitial); err == nil {
		t.Fatal("NewExecutionFlow accepted invalid initial version")
	}
}

func TestVersionHistoryPublicationAndClonePublicBoundaries(t *testing.T) {
	nodeWithoutCurrent := versionedNodeAggregate()
	nodeWithoutCurrent.Node.CurrentVersionID = ""
	nodeWithoutCurrent.Current = NodeVersion{}
	nodeWithoutCurrent.Versions = []NodeVersion{{ID: "v1", NodeID: "other", VersionNumber: 1, DeletedAt: 1}}
	if err := nodeWithoutCurrent.ValidateLoadedHistory(); err == nil {
		t.Fatal("cross-node history accepted")
	}
	nodeWithoutCurrent.Versions[0].NodeID = nodeWithoutCurrent.Node.ID
	nodeWithoutCurrent.Versions[0].DeletedAt = 0
	if err := nodeWithoutCurrent.ValidateLoadedHistory(); err == nil {
		t.Fatal("available node version without current pointer accepted")
	}

	workflowWithoutCurrent := versionedWorkflowAggregate()
	workflowWithoutCurrent.FlowFragment.CurrentVersionID = ""
	workflowWithoutCurrent.Current = FlowFragmentVersion{}
	workflowWithoutCurrent.Versions = []FlowFragmentVersion{{ID: "v1", FlowFragmentID: "other", VersionNumber: 1, DeletedAt: 1}}
	if err := workflowWithoutCurrent.ValidateLoadedHistory(); err == nil {
		t.Fatal("cross-workflow history accepted")
	}
	workflowWithoutCurrent.Versions[0].FlowFragmentID = workflowWithoutCurrent.FlowFragment.ID
	workflowWithoutCurrent.Versions[0].DeletedAt = 0
	if err := workflowWithoutCurrent.ValidateLoadedHistory(); err == nil {
		t.Fatal("available workflow version without current pointer accepted")
	}

	node := versionedNodeAggregate()
	overflowRevisionNode := node
	overflowRevisionNode.Node.Revision = Revision(math.MaxUint64)
	if _, err := overflowRevisionNode.PublishVersion("node-v3", "", "", node.Current.Selectors, node.Current.Fingerprint, SourceManual, 3); err == nil {
		t.Fatal("node publication revision overflow accepted")
	}
	overflowVersionNode := node.Clone()
	overflowVersionNode.Versions[0].VersionNumber = math.MaxInt
	if _, err := overflowVersionNode.PublishVersion("node-v3", "", "", node.Current.Selectors, node.Current.Fingerprint, SourceManual, 3); !errors.Is(err, ErrVersionNumberOverflow) {
		t.Fatalf("node publication version overflow error = %v", err)
	}

	workflow := versionedWorkflowAggregate()
	overflowRevisionWorkflow := workflow
	overflowRevisionWorkflow.FlowFragment.Revision = Revision(math.MaxUint64)
	if _, err := overflowRevisionWorkflow.PublishVersion("workflow-v3", workflow.Current.Definition, 3); err == nil {
		t.Fatal("workflow publication revision overflow accepted")
	}
	overflowVersionWorkflow := workflow
	overflowVersionWorkflow.Versions = append([]FlowFragmentVersion(nil), workflow.Versions...)
	overflowVersionWorkflow.Versions[0].VersionNumber = math.MaxInt
	if _, err := overflowVersionWorkflow.PublishVersion("workflow-v3", workflow.Current.Definition, 3); !errors.Is(err, ErrVersionNumberOverflow) {
		t.Fatalf("workflow publication version overflow error = %v", err)
	}

	clone := node.Clone()
	if clone.Node.ID != node.Node.ID || len(clone.Versions) != len(node.Versions) {
		t.Fatalf("Clone() = %#v", clone)
	}
	if next, err := Revision(math.MaxUint64).Next(); next != 0 || !errors.Is(err, ErrRevisionOverflow) {
		t.Fatalf("Revision.Next() = %d, %v", next, err)
	}
	if next, err := Revision(0).Next(); next != 0 || !errors.Is(err, ErrRevisionZero) {
		t.Fatalf("zero Revision.Next() = %d, %v", next, err)
	}
}

func TestNewFolderForestRejectsInvalidFolder(t *testing.T) {
	if _, err := NewFolderForest([]Folder{{ID: " ", DisplayName: "Folder", Kind: FolderWorkflow}}); err == nil {
		t.Fatal("NewFolderForest accepted invalid folder")
	}
}
