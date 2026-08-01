package scheduling

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/application/engine"
	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

type oneViewCatalog struct {
	task        automation.ExecutionFlow
	version     automation.ExecutionFlowVersion
	workflows   map[string]automation.FlowFragment
	versions    map[string]automation.FlowFragmentVersion
	nodes       map[string]automation.ElementTarget
	nodeVersion map[string]automation.ElementTargetVersion
	environment automation.Environment
}

func newOneViewResolverTx(view oneViewCatalog) *oneViewResolverTx {
	captured := view
	captured.version.Items = append([]automation.ExecutionFlowItem(nil), view.version.Items...)
	for index := range captured.version.Items {
		captured.version.Items[index].Parameters = cloneParameterValues(view.version.Items[index].Parameters)
	}
	captured.version.RequiredEnvironmentKeys = append([]string(nil), view.version.RequiredEnvironmentKeys...)
	captured.workflows = make(map[string]automation.FlowFragment, len(view.workflows))
	for id, workflow := range view.workflows {
		copy := workflow
		copy.Properties = cloneProperties(workflow.Properties)
		captured.workflows[id] = copy
	}
	captured.versions = make(map[string]automation.FlowFragmentVersion, len(view.versions))
	for id, version := range view.versions {
		copy := version
		copy.Definition.Parameters = append([]automation.ParameterDefinition(nil), version.Definition.Parameters...)
		copy.Definition.Steps = cloneCatalogSteps(version.Definition.Steps)
		captured.versions[id] = copy
	}
	captured.nodes = make(map[string]automation.ElementTarget, len(view.nodes))
	for id, node := range view.nodes {
		copy := node
		copy.Properties = cloneProperties(node.Properties)
		captured.nodes[id] = copy
	}
	captured.nodeVersion = make(map[string]automation.ElementTargetVersion, len(view.nodeVersion))
	for id, version := range view.nodeVersion {
		copy := version
		copy.Selectors = append([]fingerprint.Selector(nil), version.Selectors...)
		copy.Fingerprint.Attributes = cloneProperties(version.Fingerprint.Attributes)
		copy.Fingerprint.Path = append([]string(nil), version.Fingerprint.Path...)
		copy.Fingerprint.Framework = version.Fingerprint.Framework.Clone()
		captured.nodeVersion[id] = copy
	}
	captured.environment.Variables = view.environment.Variables.Clone()
	return &oneViewResolverTx{view: captured}
}

func cloneCatalogSteps(source []automation.FlowFragmentStep) []automation.FlowFragmentStep {
	result := make([]automation.FlowFragmentStep, len(source))
	for index, step := range source {
		result[index] = step
		result[index].Values = append([]string(nil), step.Values...)
		result[index].Children = cloneCatalogSteps(step.Children)
		if step.Reference != nil {
			copy := *step.Reference
			copy.ParameterBindings = cloneParameterBindings(step.Reference.ParameterBindings)
			result[index].Reference = &copy
		}
	}
	return result
}

func concreteChildPath(parentPath, stepID string) string {
	return parentPath + fmt.Sprintf("/%d:%s", len(stepID), stepID)
}

type oneViewResolverTx struct {
	view                  oneViewCatalog
	readToken             string
	reads                 []string
	maxDepth              int
	maxInvocations        int
	maxReferenceEdges     int
	maxResolvedValueCount int
}

func (tx *oneViewResolverTx) FindCommand(context.Context, string) (StoredCreateRunCommand, bool, error) {
	return StoredCreateRunCommand{}, false, nil
}
func (tx *oneViewResolverTx) InsertCreateRun(context.Context, CreateRunIntent) (InsertCreateRunOutcome, error) {
	return InsertCreateRunOutcome{}, errors.New("not used")
}
func (tx *oneViewResolverTx) ResolveCreateRun(_ context.Context, command CreateRunCommand) (ResolvedCreateRun, error) {
	tx.reads = append(tx.reads, tx.readToken)
	if tx.view.task.ID != command.ExecutionFlowID || tx.view.version.ID != command.TestTaskVersionID {
		return ResolvedCreateRun{}, createRunCatalogGraphUnresolvableError(errors.New("missing task/version"))
	}
	plan := automation.ResolvedExecutionFlow{Task: tx.view.task, Version: tx.view.version}
	invocations := []execution.InvocationScopeSnapshot{}
	seenDependencies := map[string]bool{}
	seenEdges := map[string]automation.FlowFragmentReferenceResolution{}
	maxDepth, maxInvocations, maxEdges, maxValues := tx.maxDepth, tx.maxInvocations, tx.maxReferenceEdges, tx.maxResolvedValueCount
	if maxDepth == 0 {
		maxDepth = execution.MaxWorkflowReferenceDepth
	}
	if maxInvocations == 0 {
		maxInvocations = execution.MaxAggregateCollectionElements
	}
	if maxEdges == 0 {
		maxEdges = execution.MaxWorkflowReferenceEdges
	}
	if maxValues == 0 {
		maxValues = execution.MaxAggregateParameters
	}
	invocationCount, edgeCount, valueCount := 0, 0, 0
	active := map[string]bool{}
	var resolveWorkflow func(string, bool, string, string, string, map[string]parameter.Value, map[string]parameter.Binding, int) error
	resolveWorkflow = func(versionID string, latest bool, path, parentPath, stepID string, values map[string]parameter.Value, bindings map[string]parameter.Binding, depth int) error {
		if depth > maxDepth {
			return createRunCatalogGraphUnresolvableError(errors.New("depth limit exceeded"))
		}
		if active[versionID] {
			return createRunCatalogGraphUnresolvableError(errors.New("cycle detected"))
		}
		if invocationCount >= maxInvocations || len(values) > maxValues-valueCount {
			return createRunCatalogGraphUnresolvableError(errors.New("invocation or resolved-value limit exceeded"))
		}
		invocationCount++
		valueCount += len(values)
		active[versionID] = true
		defer delete(active, versionID)
		version, ok := tx.view.versions[versionID]
		if !ok {
			return createRunCatalogGraphUnresolvableError(fmt.Errorf("missing %s", versionID))
		}
		workflow, ok := tx.view.workflows[version.FlowFragmentID]
		if !ok {
			return createRunCatalogGraphUnresolvableError(fmt.Errorf("missing %s", version.FlowFragmentID))
		}
		if latest && workflow.CurrentVersionID != version.ID {
			return createRunCatalogGraphUnresolvableError(errors.New("current pointer mismatch"))
		}
		if !seenDependencies[version.ID] {
			seenDependencies[version.ID] = true
			plan.Workflows = append(plan.Workflows, automation.FlowFragmentDependencySnapshot{FlowFragment: workflow, Version: version, ResolvedFromLatest: latest})
		}
		invocation := execution.InvocationScopeSnapshot{Path: path, ParentPath: parentPath, StepID: stepID, FlowFragmentID: workflow.ID, WorkflowVersionID: version.ID, ResolvedFromLatest: latest, Values: cloneParameterValues(values), Bindings: cloneParameterBindings(bindings)}
		if parentPath != "" {
			parentVersion := ""
			for _, candidate := range invocations {
				if candidate.Path == parentPath {
					parentVersion = candidate.WorkflowVersionID
				}
			}
			invocation.ParentVersionID = parentVersion
		}
		invocations = append(invocations, invocation)
		for _, step := range version.Definition.Steps {
			if step.ElementTargetID != "" {
				node, nodeOK := tx.view.nodes[step.ElementTargetID]
				nodeVersion, versionOK := tx.view.nodeVersion[step.ElementTargetVersionID]
				if !nodeOK || !versionOK || nodeVersion.ElementTargetID != node.ID {
					return createRunCatalogGraphUnresolvableError(errors.New("node or version missing"))
				}
				found := false
				for _, existing := range plan.Nodes {
					if existing.Version.ID == nodeVersion.ID {
						found = true
					}
				}
				if !found {
					plan.Nodes = append(plan.Nodes, automation.ElementTargetDependencySnapshot{ElementTarget: node, Version: nodeVersion})
				}
			}
			if step.Reference == nil {
				continue
			}
			if edgeCount >= maxEdges {
				return createRunCatalogGraphUnresolvableError(errors.New("reference-edge limit exceeded"))
			}
			edgeCount++
			targetID := step.Reference.WorkflowVersionID
			fromLatest := step.Reference.LatestPublished
			if fromLatest {
				target, ok := tx.view.workflows[step.Reference.FlowFragmentID]
				if !ok || target.CurrentVersionID == "" {
					return createRunCatalogGraphUnresolvableError(errors.New("missing current pointer"))
				}
				targetID = target.CurrentVersionID
			}
			key := version.ID + "\x00" + step.ID
			resolution := automation.FlowFragmentReferenceResolution{ParentFlowFragmentVersionID: version.ID, StepID: step.ID, FlowFragmentID: step.Reference.FlowFragmentID, WorkflowVersionID: targetID, ResolvedFromLatest: fromLatest}
			if existing, exists := seenEdges[key]; exists && existing != resolution {
				return createRunCatalogGraphUnresolvableError(errors.New("conflicting duplicate edge"))
			}
			if _, exists := seenEdges[key]; !exists {
				seenEdges[key] = resolution
				plan.References = append(plan.References, resolution)
			}
			childVersion, ok := tx.view.versions[targetID]
			if !ok {
				return createRunCatalogGraphUnresolvableError(errors.New("missing exact version"))
			}
			resolvedValues := map[string]parameter.Value{}
			for name, binding := range step.Reference.ParameterBindings {
				value, err := binding.Resolve(values)
				if err != nil {
					return createRunCatalogGraphUnresolvableError(err)
				}
				resolvedValues[name] = value
			}
			for _, definition := range childVersion.Definition.Parameters {
				if _, ok := resolvedValues[definition.Name]; !ok {
					if value, present := definition.Default.Value(); present {
						resolvedValues[definition.Name] = value
					}
				}
			}
			childPath := concreteChildPath(path, step.ID)
			if err := resolveWorkflow(targetID, fromLatest, childPath, path, step.ID, resolvedValues, step.Reference.ParameterBindings, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, item := range tx.view.version.Items {
		workflow, ok := tx.view.workflows[item.FlowFragmentID]
		if !ok {
			return ResolvedCreateRun{}, createRunCatalogGraphUnresolvableError(errors.New("missing root"))
		}
		versionID, latest := item.WorkflowVersionID, item.VersionPolicy == automation.FlowFragmentVersionLatest
		if latest {
			if workflow.CurrentVersionID == "" {
				return ResolvedCreateRun{}, createRunCatalogGraphUnresolvableError(errors.New("missing current pointer"))
			}
			versionID = workflow.CurrentVersionID
		}
		version := tx.view.versions[versionID]
		values, err := automation.ResolveParameterValues(version.Definition.Parameters, command.Entries[item.ID])
		if err != nil {
			return ResolvedCreateRun{}, createRunCatalogGraphUnresolvableError(err)
		}
		if err := resolveWorkflow(versionID, latest, concreteRootPath(command.RunID.String(), item.ID), "", "", values, nil, 0); err != nil {
			return ResolvedCreateRun{}, err
		}
	}
	return ResolvedCreateRun{Plan: plan, Environment: tx.view.environment, Invocations: invocations}, nil
}

func catalogFromMapperSource() oneViewCatalog {
	source := validMapperSource()
	catalog := oneViewCatalog{task: source.Publication.Task, version: source.Publication.Version, workflows: map[string]automation.FlowFragment{}, versions: map[string]automation.FlowFragmentVersion{}, nodes: map[string]automation.ElementTarget{}, nodeVersion: map[string]automation.ElementTargetVersion{}, environment: automation.Environment{ID: "env", DisplayName: "Environment", BaseURL: "https://example.test", Variables: automation.EnvironmentVariables{"Region": parameter.TextValue("east")}, Revision: 1}}
	for _, item := range source.Publication.Workflows {
		catalog.workflows[item.FlowFragment.ID] = item.FlowFragment
		catalog.versions[item.Version.ID] = item.Version
	}
	for _, item := range source.Publication.Nodes {
		catalog.nodes[item.ElementTarget.ID] = item.ElementTarget
		catalog.nodeVersion[item.Version.ID] = item.Version
	}
	return catalog
}

func TestConcreteChildPathIsCollisionProofForAdversarialStepIDs(t *testing.T) {
	paths := map[string]bool{}
	for _, stepID := range []string{"a/b", "1:a", "a\x00b", "", "/", "2:ab"} {
		path := concreteChildPath("4:root", stepID)
		if paths[path] {
			t.Fatalf("path collision for %q: %q", stepID, path)
		}
		paths[path] = true
	}
	if concreteChildPath("4:root", "a/b") == concreteChildPath("4:root/1:a", "b") {
		t.Fatal("recursive path boundaries collided")
	}
}

func TestOneViewResolverCapturesCatalogAtTransactionStart(t *testing.T) {
	source := catalogFromMapperSource()
	tx := newOneViewResolverTx(source)
	workflow := source.versions["child-v1"]
	workflow.Definition.Steps[0].DisplayName = "source mutation"
	source.versions[workflow.ID] = workflow
	source.environment.Variables["Region"] = parameter.TextValue("west")
	resolved, err := tx.ResolveCreateRun(context.Background(), validCreateRunCommand())
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Environment.Variables["Region"].Text() != "east" || resolved.Plan.Workflows[1].Version.Definition.Steps[0].DisplayName == "source mutation" {
		t.Fatal("transaction observed mixed catalog revisions")
	}
}

func TestOneViewResolverResolvesLatestFixedNodesAndRepeatedPaths(t *testing.T) {
	command := validCreateRunCommand()
	tx := newOneViewResolverTx(catalogFromMapperSource())
	tx.readToken = "tx-1"
	resolved, err := tx.ResolveCreateRun(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if len(tx.reads) != 1 || tx.reads[0] != "tx-1" || len(resolved.Invocations) != 4 || len(resolved.Plan.Nodes) != 1 {
		t.Fatalf("resolution=%#v reads=%#v", resolved, tx.reads)
	}
	if !resolved.Invocations[0].ResolvedFromLatest || !resolved.Invocations[1].ResolvedFromLatest || resolved.Invocations[1].Path == resolved.Invocations[3].Path {
		t.Fatal("latest provenance or concrete paths lost")
	}
	if _, err := BuildRunSnapshot(command, resolved); err != nil {
		t.Fatal(err)
	}
}

func TestOneViewResolverRejectsCatalogGraphFailuresWithTypedErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*oneViewCatalog)
	}{
		{"missing root", func(c *oneViewCatalog) { delete(c.workflows, "root") }},
		{"missing current", func(c *oneViewCatalog) { w := c.workflows["root"]; w.CurrentVersionID = ""; c.workflows["root"] = w }},
		{"missing workflow version", func(c *oneViewCatalog) { delete(c.versions, "root-v1") }},
		{"missing node", func(c *oneViewCatalog) { delete(c.nodes, "660e8400-e29b-41d4-a716-446655440000") }},
		{"missing node version", func(c *oneViewCatalog) { delete(c.nodeVersion, "550e8400-e29b-41d4-a716-446655440000") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			view := catalogFromMapperSource()
			test.mutate(&view)
			_, err := (newOneViewResolverTx(view)).ResolveCreateRun(context.Background(), validCreateRunCommand())
			if !fault.IsCode(err, CodeCreateInstanceCatalogGraphUnresolvable) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestOneViewResolverPreservesFixedNestedReference(t *testing.T) {
	view := catalogFromMapperSource()
	root := view.versions["root-v1"]
	root.Definition.Steps[0].Reference.LatestPublished = false
	root.Definition.Steps[0].Reference.WorkflowVersionID = "child-v1"
	view.versions[root.ID] = root
	resolved, err := (newOneViewResolverTx(view)).ResolveCreateRun(context.Background(), validCreateRunCommand())
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Plan.References) != 1 || resolved.Plan.References[0].ResolvedFromLatest || resolved.Invocations[1].ResolvedFromLatest {
		t.Fatalf("fixed provenance drifted: %#v", resolved.Plan.References)
	}
}

func TestOneViewResolverRejectsConflictingDuplicateEdgeResolution(t *testing.T) {
	view := catalogFromMapperSource()
	root := view.versions["root-v1"]
	duplicate := root.Definition.Steps[0]
	duplicate.Reference = &automation.FlowFragmentReference{FlowFragmentID: "child", WorkflowVersionID: "child-v1"}
	root.Definition.Steps = append(root.Definition.Steps, duplicate)
	view.versions[root.ID] = root
	_, err := newOneViewResolverTx(view).ResolveCreateRun(context.Background(), validCreateRunCommand())
	if !fault.IsCode(err, CodeCreateInstanceCatalogGraphUnresolvable) {
		t.Fatalf("error=%v", err)
	}
}

func TestOneViewResolverRejectsCycleAndConfiguredLimits(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*oneViewResolverTx)
	}{
		{"cycle", func(tx *oneViewResolverTx) {
			child := tx.view.versions["child-v1"]
			child.Definition.Steps = []automation.FlowFragmentStep{{ID: "back", DisplayName: "Back", Kind: automation.StepFlowFragmentRef, Reference: &automation.FlowFragmentReference{FlowFragmentID: "root", LatestPublished: true}}}
			tx.view.versions[child.ID] = child
		}},
		{"depth", func(tx *oneViewResolverTx) { tx.maxDepth = -1 }},
		{"concrete invocation", func(tx *oneViewResolverTx) { tx.maxInvocations = 1 }},
		{"reference edge", func(tx *oneViewResolverTx) {
			tx.maxReferenceEdges = 1
			root := tx.view.versions["root-v1"]
			root.Definition.Steps = append(root.Definition.Steps, automation.FlowFragmentStep{ID: "call-child-2", DisplayName: "Call child 2", Kind: automation.StepFlowFragmentRef, Reference: &automation.FlowFragmentReference{FlowFragmentID: "child", LatestPublished: true}})
			tx.view.versions[root.ID] = root
		}},
		{"resolved values", func(tx *oneViewResolverTx) {
			tx.maxResolvedValueCount = 1
			root := tx.view.versions["root-v1"]
			root.Definition.Parameters = []automation.ParameterDefinition{{Name: "a", DisplayName: "A", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue("a"))}, {Name: "b", DisplayName: "B", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue("b"))}}
			tx.view.versions[root.ID] = root
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := newOneViewResolverTx(catalogFromMapperSource())
			test.configure(tx)
			_, err := tx.ResolveCreateRun(context.Background(), validCreateRunCommand())
			if !fault.IsCode(err, CodeCreateInstanceCatalogGraphUnresolvable) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestOneViewResolverSeparatesRetryableAdapterErrors(t *testing.T) {
	cause := errors.New("serialization")
	err := fmt.Errorf("transaction read: %w", createRunRetryableError(cause))
	if !fault.IsCode(err, CodeCreateInstanceRetryable) || !errors.Is(err, cause) || fault.IsCode(err, CodeCreateInstanceCatalogGraphUnresolvable) {
		t.Fatalf("error category drifted: %v", err)
	}
}

func TestSealedResolverOutputIgnoresLaterCatalogMutation(t *testing.T) {
	command := validCreateRunCommand()
	tx := newOneViewResolverTx(catalogFromMapperSource())
	resolved, err := tx.ResolveCreateRun(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := BuildRunSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	digest := snapshot.Digest()
	compiledBefore, err := engine.CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	tx.view.environment.Variables["Region"] = parameter.TextValue("west")
	node := tx.view.nodes["660e8400-e29b-41d4-a716-446655440000"]
	node.DisplayName = "mutated"
	tx.view.nodes[node.ID] = node
	workflow := tx.view.versions["child-v1"]
	workflow.Definition.Steps[0].DisplayName = "mutated"
	tx.view.versions[workflow.ID] = workflow
	compiledAfter, err := engine.CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Digest() != digest || snapshot.Environment().Variables["Region"].Text() != "east" || !reflect.DeepEqual(compiledBefore, compiledAfter) {
		t.Fatal("sealed resolver output or compilation changed after catalog mutation")
	}
}

var _ CreateRunTx = (*oneViewResolverTx)(nil)
var _ = parameter.TextValue
