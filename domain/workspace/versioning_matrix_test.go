package workspace

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestNodeAggregateValidateLoadedHistoryMatrix(t *testing.T) {
	current := versionedNodeVersion("node-v2", "node", 2, 0)
	base := NodeAggregate{
		Node:     Node{ID: "node", DisplayName: "节点", Properties: Properties{}, CurrentVersionID: current.ID},
		Current:  current,
		Versions: []NodeVersion{current, versionedNodeVersion("node-v1", "node", 1, 2)},
	}
	tests := []struct {
		name   string
		mutate func(*NodeAggregate)
		want   string
	}{
		{name: "unsorted complete history"},
		{name: "current absent from history", mutate: func(aggregate *NodeAggregate) { aggregate.Versions = aggregate.Versions[1:] }, want: "missing from loaded history"},
		{name: "history owner mismatch", mutate: func(aggregate *NodeAggregate) { aggregate.Versions[1].NodeID = "other" }, want: "belongs to another node"},
		{name: "blank history version id", mutate: func(aggregate *NodeAggregate) { aggregate.Versions[1].ID = "" }, want: "invalid version identity"},
		{name: "duplicate history id", mutate: func(aggregate *NodeAggregate) { aggregate.Versions[1].ID = aggregate.Versions[0].ID }, want: "duplicate version identity"},
		{name: "duplicate history number", mutate: func(aggregate *NodeAggregate) { aggregate.Versions[1].VersionNumber = 2 }, want: "duplicate version identity"},
		{name: "gap in history numbers", mutate: func(aggregate *NodeAggregate) { aggregate.Versions[0].VersionNumber = 3 }, want: "contiguous from 1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			aggregate := base
			aggregate.Versions = append([]NodeVersion(nil), base.Versions...)
			if test.mutate != nil {
				test.mutate(&aggregate)
			}
			err := aggregate.ValidateLoadedHistory()
			if test.want == "" && err != nil {
				t.Fatalf("valid history rejected: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("ValidateLoadedHistory() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestLoadedHistoryWithoutCurrentRequiresAllVersionsDeleted(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{name: "node all deleted", run: func() error {
			return (NodeAggregate{Node: Node{ID: "node"}, Versions: []NodeVersion{
				versionedNodeVersion("node-v2", "node", 2, 3), versionedNodeVersion("node-v1", "node", 1, 2),
			}}).ValidateLoadedHistory()
		}},
		{name: "node available version", run: func() error {
			return (NodeAggregate{Node: Node{ID: "node"}, Versions: []NodeVersion{versionedNodeVersion("node-v1", "node", 1, 0)}}).ValidateLoadedHistory()
		}, want: "requires a current pointer"},
		{name: "node carries current value", run: func() error {
			return (NodeAggregate{Node: Node{ID: "node"}, Current: versionedNodeVersion("node-v1", "node", 1, 2)}).ValidateLoadedHistory()
		}, want: "cannot carry a current version"},
		{name: "workflow all deleted", run: func() error {
			return (WorkflowAggregate{Workflow: Workflow{ID: "workflow"}, Versions: []WorkflowVersion{
				versionedWorkflowVersion("workflow-v2", "workflow", 2, 3), versionedWorkflowVersion("workflow-v1", "workflow", 1, 2),
			}}).ValidateLoadedHistory()
		}},
		{name: "workflow available version", run: func() error {
			return (WorkflowAggregate{Workflow: Workflow{ID: "workflow"}, Versions: []WorkflowVersion{versionedWorkflowVersion("workflow-v1", "workflow", 1, 0)}}).ValidateLoadedHistory()
		}, want: "requires a current pointer"},
		{name: "workflow carries current value", run: func() error {
			return (WorkflowAggregate{Workflow: Workflow{ID: "workflow"}, Current: versionedWorkflowVersion("workflow-v1", "workflow", 1, 2)}).ValidateLoadedHistory()
		}, want: "cannot carry a current version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run()
			if test.want == "" && err != nil {
				t.Fatalf("valid all-deleted history rejected: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("ValidateLoadedHistory() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestWorkflowAggregateValidateLoadedHistoryIdentityMatrix(t *testing.T) {
	current := versionedWorkflowVersion("workflow-v2", "workflow", 2, 0)
	base := WorkflowAggregate{
		Workflow: Workflow{ID: "workflow", DisplayName: "流程", Properties: Properties{}, CurrentVersionID: current.ID},
		Current:  current,
		Versions: []WorkflowVersion{versionedWorkflowVersion("workflow-v1", "workflow", 1, 2), current},
	}
	tests := []struct {
		name   string
		mutate func(*WorkflowAggregate)
		want   string
	}{
		{name: "complete history"},
		{name: "current absent", mutate: func(aggregate *WorkflowAggregate) { aggregate.Versions = aggregate.Versions[:1] }, want: "missing from loaded history"},
		{name: "owner mismatch", mutate: func(aggregate *WorkflowAggregate) { aggregate.Versions[0].WorkflowID = "other" }, want: "belongs to another workflow"},
		{name: "duplicate id", mutate: func(aggregate *WorkflowAggregate) { aggregate.Versions[0].ID = aggregate.Versions[1].ID }, want: "duplicate version identity"},
		{name: "duplicate number", mutate: func(aggregate *WorkflowAggregate) { aggregate.Versions[0].VersionNumber = 2 }, want: "duplicate version identity"},
		{name: "gap", mutate: func(aggregate *WorkflowAggregate) { aggregate.Versions[1].VersionNumber = 3 }, want: "contiguous from 1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			aggregate := base
			aggregate.Versions = append([]WorkflowVersion(nil), base.Versions...)
			if test.mutate != nil {
				test.mutate(&aggregate)
			}
			err := aggregate.ValidateLoadedHistory()
			if test.want == "" && err != nil {
				t.Fatalf("valid history rejected: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("ValidateLoadedHistory() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestWorkflowVersionValidateForUsesSelectedHistoricalVersion(t *testing.T) {
	workflow := Workflow{ID: "workflow", DisplayName: "流程", Properties: Properties{}, CurrentVersionID: "workflow-v2"}
	historical := versionedWorkflowVersion("workflow-v1", "workflow", 1, 0)
	if err := historical.ValidateFor(workflow); err != nil {
		t.Fatalf("historical selected version rejected: %v", err)
	}
	historical.WorkflowID = "other"
	if err := historical.ValidateFor(workflow); err == nil || !strings.Contains(err.Error(), "belong to workflow") {
		t.Fatalf("wrong historical owner error = %v", err)
	}
}

func TestPublishVersionRejectsNewVersionIdentityMatrix(t *testing.T) {
	node := versionedNodeAggregate()
	workflow := versionedWorkflowAggregate()
	tests := []struct {
		name      string
		versionID string
		at        int64
		want      string
	}{
		{name: "blank id", versionID: " ", at: 2, want: "version id is required"},
		{name: "non-positive timestamp", versionID: "v2", at: 0, want: "time must be positive"},
		{name: "same as current", versionID: "current", at: 2, want: "differ from the current"},
		{name: "already in history", versionID: "old", at: 2, want: "already exists in history"},
	}
	for _, test := range tests {
		t.Run("node "+test.name, func(t *testing.T) {
			versionID := test.versionID
			if versionID == "current" {
				versionID = node.Current.ID
			}
			if versionID == "old" {
				versionID = node.Versions[0].ID
			}
			_, err := node.PublishVersion(versionID, "", "", node.Current.Selectors, node.Current.Fingerprint, SourceManual, test.at)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("PublishVersion() error = %v, want substring %q", err, test.want)
			}
		})
		t.Run("workflow "+test.name, func(t *testing.T) {
			versionID := test.versionID
			if versionID == "current" {
				versionID = workflow.Current.ID
			}
			if versionID == "old" {
				versionID = workflow.Versions[0].ID
			}
			_, err := workflow.PublishVersion(versionID, workflow.Current.Definition, test.at)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("PublishVersion() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestPublishVersionRejectsInvalidPublishedContentAndCurrentAggregate(t *testing.T) {
	t.Run("invalid node publication content", func(t *testing.T) {
		aggregate := versionedNodeAggregate()
		_, err := aggregate.PublishVersion("node-v3", "", "", nil, fingerprint.Fingerprint{}, SourceManual, 3)
		if err == nil || !strings.Contains(err.Error(), "at least one selector") {
			t.Fatalf("invalid node version error = %v", err)
		}
	})
	t.Run("invalid workflow publication content", func(t *testing.T) {
		aggregate := versionedWorkflowAggregate()
		_, err := aggregate.PublishVersion("workflow-v3", WorkflowDefinition{}, 3)
		if err == nil || !strings.Contains(err.Error(), "requires at least one step") {
			t.Fatalf("invalid workflow version error = %v", err)
		}
	})
	t.Run("invalid workflow current aggregate", func(t *testing.T) {
		aggregate := versionedWorkflowAggregate()
		aggregate.Workflow.CurrentVersionID = "other"
		_, err := aggregate.PublishVersion("workflow-v3", aggregate.Current.Definition, 3)
		if err == nil || !strings.Contains(err.Error(), "invalid current workflow aggregate") {
			t.Fatalf("invalid current workflow error = %v", err)
		}
	})
}

func TestPublishVersionCountsCurrentWhenLoadedHistoryOmitsIt(t *testing.T) {
	node := versionedNodeAggregate()
	node.Versions = node.Versions[:1]
	publishedNode, err := node.PublishVersion("node-v3", "", "", node.Current.Selectors, node.Current.Fingerprint, SourceManual, 3)
	if err != nil {
		t.Fatalf("publish node from list-style history: %v", err)
	}
	if publishedNode.Current.VersionNumber != 3 {
		t.Fatalf("node version number = %d, want 3", publishedNode.Current.VersionNumber)
	}

	workflow := versionedWorkflowAggregate()
	workflow.Versions = workflow.Versions[:1]
	publishedWorkflow, err := workflow.PublishVersion("workflow-v3", workflow.Current.Definition, 3)
	if err != nil {
		t.Fatalf("publish workflow from list-style history: %v", err)
	}
	if publishedWorkflow.Current.VersionNumber != 3 {
		t.Fatalf("workflow version number = %d, want 3", publishedWorkflow.Current.VersionNumber)
	}
}

func TestWorkflowPublishVersionDeepCopiesEveryMutableDefinitionField(t *testing.T) {
	aggregate := versionedWorkflowAggregate()
	definition := WorkflowDefinition{
		Parameters: []ParameterDefinition{{Name: "region", DisplayName: "区域", Type: ParameterMultiSelect, Options: []string{"east", "west"}}},
		Steps: []WorkflowStep{
			{ID: "select", DisplayName: "选择", Kind: StepAction, Action: "select", NodeID: "node", NodeVersionID: "node-v1", Values: []string{"east"}},
			{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1, Children: []WorkflowStep{{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}},
			{ID: "ref", DisplayName: "引用", Kind: StepWorkflowRef, Reference: &WorkflowReference{WorkflowID: "child", WorkflowVersionID: "child-v1", ParameterBindings: map[string]string{"region": "east"}}},
			{ID: "validation", DisplayName: "验证", Kind: StepValidation, NodeID: "node", NodeVersionID: "node-v1", Validation: &ValidationConfig{
				Assertion: ValidationAssertion{Kind: ValidationSelectedSetEquals, ExpectedValues: []string{"east"}},
				Wait:      ValidationWait{MaxWaitMS: 2_000, StabilityMS: 200}, SupportedKinds: []ValidationAssertionKind{ValidationSelectedSetEquals},
			}},
			{ID: "group", DisplayName: "验证组", Kind: StepValidationGroup, ValidationGroup: &ValidationGroup{
				Wait: ValidationWait{MaxWaitMS: 2_000, StabilityMS: 200},
				Branches: []ValidationBranch{
					{ID: "branch", Name: "分支", Steps: []WorkflowStep{
						{
							ID: "member", DisplayName: "成员", Kind: StepValidation, NodeID: "node", NodeVersionID: "node-v1",
							Validation: &ValidationConfig{Assertion: ValidationAssertion{Kind: ValidationVisible}},
						},
					}},
				},
			}},
		},
	}
	published, err := aggregate.PublishVersion("workflow-v3", definition, 3)
	if err != nil {
		t.Fatalf("PublishVersion: %v", err)
	}

	definition.Parameters[0].Options[0] = "mutated"
	definition.Steps[0].Values[0] = "mutated"
	definition.Steps[1].Children[0].DisplayName = "mutated"
	definition.Steps[2].Reference.ParameterBindings["region"] = "mutated"
	definition.Steps[3].Validation.Assertion.ExpectedValues[0] = "mutated"
	definition.Steps[3].Validation.SupportedKinds[0] = ValidationExists
	definition.Steps[4].ValidationGroup.Branches[0].Steps[0].DisplayName = "mutated"

	got := published.Current.Definition
	if got.Parameters[0].Options[0] != "east" || got.Steps[0].Values[0] != "east" ||
		got.Steps[1].Children[0].DisplayName != "等待" || got.Steps[2].Reference.ParameterBindings["region"] != "east" ||
		got.Steps[3].Validation.Assertion.ExpectedValues[0] != "east" || got.Steps[3].Validation.SupportedKinds[0] != ValidationSelectedSetEquals ||
		got.Steps[4].ValidationGroup.Branches[0].Steps[0].DisplayName != "成员" {
		t.Fatalf("published definition aliases caller input: %#v", got)
	}
}

func versionedNodeVersion(id, nodeID string, number int, deletedAt int64) NodeVersion {
	return NodeVersion{ID: id, NodeID: nodeID, VersionNumber: number, DeletedAt: deletedAt,
		Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#node"}},
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}, Source: SourceManual}
}

func versionedWorkflowVersion(id, workflowID string, number int, deletedAt int64) WorkflowVersion {
	return WorkflowVersion{ID: id, WorkflowID: workflowID, VersionNumber: number, DeletedAt: deletedAt,
		Definition: WorkflowDefinition{Steps: []WorkflowStep{{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}}}
}

func versionedNodeAggregate() NodeAggregate {
	v1 := versionedNodeVersion("node-v1", "node", 1, 2)
	v2 := versionedNodeVersion("node-v2", "node", 2, 0)
	return NodeAggregate{Node: Node{ID: "node", DisplayName: "节点", Properties: Properties{}, CurrentVersionID: v2.ID, CreatedAt: 1, UpdatedAt: 2},
		Current: v2, Versions: []NodeVersion{v1, v2}}
}

func versionedWorkflowAggregate() WorkflowAggregate {
	v1 := versionedWorkflowVersion("workflow-v1", "workflow", 1, 2)
	v2 := versionedWorkflowVersion("workflow-v2", "workflow", 2, 0)
	return WorkflowAggregate{Workflow: Workflow{ID: "workflow", DisplayName: "流程", Properties: Properties{}, CurrentVersionID: v2.ID, CreatedAt: 1, UpdatedAt: 2},
		Current: v2, Versions: []WorkflowVersion{v1, v2}}
}
