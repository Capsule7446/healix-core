package automation

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestSamplingPublicationValidatePublicScenarioMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SamplingPublication)
		want   string
	}{
		{name: "invalid workflow", mutate: func(p *SamplingPublication) { p.Workflow.Workflow.ID = " " }, want: "sampled workflow"},
		{name: "merge invalid expected revision", mutate: func(p *SamplingPublication) {
			p.Nodes[0].ResolutionMode = "MERGE"
			p.Nodes[0].Aggregate = versionedNodeAggregate()
		}, want: "merge revision"},
		{name: "merge authority mismatch", mutate: func(p *SamplingPublication) {
			p.Nodes[0].ResolutionMode = "MERGE"
			p.Nodes[0].Aggregate = versionedNodeAggregate()
			p.Nodes[0].ExpectedRevision = 1
			p.Nodes[0].ExpectedCurrentVersionID = "node-v1"
		}, want: "requires current revision"},
		{name: "merge repeats current version", mutate: func(p *SamplingPublication) {
			p.Nodes[0].ResolutionMode = "MERGE"
			p.Nodes[0].Aggregate = versionedNodeAggregate()
			p.Nodes[0].Aggregate.Node.Revision = 2
			p.Nodes[0].ExpectedRevision = 1
			p.Nodes[0].ExpectedCurrentVersionID = p.Nodes[0].Aggregate.Current.ID
		}, want: "cannot publish the expected current version"},
		{name: "merge version below two", mutate: func(p *SamplingPublication) {
			p.Nodes[0].ResolutionMode = "MERGE"
			p.Nodes[0].Aggregate.Node.Revision = 2
			p.Nodes[0].ExpectedRevision = 1
			p.Nodes[0].ExpectedCurrentVersionID = "previous-version"
		}, want: "version 2 or later"},
		{name: "duplicate formal version", mutate: func(p *SamplingPublication) {
			second := p.Nodes[0]
			second.Aggregate = p.Nodes[0].Aggregate.Clone()
			second.TemporaryNodeID = "temporary-two"
			second.Aggregate.Node.ID = "node-two"
			second.Aggregate.Current.NodeID = "node-two"
			second.Aggregate.Versions[0] = second.Aggregate.Current
			p.Nodes = append(p.Nodes, second)
		}, want: "duplicate formal sampled node version"},
		{name: "root reference lacks decision", mutate: func(p *SamplingPublication) {
			p.Workflow = workflowWithSteps(WorkflowStep{ID: "click", DisplayName: "Click", Kind: StepAction, Action: "click", NodeID: "missing", NodeVersionID: "missing-v1"})
		}, want: "no matching node decision"},
		{name: "child reference lacks decision", mutate: func(p *SamplingPublication) {
			p.Workflow = workflowWithSteps(WorkflowStep{ID: "repeat", DisplayName: "Repeat", Kind: StepRepeat, RepeatCount: 1, Children: []WorkflowStep{{ID: "click", DisplayName: "Click", Kind: StepAction, Action: "click", NodeID: "missing", NodeVersionID: "missing-v1"}}})
		}, want: "no matching node decision"},
		{name: "validation branch reference lacks decision", mutate: func(p *SamplingPublication) {
			p.Workflow = validationWorkflow(WorkflowStep{ID: "group", DisplayName: "Group", Kind: StepValidationGroup, ValidationGroup: &ValidationGroup{Wait: ValidationWait{MaxWaitMS: 10_000, StabilityMS: 500}, Branches: []ValidationBranch{{ID: "branch", Name: "Branch", Steps: []WorkflowStep{validationMember("member", "Member")}}}}})
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
		name   string
		mutate func(*TestTask)
		want   string
	}{
		{name: "missing display name", mutate: func(value *TestTask) { value.DisplayName = " " }, want: "display name"},
		{name: "missing current version", mutate: func(value *TestTask) { value.CurrentVersionID = "" }, want: "current version"},
		{name: "missing timestamp", mutate: func(value *TestTask) { value.CreatedAt = 0 }, want: "timestamps"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.mutate(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestTestTaskVersionValidateSingleFactorRuleMatrix(t *testing.T) {
	base := validTestTaskVersionPlan().Version
	tests := []struct {
		name   string
		mutate func(*TestTaskVersion)
		want   string
	}{
		{name: "missing identity", mutate: func(value *TestTaskVersion) { value.ID = " " }, want: "id and owner"},
		{name: "version below boundary", mutate: func(value *TestTaskVersion) { value.VersionNumber = 0 }, want: "version number"},
		{name: "missing created timestamp", mutate: func(value *TestTaskVersion) { value.CreatedAt = 0 }, want: "created timestamp"},
		{name: "missing items", mutate: func(value *TestTaskVersion) { value.Items = nil }, want: "at least one item"},
		{name: "invalid environment key", mutate: func(value *TestTaskVersion) { value.RequiredEnvironmentKeys = []string{" "} }, want: "environment keys"},
		{name: "missing item id", mutate: func(value *TestTaskVersion) { value.Items[0].ID = " " }, want: "item 1 id"},
		{name: "duplicate item id", mutate: func(value *TestTaskVersion) {
			second := value.Items[0]
			second.SequenceNumber = 2
			value.Items = append(value.Items, second)
		}, want: "duplicate item id"},
		{name: "wrong item owner", mutate: func(value *TestTaskVersion) { value.Items[0].TestTaskVersionID = "other" }, want: "another version"},
		{name: "noncontiguous sequence", mutate: func(value *TestTaskVersion) { value.Items[0].SequenceNumber = 2 }, want: "contiguous"},
		{name: "missing workflow id", mutate: func(value *TestTaskVersion) { value.Items[0].WorkflowID = " " }, want: "workflow id"},
		{name: "invalid workflow policy", mutate: func(value *TestTaskVersion) { value.Items[0].VersionPolicy = "UNKNOWN" }, want: "unsupported workflow version policy"},
		{name: "fixed policy missing version", mutate: func(value *TestTaskVersion) { value.Items[0].VersionPolicy = WorkflowVersionFixed }, want: "fixed version id"},
		{name: "latest policy persists version", mutate: func(value *TestTaskVersion) { value.Items[0].WorkflowVersionID = "workflow-v1" }, want: "latest policy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := cloneTestTaskVersion(base)
			test.mutate(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestTestTaskAggregateValidateHistoryRuleMatrix(t *testing.T) {
	plan := validTestTaskVersionPlan()
	base, err := NewTestTask(plan.Task, plan.Version)
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
	twoVersions.Versions = []TestTaskVersion{cloneTestTaskVersion(base.Current), cloneTestTaskVersion(versionTwo)}

	tests := []struct {
		name   string
		mutate func(*TestTaskAggregate)
		want   string
	}{
		{name: "missing history", mutate: func(value *TestTaskAggregate) { value.Versions = nil }, want: "requires version history"},
		{name: "version belongs elsewhere", mutate: func(value *TestTaskAggregate) { value.Versions[0].TestTaskID = "other" }, want: "another task"},
		{name: "duplicate identity", mutate: func(value *TestTaskAggregate) {
			value.Versions = append(value.Versions, cloneTestTaskVersion(value.Versions[0]))
		}, want: "duplicate version identity"},
		{name: "noncontiguous versions", mutate: func(value *TestTaskAggregate) {
			value.Versions[1].VersionNumber = 3
			value.Current.VersionNumber = 3
		}, want: "contiguous"},
		{name: "version one has source", mutate: func(value *TestTaskAggregate) { value.Versions[0].SourceVersionID = "source" }, want: "version 1"},
		{name: "later version has missing source", mutate: func(value *TestTaskAggregate) { value.Versions[1].SourceVersionID = "missing" }, want: "source must be an earlier version"},
		{name: "current content mismatch", mutate: func(value *TestTaskAggregate) { value.Current.Items[0].WorkflowID = "changed" }, want: "content must match history"},
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
		mutate func(*TestTaskVersionPlan)
		want   string
	}{
		{name: "invalid task", mutate: func(plan *TestTaskVersionPlan) { plan.Task.ID = " " }, want: "test task id"},
		{name: "invalid version", mutate: func(plan *TestTaskVersionPlan) { plan.Version.ID = " " }, want: "version id"},
		{name: "inconsistent candidate identity", mutate: func(plan *TestTaskVersionPlan) { plan.Task.ID = "other" }, want: "candidate identity"},
		{name: "version one carries expected revision", mutate: func(plan *TestTaskVersionPlan) { plan.ExpectedTaskRevision = 1 }, want: "without a source version"},
		{name: "invalid workflow dependency", mutate: func(plan *TestTaskVersionPlan) { plan.Workflows[0].Version.WorkflowID = "other" }, want: "workflow dependency snapshot identity"},
		{name: "duplicate workflow dependency", mutate: func(plan *TestTaskVersionPlan) { plan.Workflows = append(plan.Workflows, plan.Workflows[0]) }, want: "duplicate workflow dependency"},
		{name: "invalid node dependency", mutate: func(plan *TestTaskVersionPlan) { plan.Nodes = []NodeDependencySnapshot{{}} }, want: "node dependency snapshot identity"},
		{name: "duplicate node dependency", mutate: func(plan *TestTaskVersionPlan) {
			node := versionedNodeAggregate()
			dependency := NodeDependencySnapshot{Node: node.Node, Version: node.Current}
			plan.Nodes = []NodeDependencySnapshot{dependency, dependency}
		}, want: "duplicate node dependency"},
		{name: "invalid item parameter", mutate: func(plan *TestTaskVersionPlan) {
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
	plan.Version.Items[0].VersionPolicy = WorkflowVersionFixed
	plan.Version.Items[0].WorkflowVersionID = matching.Version.ID
	matching.ResolvedFromLatest = false
	unrelated := matching
	unrelated.Workflow.ID = "unrelated"
	unrelated.Workflow.CurrentVersionID = "unrelated-v1"
	unrelated.Version.ID = "unrelated-v1"
	unrelated.Version.WorkflowID = unrelated.Workflow.ID
	plan.Workflows = []WorkflowDependencySnapshot{unrelated, matching}
	second := plan.Version.Items[0]
	second.ID = "item-two"
	second.SequenceNumber = 2
	second.WorkflowID = unrelated.Workflow.ID
	second.WorkflowVersionID = unrelated.Version.ID
	second.Parameters = map[string]parameter.Value{}
	plan.Version.Items = append(plan.Version.Items, second)

	if err := plan.Validate(); err != nil {
		t.Fatalf("fixed dependency after unrelated snapshot rejected: %v", err)
	}
}
