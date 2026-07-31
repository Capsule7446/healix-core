package automation

import (
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestNodeAggregateValidateLoadedHistoryMatrix(t *testing.T) {
	current := versionedNodeVersion("node-v2", "node", 2, 0)
	base := ElementTargetAggregate{
		ElementTarget: ElementTarget{ID: "node", DisplayName: "节点", Properties: Properties{}, CurrentVersionID: current.ID},
		Current:       current,
		Versions:      []ElementTargetVersion{current, versionedNodeVersion("node-v1", "node", 1, 2)},
	}
	tests := []struct {
		name      string
		mutate    func(*ElementTargetAggregate)
		wantCode  fault.Code
		wantField string
	}{
		{name: "unsorted complete history"},
		{name: "current absent from history", mutate: func(aggregate *ElementTargetAggregate) { aggregate.Versions = aggregate.Versions[1:] }, wantCode: fault.CodeFieldMismatch, wantField: "currentVersionId"},
		{name: "history owner mismatch", mutate: func(aggregate *ElementTargetAggregate) { aggregate.Versions[1].ElementTargetID = "other" }, wantCode: fault.CodeFieldMismatch, wantField: "versions"},
		{name: "blank history version id", mutate: func(aggregate *ElementTargetAggregate) { aggregate.Versions[1].ID = "" }, wantCode: fault.CodeFieldInvalid, wantField: "versions"},
		{name: "duplicate history id", mutate: func(aggregate *ElementTargetAggregate) { aggregate.Versions[1].ID = aggregate.Versions[0].ID }, wantCode: fault.CodeFieldDuplicate, wantField: "versions"},
		{name: "duplicate history number", mutate: func(aggregate *ElementTargetAggregate) { aggregate.Versions[1].VersionNumber = 2 }, wantCode: fault.CodeFieldDuplicate, wantField: "versions"},
		{name: "gap in history numbers", mutate: func(aggregate *ElementTargetAggregate) { aggregate.Versions[0].VersionNumber = 3 }, wantCode: fault.CodeFieldInvalid, wantField: "versions"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			aggregate := base
			aggregate.Versions = append([]ElementTargetVersion(nil), base.Versions...)
			if test.mutate != nil {
				test.mutate(&aggregate)
			}
			err := aggregate.ValidateLoadedHistory()
			if test.wantField == "" {
				if err != nil {
					t.Fatalf("valid history rejected: %v", err)
				}
				return
			}
			requireViolationOf(t, err, CodeElementTargetHistoryInvalid, test.wantCode, test.wantField)
		})
	}
}

func TestLoadedHistoryWithoutCurrentRequiresAllVersionsDeleted(t *testing.T) {
	tests := []struct {
		name      string
		run       func() error
		wantCode  fault.Code
		wantField string
		envelope  fault.Code
	}{
		{name: "node all deleted", run: func() error {
			return (ElementTargetAggregate{ElementTarget: ElementTarget{ID: "node"}, Versions: []ElementTargetVersion{
				versionedNodeVersion("node-v2", "node", 2, 3), versionedNodeVersion("node-v1", "node", 1, 2),
			}}).ValidateLoadedHistory()
		}},
		{name: "node available version", run: func() error {
			return (ElementTargetAggregate{ElementTarget: ElementTarget{ID: "node"}, Versions: []ElementTargetVersion{versionedNodeVersion("node-v1", "node", 1, 0)}}).ValidateLoadedHistory()
		}, envelope: CodeElementTargetHistoryInvalid, wantCode: fault.CodeFieldRequired, wantField: "currentVersionId"},
		{name: "node carries current value", run: func() error {
			return (ElementTargetAggregate{ElementTarget: ElementTarget{ID: "node"}, Current: versionedNodeVersion("node-v1", "node", 1, 2)}).ValidateLoadedHistory()
		}, envelope: CodeElementTargetHistoryInvalid, wantCode: fault.CodeFieldMismatch, wantField: "currentVersionId"},
		{name: "workflow all deleted", run: func() error {
			return (FlowFragmentAggregate{FlowFragment: FlowFragment{ID: "workflow"}, Versions: []FlowFragmentVersion{
				versionedWorkflowVersion("workflow-v2", "workflow", 2, 3), versionedWorkflowVersion("workflow-v1", "workflow", 1, 2),
			}}).ValidateLoadedHistory()
		}},
		{name: "workflow available version", run: func() error {
			return (FlowFragmentAggregate{FlowFragment: FlowFragment{ID: "workflow"}, Versions: []FlowFragmentVersion{versionedWorkflowVersion("workflow-v1", "workflow", 1, 0)}}).ValidateLoadedHistory()
		}, envelope: CodeFlowFragmentHistoryInvalid, wantCode: fault.CodeFieldRequired, wantField: "currentVersionId"},
		{name: "workflow carries current value", run: func() error {
			return (FlowFragmentAggregate{FlowFragment: FlowFragment{ID: "workflow"}, Current: versionedWorkflowVersion("workflow-v1", "workflow", 1, 2)}).ValidateLoadedHistory()
		}, envelope: CodeFlowFragmentHistoryInvalid, wantCode: fault.CodeFieldMismatch, wantField: "currentVersionId"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run()
			if test.wantField == "" {
				if err != nil {
					t.Fatalf("valid all-deleted history rejected: %v", err)
				}
				return
			}
			requireViolationOf(t, err, test.envelope, test.wantCode, test.wantField)
		})
	}
}

func TestWorkflowAggregateValidateLoadedHistoryIdentityMatrix(t *testing.T) {
	current := versionedWorkflowVersion("workflow-v2", "workflow", 2, 0)
	base := FlowFragmentAggregate{
		FlowFragment: FlowFragment{ID: "workflow", DisplayName: "流程", Properties: Properties{}, CurrentVersionID: current.ID},
		Current:      current,
		Versions:     []FlowFragmentVersion{versionedWorkflowVersion("workflow-v1", "workflow", 1, 2), current},
	}
	tests := []struct {
		name      string
		mutate    func(*FlowFragmentAggregate)
		wantCode  fault.Code
		wantField string
	}{
		{name: "complete history"},
		{name: "current absent", mutate: func(aggregate *FlowFragmentAggregate) { aggregate.Versions = aggregate.Versions[:1] }, wantCode: fault.CodeFieldMismatch, wantField: "currentVersionId"},
		{name: "owner mismatch", mutate: func(aggregate *FlowFragmentAggregate) { aggregate.Versions[0].FlowFragmentID = "other" }, wantCode: fault.CodeFieldMismatch, wantField: "versions"},
		{name: "duplicate id", mutate: func(aggregate *FlowFragmentAggregate) { aggregate.Versions[0].ID = aggregate.Versions[1].ID }, wantCode: fault.CodeFieldDuplicate, wantField: "versions"},
		{name: "duplicate number", mutate: func(aggregate *FlowFragmentAggregate) { aggregate.Versions[0].VersionNumber = 2 }, wantCode: fault.CodeFieldDuplicate, wantField: "versions"},
		{name: "gap", mutate: func(aggregate *FlowFragmentAggregate) { aggregate.Versions[1].VersionNumber = 3 }, wantCode: fault.CodeFieldInvalid, wantField: "versions"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			aggregate := base
			aggregate.Versions = append([]FlowFragmentVersion(nil), base.Versions...)
			if test.mutate != nil {
				test.mutate(&aggregate)
			}
			err := aggregate.ValidateLoadedHistory()
			if test.wantField == "" {
				if err != nil {
					t.Fatalf("valid history rejected: %v", err)
				}
				return
			}
			requireViolationOf(t, err, CodeFlowFragmentHistoryInvalid, test.wantCode, test.wantField)
		})
	}
}

func TestWorkflowVersionValidateForUsesSelectedHistoricalVersion(t *testing.T) {
	workflow := FlowFragment{ID: "workflow", DisplayName: "流程", Properties: Properties{}, CurrentVersionID: "workflow-v2"}
	historical := versionedWorkflowVersion("workflow-v1", "workflow", 1, 0)
	if err := historical.ValidateFor(workflow); err != nil {
		t.Fatalf("historical selected version rejected: %v", err)
	}
	historical.FlowFragmentID = "other"
	requireViolationOf(t, historical.ValidateFor(workflow), CodeFlowFragmentInvalid, fault.CodeFieldMismatch, "current")
}

func TestPublishVersionRejectsNewVersionIdentityMatrix(t *testing.T) {
	node := versionedNodeAggregate()
	workflow := versionedWorkflowAggregate()
	tests := []struct {
		name           string
		versionID      string
		at             int64
		envelope       fault.Code
		wantCode       fault.Code
		wantField      string
		crossAggregate bool
	}{
		{name: "blank id", versionID: " ", at: 2, wantCode: fault.CodeFieldRequired, wantField: "versionId"},
		{name: "non-positive timestamp", versionID: "v2", at: 0, crossAggregate: true, wantCode: fault.CodeFieldInvalid, wantField: "at"},
		{name: "same as current", versionID: "current", at: 2, wantCode: fault.CodeFieldInvalid, wantField: "versionId"},
		{name: "already in history", versionID: "old", at: 2, wantCode: fault.CodeFieldDuplicate, wantField: "versionId"},
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
			envelope := CodeElementTargetHistoryInvalid
			if test.crossAggregate {
				envelope = CodeAggregateTransitionInvalid
			}
			requireViolationOf(t, err, envelope, test.wantCode, test.wantField)
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
			envelope := CodeFlowFragmentHistoryInvalid
			if test.crossAggregate {
				envelope = CodeAggregateTransitionInvalid
			}
			requireViolationOf(t, err, envelope, test.wantCode, test.wantField)
		})
	}
}

func TestPublishVersionRejectsInvalidPublishedContentAndCurrentAggregate(t *testing.T) {
	t.Run("invalid node publication content", func(t *testing.T) {
		aggregate := versionedNodeAggregate()
		_, err := aggregate.PublishVersion("node-v3", "", "", nil, fingerprint.Fingerprint{}, SourceManual, 3)
		requireViolationOf(t, err, CodeElementTargetInvalid, fault.CodeFieldRequired, "current.selectors")
	})
	t.Run("invalid workflow publication content", func(t *testing.T) {
		aggregate := versionedWorkflowAggregate()
		_, err := aggregate.PublishVersion("workflow-v3", FlowFragmentContent{}, 3)
		requireViolationOf(t, err, CodeFlowFragmentInvalid, fault.CodeFieldRequired, "steps")
	})
	t.Run("invalid workflow current aggregate", func(t *testing.T) {
		aggregate := versionedWorkflowAggregate()
		aggregate.FlowFragment.CurrentVersionID = "other"
		_, err := aggregate.PublishVersion("workflow-v3", aggregate.Current.Definition, 3)
		requireViolationOf(t, err, CodeFlowFragmentInvalid, fault.CodeFieldMismatch, "currentVersionId")
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
	definition := FlowFragmentContent{
		Parameters: []ParameterDefinition{{Name: "region", DisplayName: "区域", Type: parameter.MultiSelect, Options: []string{"east", "west"}, Default: parameter.PresentValue(parameter.MultiSelectValue([]string{"east"}))}},
		Steps: []FlowFragmentStep{
			{ID: "select", DisplayName: "选择", Kind: StepAction, Action: "select", ElementTargetID: "node", ElementTargetVersionID: "node-v1", Values: []string{"east"}},
			{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1, Children: []FlowFragmentStep{{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}},
			{ID: "ref", DisplayName: "引用", Kind: StepFlowFragmentRef, Reference: &FlowFragmentReference{FlowFragmentID: "child", WorkflowVersionID: "child-v1", ParameterBindings: map[string]parameter.Binding{"region": parameter.LiteralBinding(parameter.SingleSelectValue("east"))}}},
			{ID: "validation", DisplayName: "验证", Kind: StepValidation, ElementTargetID: "node", ElementTargetVersionID: "node-v1", Validation: &ValidationConfig{
				Assertion: ValidationAssertion{Kind: ValidationSelectedSetEquals, ExpectedValues: []string{"east"}},
				Wait:      ValidationWait{MaxWaitMS: 2_000, StabilityMS: 200}, SupportedKinds: []ValidationAssertionKind{ValidationSelectedSetEquals},
			}},
			{ID: "group", DisplayName: "验证组", Kind: StepValidationGroup, ValidationGroup: &ValidationGroup{
				Wait: ValidationWait{MaxWaitMS: 2_000, StabilityMS: 200},
				Branches: []ValidationBranch{
					{ID: "branch", Name: "分支", Steps: []FlowFragmentStep{
						{
							ID: "member", DisplayName: "成员", Kind: StepValidation, ElementTargetID: "node", ElementTargetVersionID: "node-v1",
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
	definition.Steps[2].Reference.ParameterBindings["region"] = parameter.LiteralBinding(parameter.SingleSelectValue("mutated"))
	definition.Steps[3].Validation.Assertion.ExpectedValues[0] = "mutated"
	definition.Steps[3].Validation.SupportedKinds[0] = ValidationExists
	definition.Steps[4].ValidationGroup.Branches[0].Steps[0].DisplayName = "mutated"

	got := published.Current.Definition
	if got.Parameters[0].Options[0] != "east" || got.Steps[0].Values[0] != "east" ||
		got.Steps[1].Children[0].DisplayName != "等待" || !literalBindingEqual(got.Steps[2].Reference.ParameterBindings["region"], parameter.SingleSelectValue("east")) ||
		got.Steps[3].Validation.Assertion.ExpectedValues[0] != "east" || got.Steps[3].Validation.SupportedKinds[0] != ValidationSelectedSetEquals ||
		got.Steps[4].ValidationGroup.Branches[0].Steps[0].DisplayName != "成员" {
		t.Fatalf("published definition aliases caller input: %#v", got)
	}
}

func versionedNodeVersion(id, nodeID string, number int, deletedAt int64) ElementTargetVersion {
	return ElementTargetVersion{ID: id, ElementTargetID: nodeID, VersionNumber: number, DeletedAt: deletedAt,
		Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#node"}},
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}, Source: SourceManual}
}

func versionedWorkflowVersion(id, workflowID string, number int, deletedAt int64) FlowFragmentVersion {
	return FlowFragmentVersion{ID: id, FlowFragmentID: workflowID, VersionNumber: number, DeletedAt: deletedAt,
		Definition: FlowFragmentContent{Steps: []FlowFragmentStep{{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}}}
}

func versionedNodeAggregate() ElementTargetAggregate {
	v1 := versionedNodeVersion("node-v1", "node", 1, 2)
	v2 := versionedNodeVersion("node-v2", "node", 2, 0)
	return ElementTargetAggregate{ElementTarget: ElementTarget{ID: "node", DisplayName: "节点", Properties: Properties{}, CurrentVersionID: v2.ID, CreatedAt: 1, UpdatedAt: 2, Revision: 1},
		Current: v2, Versions: []ElementTargetVersion{v1, v2}}
}

func versionedWorkflowAggregate() FlowFragmentAggregate {
	v1 := versionedWorkflowVersion("workflow-v1", "workflow", 1, 2)
	v2 := versionedWorkflowVersion("workflow-v2", "workflow", 2, 0)
	return FlowFragmentAggregate{FlowFragment: FlowFragment{ID: "workflow", DisplayName: "流程", Properties: Properties{}, CurrentVersionID: v2.ID, CreatedAt: 1, UpdatedAt: 2, Revision: 1},
		Current: v2, Versions: []FlowFragmentVersion{v1, v2}}
}
