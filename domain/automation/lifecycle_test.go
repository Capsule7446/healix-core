package automation

import (
	"github.com/Capsule7446/healix-core/domain/parameter"
	"math"
	"strings"
	"testing"
)

func TestLifecycleDeleteRestoreValidateSourceAndTimeBoundaries(t *testing.T) {
	t.Run("environment", func(t *testing.T) {
		base, err := NewEnvironment(Environment{ID: "env", DisplayName: "Environment", CreatedAt: 10, UpdatedAt: 10})
		if err != nil {
			t.Fatal(err)
		}
		invalid := base
		invalid.ID = ""
		if _, err := invalid.Delete(10); err == nil || !strings.Contains(err.Error(), "environment id") {
			t.Fatalf("invalid source Delete error = %v", err)
		}
		overflow := base
		overflow.Revision = Revision(math.MaxUint64)
		if _, err := overflow.Delete(10); err == nil {
			t.Fatal("revision overflow accepted by Delete")
		}
		assertLifecycleTimeBoundaries(t, base.UpdatedAt, func(at int64) error {
			_, err := base.Delete(at)
			return err
		})
		deleted, err := base.Delete(10)
		if err != nil {
			t.Fatal(err)
		}
		deleted.ID = ""
		if _, err := deleted.Restore(11); err == nil || !strings.Contains(err.Error(), "environment id") {
			t.Fatalf("invalid source Restore error = %v", err)
		}
	})

	t.Run("node", func(t *testing.T) {
		base := versionedNodeAggregate()
		base.Node.UpdatedAt = 10
		base.Node.CreatedAt = 10
		base.Current.CreatedAt = 10
		base.Versions[0].CreatedAt = 10
		invalid := base
		invalid.Node.CurrentVersionID = "missing"
		if _, err := invalid.Delete(10); err == nil {
			t.Fatal("invalid history accepted by Delete")
		}
		assertLifecycleTimeBoundaries(t, base.Node.UpdatedAt, func(at int64) error {
			_, err := base.Delete(at)
			return err
		})
		deleted, err := base.Delete(10)
		if err != nil {
			t.Fatal(err)
		}
		deleted.Node.CurrentVersionID = "missing"
		if _, err := deleted.Restore(11); err == nil {
			t.Fatal("invalid history accepted by Restore")
		}
	})

	t.Run("workflow", func(t *testing.T) {
		base := versionedWorkflowAggregate()
		base.FlowFragment.UpdatedAt = 10
		base.FlowFragment.CreatedAt = 10
		base.Current.CreatedAt = 10
		base.Versions[0].CreatedAt = 10
		invalid := base
		invalid.FlowFragment.CurrentVersionID = "missing"
		if _, err := invalid.Delete(10); err == nil {
			t.Fatal("invalid history accepted by Delete")
		}
		assertLifecycleTimeBoundaries(t, base.FlowFragment.UpdatedAt, func(at int64) error {
			_, err := base.Delete(at)
			return err
		})
		deleted, err := base.Delete(10)
		if err != nil {
			t.Fatal(err)
		}
		deleted.FlowFragment.CurrentVersionID = "missing"
		if _, err := deleted.Restore(11); err == nil {
			t.Fatal("invalid history accepted by Restore")
		}
	})
}

func assertLifecycleTimeBoundaries(t *testing.T, updatedAt int64, transition func(int64) error) {
	t.Helper()
	for _, delta := range []int64{-1, 0, 1} {
		err := transition(updatedAt + delta)
		if delta < 0 && err == nil {
			t.Errorf("transition at UpdatedAt%+d accepted", delta)
		}
		if delta >= 0 && err != nil {
			t.Errorf("transition at UpdatedAt%+d rejected: %v", delta, err)
		}
	}
}

func TestNodeLifecycleTransitionsAreImmutableAndRevisioned(t *testing.T) {
	base := versionedNodeAggregate()
	base.Node.UpdatedAt = base.Node.CreatedAt
	base.Current.CreatedAt = base.Node.CreatedAt
	created, err := NewNode(base.Node, base.Current)
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}
	if created.Node.Revision != 1 || created.Current.VersionNumber != 1 {
		t.Fatalf("creation identities = revision %d version %d", created.Node.Revision, created.Current.VersionNumber)
	}

	properties := Properties{"owner": "updated"}
	updated, err := created.UpdateMetadata("新节点", "folder", properties, 2)
	if err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	properties["owner"] = "mutated"
	if updated.Node.Revision != 2 || updated.Node.Properties["owner"] != "updated" || created.Node.DisplayName == updated.Node.DisplayName {
		t.Fatalf("metadata update was mutable or not revisioned: %#v", updated.Node)
	}
	deleted, err := updated.Delete(3)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	restored, err := deleted.Restore(4)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if deleted.Node.Revision != 3 || restored.Node.Revision != 4 || restored.Node.DeletedAt != 0 {
		t.Fatalf("lifecycle revisions = deleted %d restored %d", deleted.Node.Revision, restored.Node.Revision)
	}
}

func TestNodeLifecycleRejectsInvalidTransitions(t *testing.T) {
	base := versionedNodeAggregate()
	cases := []struct {
		name string
		run  func(NodeAggregate) error
	}{
		{name: "stale metadata time", run: func(a NodeAggregate) error { _, err := a.UpdateMetadata("node", "", Properties{}, 0); return err }},
		{name: "restore active", run: func(a NodeAggregate) error { _, err := a.Restore(2); return err }},
		{name: "revision overflow", run: func(a NodeAggregate) error {
			a.Node.Revision = Revision(math.MaxUint64)
			_, err := a.Delete(2)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(base); err == nil {
				t.Fatal("expected transition error")
			}
		})
	}
	deleted, err := base.Delete(3)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := deleted.UpdateMetadata("node", "", Properties{}, 4); err != ErrDeletedAggregate {
		t.Fatalf("deleted update error = %v", err)
	}
	if _, err := deleted.Delete(4); err == nil {
		t.Fatal("duplicate delete accepted")
	}
}

func TestWorkflowLifecycleTransitionsAreImmutableAndRevisioned(t *testing.T) {
	base := versionedWorkflowAggregate()
	base.FlowFragment.UpdatedAt = base.FlowFragment.CreatedAt
	base.Current.CreatedAt = base.FlowFragment.CreatedAt
	created, err := NewFlowFragment(base.FlowFragment, base.Current)
	if err != nil {
		t.Fatalf("NewFlowFragment: %v", err)
	}
	updated, err := created.UpdateMetadata("新流程", "folder", Properties{"owner": "team"}, 2)
	if err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	deleted, err := updated.Delete(3)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	restored, err := deleted.Restore(4)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if created.FlowFragment.Revision != 1 || updated.FlowFragment.Revision != 2 || deleted.FlowFragment.Revision != 3 || restored.FlowFragment.Revision != 4 {
		t.Fatalf("unexpected workflow revisions")
	}
	if created.FlowFragment.DisplayName == updated.FlowFragment.DisplayName || restored.FlowFragment.DeletedAt != 0 {
		t.Fatalf("workflow lifecycle mutated receiver or failed restore")
	}
}

func TestWorkflowLifecycleRejectsInvalidTransitions(t *testing.T) {
	base := versionedWorkflowAggregate()
	if _, err := base.UpdateMetadata("workflow", "", Properties{}, 0); err == nil {
		t.Fatal("stale metadata update accepted")
	}
	if _, err := base.Restore(2); err == nil {
		t.Fatal("active workflow restore accepted")
	}
	base.FlowFragment.Revision = Revision(math.MaxUint64)
	if _, err := base.Delete(3); err == nil {
		t.Fatal("revision overflow accepted")
	}
}

func TestNewTestTaskCreatesValidImmutableAggregate(t *testing.T) {
	plan := validTestTaskVersionPlan()
	plan.Task.CurrentVersionID = "ignored"
	plan.Task.Revision = 99
	created, err := NewExecutionFlow(plan.Task, plan.Version)
	if err != nil {
		t.Fatalf("NewExecutionFlow: %v", err)
	}
	if err := created.Validate(); err != nil {
		t.Fatalf("created aggregate invalid: %v", err)
	}
	plan.Version.Items[0].Parameters["key"] = parameter.TextValue("mutated")
	if created.Task.Revision != 1 || created.Task.CurrentVersionID != plan.Version.ID || created.Current.VersionNumber != 1 {
		t.Fatalf("invalid created task: %#v", created)
	}
	if _, exists := created.Current.Items[0].Parameters["key"]; exists {
		t.Fatal("created task aliases input parameters")
	}
}

func TestTestTaskAggregatePublishVersionDerivesAuthorityAndOwnsInput(t *testing.T) {
	plan := validTestTaskVersionPlan()
	created, err := NewExecutionFlow(plan.Task, plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	publication := ExecutionFlowVersionPublication{
		ID:                      "task-v2",
		Items:                   cloneTestTaskVersion(plan.Version).Items,
		FailurePolicy:           plan.Version.FailurePolicy,
		RequiredEnvironmentKeys: append([]string(nil), plan.Version.RequiredEnvironmentKeys...),
		CreatedAt:               2,
	}

	published, err := created.PublishVersion(publication)
	if err != nil {
		t.Fatal(err)
	}
	publication.Items[0].Parameters["key"] = parameter.TextValue("mutated")
	if published.Task.Revision != 2 || published.Task.CurrentVersionID != "task-v2" ||
		published.Current.ExecutionFlowID != "task" || published.Current.VersionNumber != 2 ||
		published.Current.SourceVersionID != "task-v1" ||
		published.Current.Items[0].TestTaskVersionID != "task-v2" ||
		published.Current.Items[0].SequenceNumber != 1 {
		t.Fatalf("publication did not derive authoritative fields: %#v", published)
	}
	if _, exists := published.Current.Items[0].Parameters["key"]; exists {
		t.Fatal("published task aliases candidate parameters")
	}
	if len(created.Versions) != 1 || created.Task.Revision != 1 {
		t.Fatalf("publication mutated source aggregate: %#v", created)
	}
}

func TestNewTestTaskRejectsInvalidCreation(t *testing.T) {
	plan := validTestTaskVersionPlan()
	plan.Version.CreatedAt = 2
	if _, err := NewExecutionFlow(plan.Task, plan.Version); err == nil || !strings.Contains(err.Error(), "timestamps") {
		t.Fatalf("creation error = %v", err)
	}
}

func TestEnvironmentAcceptsAllVariableKindsAndOwnsValues(t *testing.T) {
	number, err := parameter.NewNumberValue("12.50")
	if err != nil {
		t.Fatalf("NewNumberValue: %v", err)
	}
	variables := EnvironmentVariables{
		"PASSWORD": parameter.TextValue("plain-text"),
		"count":    number,
		"enabled":  parameter.BooleanValue(true),
		"region":   parameter.SingleSelectValue("east"),
		"regions":  parameter.MultiSelectValue([]string{"east", "west"}),
	}
	environment := Environment{ID: "env", DisplayName: "环境", Variables: variables, CreatedAt: 1, UpdatedAt: 1}
	created, err := NewEnvironment(environment)
	if err != nil {
		t.Fatalf("typed variables rejected: %v", err)
	}
	variables["PASSWORD"] = parameter.TextValue("mutated")
	multi := variables["regions"].MultiSelect()
	multi[0] = "mutated"
	if created.Variables["PASSWORD"].Text() != "plain-text" || created.Variables["regions"].MultiSelect()[0] != "east" {
		t.Fatal("NewEnvironment aliases variables")
	}

	updatedInput := EnvironmentVariables{"regions": parameter.MultiSelectValue([]string{"north", "south"})}
	updated, err := created.UpdateMetadata("Updated", "https://example.test", updatedInput, 2)
	if err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	updatedInput["regions"] = parameter.MultiSelectValue([]string{"mutated"})
	if updated.Variables["regions"].MultiSelect()[0] != "north" || created.Variables["PASSWORD"].Text() != "plain-text" {
		t.Fatal("UpdateMetadata does not own variables or mutated receiver")
	}

	deleted, err := updated.Delete(3)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	deleted.Variables["regions"] = parameter.TextValue("mutated")
	if updated.Variables["regions"].MultiSelect()[0] != "north" {
		t.Fatal("Delete aliases variables")
	}
	restored, err := deleted.Restore(4)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	restored.Variables["regions"] = parameter.TextValue("restored mutation")
	if deleted.Variables["regions"].Text() != "mutated" {
		t.Fatal("Restore aliases variables")
	}

	invalid := environment
	invalid.BaseURL = "https://user:password@example.test"
	if err := invalid.Validate(); err == nil {
		t.Fatal("URL credentials accepted")
	}
}

func TestEnvironmentVariablesRejectBlankKeyAndInvalidZeroValue(t *testing.T) {
	for name, variables := range map[string]EnvironmentVariables{
		"blank key":  {" \t": parameter.TextValue("value")},
		"zero value": {"key": parameter.Value{}},
	} {
		t.Run(name, func(t *testing.T) {
			value := Environment{ID: "env", DisplayName: "Env", Variables: variables, CreatedAt: 1, UpdatedAt: 1}
			if _, err := NewEnvironment(value); err == nil {
				t.Fatal("NewEnvironment accepted invalid variables")
			}
		})
	}
}

func TestTestTaskVersionPlanUsesRevisionForPublicationConcurrency(t *testing.T) {
	plan := validTestTaskVersionPlan()
	plan.Version.ID = "task-v2"
	plan.Version.VersionNumber = 2
	plan.Version.SourceVersionID = "task-v1"
	plan.Version.Items[0].TestTaskVersionID = plan.Version.ID
	plan.Task.CurrentVersionID = plan.Version.ID
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "expected revision") {
		t.Fatalf("missing expected revision error = %v", err)
	}
	plan.ExpectedExecutionFlowRevision = 1
	if err := plan.Validate(); err != nil {
		t.Fatalf("revision-backed publication rejected: %v", err)
	}
}
