package automation

import (
	"math"
	"strings"
	"testing"
)

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
	base.Workflow.UpdatedAt = base.Workflow.CreatedAt
	base.Current.CreatedAt = base.Workflow.CreatedAt
	created, err := NewWorkflow(base.Workflow, base.Current)
	if err != nil {
		t.Fatalf("NewWorkflow: %v", err)
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
	if created.Workflow.Revision != 1 || updated.Workflow.Revision != 2 || deleted.Workflow.Revision != 3 || restored.Workflow.Revision != 4 {
		t.Fatalf("unexpected workflow revisions")
	}
	if created.Workflow.DisplayName == updated.Workflow.DisplayName || restored.Workflow.DeletedAt != 0 {
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
	base.Workflow.Revision = Revision(math.MaxUint64)
	if _, err := base.Delete(3); err == nil {
		t.Fatal("revision overflow accepted")
	}
}

func TestNewTestTaskCreatesValidImmutableAggregate(t *testing.T) {
	plan := validTestTaskVersionPlan()
	plan.Task.CurrentVersionID = "ignored"
	plan.Task.Revision = 99
	created, err := NewTestTask(plan.Task, plan.Version)
	if err != nil {
		t.Fatalf("NewTestTask: %v", err)
	}
	if err := created.Validate(); err != nil {
		t.Fatalf("created aggregate invalid: %v", err)
	}
	plan.Version.Items[0].Parameters["key"] = "mutated"
	if created.Task.Revision != 1 || created.Task.CurrentVersionID != plan.Version.ID || created.Current.VersionNumber != 1 {
		t.Fatalf("invalid created task: %#v", created)
	}
	if _, exists := created.Current.Items[0].Parameters["key"]; exists {
		t.Fatal("created task aliases input parameters")
	}
}

func TestNewTestTaskRejectsInvalidCreation(t *testing.T) {
	plan := validTestTaskVersionPlan()
	plan.Version.CreatedAt = 2
	if _, err := NewTestTask(plan.Task, plan.Version); err == nil || !strings.Contains(err.Error(), "timestamps") {
		t.Fatalf("creation error = %v", err)
	}
}

func TestEnvironmentRejectsCredentialBearingConfiguration(t *testing.T) {
	cases := []Environment{
		{ID: "env", DisplayName: "环境", BaseURL: "https://user:password@example.test", Variables: Properties{}, Properties: Properties{}},
		{ID: "env", DisplayName: "环境", Variables: Properties{"PASSWORD": "plain-text"}, Properties: Properties{}},
		{ID: "env", DisplayName: "环境", Variables: Properties{"Api_Key": "plain-text"}, Properties: Properties{}},
		{ID: "env", DisplayName: "环境", Variables: Properties{}, Properties: Properties{"client_secret": "plain-text"}},
		{ID: "env", DisplayName: "环境", Variables: Properties{}, Properties: Properties{}, CredentialReferences: map[string]CredentialReference{"login": {Provider: "", Key: "browser/login"}}},
	}
	for _, environment := range cases {
		if err := environment.Validate(); err == nil {
			t.Fatalf("credential-bearing environment accepted: %#v", environment)
		}
	}
}

func TestNewEnvironmentOwnsCredentialReferences(t *testing.T) {
	input := Environment{ID: "env", DisplayName: "环境", Variables: Properties{}, Properties: Properties{}, CreatedAt: 1, UpdatedAt: 1,
		CredentialReferences: map[string]CredentialReference{"login": {Provider: "vault", Key: "browser/login"}}}
	created, err := NewEnvironment(input)
	if err != nil {
		t.Fatalf("NewEnvironment: %v", err)
	}
	input.CredentialReferences["login"] = CredentialReference{Provider: "mutated", Key: "mutated"}
	if created.CredentialReferences["login"].Provider != "vault" {
		t.Fatal("environment aliases credential references")
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
	plan.ExpectedTaskRevision = 1
	if err := plan.Validate(); err != nil {
		t.Fatalf("revision-backed publication rejected: %v", err)
	}
}
