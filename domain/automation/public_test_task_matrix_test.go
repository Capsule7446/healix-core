package automation

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestSamplingPublicationValidatePublicScenarioMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SamplingPublication)
		want   string
	}{
		{name: "invalid workflow", mutate: func(p *SamplingPublication) { p.FlowFragment.FlowFragment.ID = " " }, want: "sampled workflow"},
		{name: "merge invalid expected revision", mutate: func(p *SamplingPublication) {
			p.Nodes[0].ResolutionMode = "MERGE"
			p.Nodes[0].Aggregate = versionedNodeAggregate()
			// Revision.Next's classified fault now surfaces directly instead of being
			// buried inside an unclassified "merge revision" wrapper.
		}, want: string(CodePersistedRevisionInvalid)},
		{name: "merge authority mismatch", mutate: func(p *SamplingPublication) {
			p.Nodes[0].ResolutionMode = "MERGE"
			p.Nodes[0].Aggregate = versionedNodeAggregate()
			p.Nodes[0].ExpectedRevision = 1
			p.Nodes[0].ExpectedCurrentVersionID = "node-v1"
		}, want: "requires current revision"},
		{name: "merge repeats current version", mutate: func(p *SamplingPublication) {
			p.Nodes[0].ResolutionMode = "MERGE"
			p.Nodes[0].Aggregate = versionedNodeAggregate()
			p.Nodes[0].Aggregate.ElementTarget.Revision = 2
			p.Nodes[0].ExpectedRevision = 1
			p.Nodes[0].ExpectedCurrentVersionID = p.Nodes[0].Aggregate.Current.ID
		}, want: "cannot publish the expected current version"},
		{name: "merge version below two", mutate: func(p *SamplingPublication) {
			p.Nodes[0].ResolutionMode = "MERGE"
			p.Nodes[0].Aggregate.ElementTarget.Revision = 2
			p.Nodes[0].ExpectedRevision = 1
			p.Nodes[0].ExpectedCurrentVersionID = "previous-version"
		}, want: "version 2 or later"},
		{name: "duplicate formal version", mutate: func(p *SamplingPublication) {
			second := p.Nodes[0]
			second.Aggregate = p.Nodes[0].Aggregate.Clone()
			second.TemporaryElementTargetID = "temporary-two"
			second.Aggregate.ElementTarget.ID = "node-two"
			second.Aggregate.Current.ElementTargetID = "node-two"
			second.Aggregate.Versions[0] = second.Aggregate.Current
			p.Nodes = append(p.Nodes, second)
		}, want: "duplicate formal sampled node version"},
		{name: "root reference lacks decision", mutate: func(p *SamplingPublication) {
			p.FlowFragment = workflowWithSteps(FlowFragmentStep{ID: "click", DisplayName: "Click", Kind: StepAction, Action: "click", ElementTargetID: "missing", ElementTargetVersionID: "missing-v1"})
		}, want: "no matching node decision"},
		{name: "child reference lacks decision", mutate: func(p *SamplingPublication) {
			p.FlowFragment = workflowWithSteps(FlowFragmentStep{ID: "repeat", DisplayName: "Repeat", Kind: StepRepeat, RepeatCount: 1, Children: []FlowFragmentStep{{ID: "click", DisplayName: "Click", Kind: StepAction, Action: "click", ElementTargetID: "missing", ElementTargetVersionID: "missing-v1"}}})
		}, want: "no matching node decision"},
		{name: "validation branch reference lacks decision", mutate: func(p *SamplingPublication) {
			p.FlowFragment = validationWorkflow(FlowFragmentStep{ID: "group", DisplayName: "Group", Kind: StepValidationGroup, ValidationGroup: &ValidationGroup{Wait: ValidationWait{MaxWaitMS: 10_000, StabilityMS: 500}, Branches: []ValidationBranch{{ID: "branch", Name: "Branch", Steps: []FlowFragmentStep{validationMember("member", "Member")}}}}})
		}, want: "no matching node decision"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publication := samplingCreatePublication().Clone()
			test.mutate(&publication)
			if err := publication.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestTestTaskValidateSingleFactorRuleMatrix(t *testing.T) {
	base := validTestTaskVersionPlan().Task
	tests := []struct {
		name      string
		mutate    func(*ExecutionFlow)
		wantCode  fault.Code
		wantField string
	}{
		{name: "missing id", mutate: func(value *ExecutionFlow) { value.ID = " " }, wantCode: fault.CodeFieldRequired, wantField: "id"},
		{name: "missing display name", mutate: func(value *ExecutionFlow) { value.DisplayName = " " }, wantCode: fault.CodeFieldRequired, wantField: "displayName"},
		{name: "missing current version", mutate: func(value *ExecutionFlow) { value.CurrentVersionID = "" }, wantCode: fault.CodeFieldRequired, wantField: "currentVersionId"},
		{name: "missing created timestamp", mutate: func(value *ExecutionFlow) { value.CreatedAt = 0 }, wantCode: fault.CodeFieldRequired, wantField: "createdAt"},
		{name: "missing updated timestamp", mutate: func(value *ExecutionFlow) { value.UpdatedAt = 0 }, wantCode: fault.CodeFieldRequired, wantField: "updatedAt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.mutate(&value)
			requireExecutionFlowViolation(t, value.Validate(), test.wantCode, test.wantField)
		})
	}
}

func TestTestTaskVersionValidateSingleFactorRuleMatrix(t *testing.T) {
	base := validTestTaskVersionPlan().Version
	tests := []struct {
		name      string
		mutate    func(*ExecutionFlowVersion)
		wantCode  fault.Code
		wantField string
	}{
		{name: "missing identity", mutate: func(value *ExecutionFlowVersion) { value.ID = " " }, wantCode: fault.CodeFieldRequired, wantField: "id"},
		{name: "missing owner", mutate: func(value *ExecutionFlowVersion) { value.ExecutionFlowID = " " }, wantCode: fault.CodeFieldRequired, wantField: "executionFlowId"},
		{name: "version below boundary", mutate: func(value *ExecutionFlowVersion) { value.VersionNumber = 0 }, wantCode: fault.CodeFieldInvalid, wantField: "versionNumber"},
		{name: "missing created timestamp", mutate: func(value *ExecutionFlowVersion) { value.CreatedAt = 0 }, wantCode: fault.CodeFieldRequired, wantField: "createdAt"},
		{name: "unsupported failure policy", mutate: func(value *ExecutionFlowVersion) { value.FailurePolicy = "UNKNOWN" }, wantCode: fault.CodeFieldInvalid, wantField: "failurePolicy"},
		{name: "missing items", mutate: func(value *ExecutionFlowVersion) { value.Items = nil }, wantCode: fault.CodeFieldRequired, wantField: "items"},
		{name: "blank environment key", mutate: func(value *ExecutionFlowVersion) { value.RequiredEnvironmentKeys = []string{" "} }, wantCode: fault.CodeFieldRequired, wantField: "requiredEnvironmentKeys.0"},
		{name: "duplicate environment key", mutate: func(value *ExecutionFlowVersion) {
			value.RequiredEnvironmentKeys = []string{"region", "region"}
		}, wantCode: fault.CodeFieldDuplicate, wantField: "requiredEnvironmentKeys.1"},
		{name: "missing item id", mutate: func(value *ExecutionFlowVersion) { value.Items[0].ID = " " }, wantCode: fault.CodeFieldRequired, wantField: "items.0.id"},
		{name: "duplicate item id", mutate: func(value *ExecutionFlowVersion) {
			second := value.Items[0]
			second.SequenceNumber = 2
			value.Items = append(value.Items, second)
		}, wantCode: fault.CodeFieldDuplicate, wantField: "items.1.id"},
		{name: "wrong item owner", mutate: func(value *ExecutionFlowVersion) { value.Items[0].TestTaskVersionID = "other" }, wantCode: fault.CodeFieldMismatch, wantField: "items.0.executionFlowVersionId"},
		{name: "noncontiguous sequence", mutate: func(value *ExecutionFlowVersion) { value.Items[0].SequenceNumber = 2 }, wantCode: fault.CodeFieldInvalid, wantField: "items.0.sequenceNumber"},
		{name: "missing flow fragment id", mutate: func(value *ExecutionFlowVersion) { value.Items[0].FlowFragmentID = " " }, wantCode: fault.CodeFieldRequired, wantField: "items.0.flowFragmentId"},
		{name: "unsupported version policy", mutate: func(value *ExecutionFlowVersion) { value.Items[0].VersionPolicy = "UNKNOWN" }, wantCode: fault.CodeFieldInvalid, wantField: "items.0.versionPolicy"},
		{name: "fixed policy missing version", mutate: func(value *ExecutionFlowVersion) { value.Items[0].VersionPolicy = FlowFragmentVersionFixed }, wantCode: fault.CodeFieldRequired, wantField: "items.0.flowFragmentVersionId"},
		{name: "latest policy persists version", mutate: func(value *ExecutionFlowVersion) { value.Items[0].WorkflowVersionID = "workflow-v1" }, wantCode: fault.CodeFieldMismatch, wantField: "items.0.flowFragmentVersionId"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := cloneTestTaskVersion(base)
			test.mutate(&value)
			requireExecutionFlowViolation(t, value.Validate(), test.wantCode, test.wantField)
		})
	}
}

func TestTestTaskAggregateValidateHistoryRuleMatrix(t *testing.T) {
	plan := validTestTaskVersionPlan()
	base, err := NewExecutionFlow(plan.Task, plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	versionTwo := cloneTestTaskVersion(base.Current)
	versionTwo.ID = "task-v2"
	versionTwo.VersionNumber = 2
	versionTwo.SourceVersionID = base.Current.ID
	versionTwo.CreatedAt = 2
	versionTwo.Items[0].TestTaskVersionID = versionTwo.ID
	twoVersions := base
	twoVersions.Task.CurrentVersionID = versionTwo.ID
	twoVersions.Task.UpdatedAt = 2
	twoVersions.Current = cloneTestTaskVersion(versionTwo)
	twoVersions.Versions = []ExecutionFlowVersion{cloneTestTaskVersion(base.Current), cloneTestTaskVersion(versionTwo)}

	tests := []struct {
		name   string
		mutate func(*ExecutionFlowAggregate)
		want   string
	}{
		{name: "missing history", mutate: func(value *ExecutionFlowAggregate) { value.Versions = nil }, want: "requires version history"},
		{name: "version belongs elsewhere", mutate: func(value *ExecutionFlowAggregate) { value.Versions[0].ExecutionFlowID = "other" }, want: "another task"},
		{name: "duplicate identity", mutate: func(value *ExecutionFlowAggregate) {
			value.Versions = append(value.Versions, cloneTestTaskVersion(value.Versions[0]))
		}, want: "duplicate version identity"},
		{name: "noncontiguous versions", mutate: func(value *ExecutionFlowAggregate) {
			value.Versions[1].VersionNumber = 3
			value.Current.VersionNumber = 3
		}, want: "contiguous"},
		{name: "version one has source", mutate: func(value *ExecutionFlowAggregate) { value.Versions[0].SourceVersionID = "source" }, want: "version 1"},
		{name: "later version has missing source", mutate: func(value *ExecutionFlowAggregate) { value.Versions[1].SourceVersionID = "missing" }, want: "source must be an earlier version"},
		{name: "current content mismatch", mutate: func(value *ExecutionFlowAggregate) { value.Current.Items[0].FlowFragmentID = "changed" }, want: "content must match history"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := cloneTestTaskAggregate(twoVersions)
			if test.name == "missing history" || test.name == "duplicate identity" || test.name == "version belongs elsewhere" {
				value = cloneTestTaskAggregate(base)
			}
			test.mutate(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestResolveParameterValuesRejectsInvalidAndDuplicateDefinitions(t *testing.T) {
	valid := ParameterDefinition{Name: "value", DisplayName: "Value", Type: parameter.Text, Required: true}
	invalid := valid
	invalid.Name = " "
	if _, err := ResolveParameterValues([]ParameterDefinition{invalid}, nil); err == nil {
		t.Fatal("invalid parameter definition accepted")
	}
	if _, err := ResolveParameterValues([]ParameterDefinition{valid, valid}, nil); err == nil || !strings.Contains(err.Error(), "duplicate parameter") {
		t.Fatalf("duplicate parameter definitions error = %v", err)
	}
}

func TestTestTaskVersionPlanValidateDependencyBoundaryMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ResolvedExecutionFlow)
		want   string
	}{
		{name: "invalid task", mutate: func(plan *ResolvedExecutionFlow) { plan.Task.ID = " " }, want: string(CodeExecutionFlowInvalid)},
		{name: "invalid version", mutate: func(plan *ResolvedExecutionFlow) { plan.Version.ID = " " }, want: string(CodeExecutionFlowInvalid)},
		{name: "inconsistent candidate identity", mutate: func(plan *ResolvedExecutionFlow) { plan.Task.ID = "other" }, want: "candidate identity"},
		{name: "version one carries expected revision", mutate: func(plan *ResolvedExecutionFlow) { plan.ExpectedExecutionFlowRevision = 1 }, want: "without a source version"},
		{name: "invalid workflow dependency", mutate: func(plan *ResolvedExecutionFlow) { plan.Workflows[0].Version.FlowFragmentID = "other" }, want: "workflow dependency snapshot identity"},
		{name: "duplicate workflow dependency", mutate: func(plan *ResolvedExecutionFlow) { plan.Workflows = append(plan.Workflows, plan.Workflows[0]) }, want: "duplicate workflow dependency"},
		{name: "invalid node dependency", mutate: func(plan *ResolvedExecutionFlow) { plan.Nodes = []ElementTargetDependencySnapshot{{}} }, want: "node dependency snapshot identity"},
		{name: "duplicate node dependency", mutate: func(plan *ResolvedExecutionFlow) {
			node := versionedNodeAggregate()
			dependency := ElementTargetDependencySnapshot{ElementTarget: node.ElementTarget, Version: node.Current}
			plan.Nodes = []ElementTargetDependencySnapshot{dependency, dependency}
		}, want: "duplicate node dependency"},
		{name: "invalid item parameter", mutate: func(plan *ResolvedExecutionFlow) {
			plan.Workflows[0].Version.Definition.Parameters = []ParameterDefinition{{Name: "value", DisplayName: "Value", Type: parameter.Text, Required: true}}
			plan.Version.Items[0].Parameters = map[string]parameter.Value{"value": parameter.BooleanValue(true)}
		}, want: "parameters"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := validTestTaskVersionPlan()
			test.mutate(&plan)
			if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestTestTaskVersionPlanAcceptsFixedDependencyAfterUnrelatedSnapshot(t *testing.T) {
	plan := validTestTaskVersionPlan()
	matching := plan.Workflows[0]
	plan.Version.Items[0].VersionPolicy = FlowFragmentVersionFixed
	plan.Version.Items[0].WorkflowVersionID = matching.Version.ID
	matching.ResolvedFromLatest = false
	unrelated := matching
	unrelated.FlowFragment.ID = "unrelated"
	unrelated.FlowFragment.CurrentVersionID = "unrelated-v1"
	unrelated.Version.ID = "unrelated-v1"
	unrelated.Version.FlowFragmentID = unrelated.FlowFragment.ID
	plan.Workflows = []FlowFragmentDependencySnapshot{unrelated, matching}
	second := plan.Version.Items[0]
	second.ID = "item-two"
	second.SequenceNumber = 2
	second.FlowFragmentID = unrelated.FlowFragment.ID
	second.WorkflowVersionID = unrelated.Version.ID
	second.Parameters = map[string]parameter.Value{}
	plan.Version.Items = append(plan.Version.Items, second)

	if err := plan.Validate(); err != nil {
		t.Fatalf("fixed dependency after unrelated snapshot rejected: %v", err)
	}
}
