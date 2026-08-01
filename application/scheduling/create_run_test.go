package scheduling

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func validCreateRunCommand() CreateRunCommand {
	return CreateRunCommand{CommandID: "command-1", RunID: mustInstanceID("run-1"), ExecutionFlowID: "task", TestTaskVersionID: "task-v1", EnvironmentID: "env", Entries: map[string]map[string]parameter.Value{"item-1": {}, "item-2": {}}, FailurePolicy: execution.FailurePolicyContinueOnFailure, CreatedAt: 10, ScreenshotPolicy: execution.ScreenshotPolicySnapshot{Version: execution.ScreenshotPolicyV1, Enabled: true, Destination: "artifacts"}, HealerPolicy: execution.DefaultHealerPolicySnapshot()}
}

func validResolvedCreateRun(t *testing.T, command CreateRunCommand) ResolvedCreateRun {
	t.Helper()
	source := validMapperSource()
	source.RunID = command.RunID
	roots := make([]execution.InvocationScopeSnapshot, 0, len(source.Entries)*2)
	for _, entry := range source.Entries {
		entry.ExecutionID = mustEntryID(concreteRootPath(command.RunID.String(), entry.TestTaskItemID))
		roots = append(roots,
			execution.InvocationScopeSnapshot{Path: execution.RootInvocationPath(entry.ExecutionID), FlowFragmentID: entry.FlowFragmentID, WorkflowVersionID: entry.WorkflowVersionID, Values: map[string]parameter.Value{}},
			execution.InvocationScopeSnapshot{Path: mustInvocationPath(entry.ExecutionID.String() + "/10:call-child"), ParentPath: execution.RootInvocationPath(entry.ExecutionID), ParentVersionID: "root-v1", StepID: "call-child", FlowFragmentID: "child", WorkflowVersionID: "child-v1", ResolvedFromLatest: true, Values: map[string]parameter.Value{}, Bindings: map[string]parameter.Binding{}},
		)
	}
	return ResolvedCreateRun{Plan: source.Publication, Environment: automation.Environment{ID: "env", DisplayName: "Environment", BaseURL: "https://example.test", Variables: automation.EnvironmentVariables{"Region": parameter.TextValue("east")}, Revision: 1}, Invocations: roots}
}

func TestCreateRunRequestDigestMatrix(t *testing.T) {
	base := validCreateRunCommand()
	base.Entries = map[string]map[string]parameter.Value{"item": {"value": parameter.TextValue("x")}}
	digest, err := CreateRunRequestDigest(base)
	if err != nil || len(digest) != 71 || !strings.HasPrefix(digest, "sha256:") || digest != strings.ToLower(digest) {
		t.Fatalf("digest=%q err=%v", digest, err)
	}
	again, _ := CreateRunRequestDigest(base)
	if again != digest {
		t.Fatal("digest is unstable")
	}
	commandOnly := base
	commandOnly.CommandID = "other"
	assertSameDigest(t, base, commandOnly)
	nilValues := base
	nilValues.Entries = map[string]map[string]parameter.Value{"item": nil}
	emptyValues := base
	emptyValues.Entries = map[string]map[string]parameter.Value{"item": {}}
	assertSameDigest(t, nilValues, emptyValues)
	variants := []CreateRunCommand{}
	add := func(edit func(*CreateRunCommand)) {
		changed := base
		changed.Entries = map[string]map[string]parameter.Value{"item": {"value": parameter.TextValue("x")}}
		edit(&changed)
		variants = append(variants, changed)
	}
	add(func(v *CreateRunCommand) { v.RunID = mustInstanceID("other") })
	add(func(v *CreateRunCommand) { v.ExecutionFlowID = "other" })
	add(func(v *CreateRunCommand) { v.TestTaskVersionID = "other" })
	add(func(v *CreateRunCommand) { v.EnvironmentID = "other" })
	add(func(v *CreateRunCommand) { v.CreatedAt++ })
	add(func(v *CreateRunCommand) { v.FailurePolicy = execution.FailurePolicyStopOnFailure })
	add(func(v *CreateRunCommand) { v.ScreenshotPolicy.Enabled = !v.ScreenshotPolicy.Enabled })
	add(func(v *CreateRunCommand) { v.ScreenshotPolicy.Destination = "other" })
	add(func(v *CreateRunCommand) { v.HealerPolicy.ReviewCap += .01 })
	add(func(v *CreateRunCommand) { v.HealerPolicy.AppliedCap += .01 })
	weightEdits := []func(*execution.HealerWeightsSnapshot){func(w *execution.HealerWeightsSnapshot) { w.Tag += .01 }, func(w *execution.HealerWeightsSnapshot) { w.ID += .01 }, func(w *execution.HealerWeightsSnapshot) { w.RoleName += .01 }, func(w *execution.HealerWeightsSnapshot) { w.Class += .01 }, func(w *execution.HealerWeightsSnapshot) { w.Attrs += .01 }, func(w *execution.HealerWeightsSnapshot) { w.Text += .01 }, func(w *execution.HealerWeightsSnapshot) { w.Index += .01 }, func(w *execution.HealerWeightsSnapshot) { w.Neighbor += .01 }, func(w *execution.HealerWeightsSnapshot) { w.LabelText += .01 }, func(w *execution.HealerWeightsSnapshot) { w.Container += .01 }}
	for _, edit := range weightEdits {
		edit := edit
		add(func(v *CreateRunCommand) { edit(&v.HealerPolicy.Weights) })
	}
	add(func(v *CreateRunCommand) {
		v.Entries = map[string]map[string]parameter.Value{"other": {"value": parameter.TextValue("x")}}
	})
	add(func(v *CreateRunCommand) { v.Entries["item"]["value"] = parameter.TextValue("y") })
	for index, variant := range variants {
		assertDifferentDigest(t, base, variant, index)
	}
}

func TestCreateRunRequestDigestPreservesTypedAndMultiBoundaries(t *testing.T) {
	base := validCreateRunCommand()
	base.Entries = map[string]map[string]parameter.Value{"item": {"value": parameter.TextValue("true")}}
	values := []parameter.Value{parameter.BooleanValue(true), parameter.SingleSelectValue("true"), parameter.MultiSelectValue([]string{"true"})}
	number, _ := parameter.NewNumberValue("1")
	values = append(values, number)
	for index, value := range values {
		changed := base
		changed.Entries = map[string]map[string]parameter.Value{"item": {"value": value}}
		assertDifferentDigest(t, base, changed, index)
	}
	left := base
	left.Entries = map[string]map[string]parameter.Value{"item": {"value": parameter.MultiSelectValue([]string{"a", "bc"})}}
	right := base
	right.Entries = map[string]map[string]parameter.Value{"item": {"value": parameter.MultiSelectValue([]string{"ab", "c"})}}
	assertDifferentDigest(t, left, right, 100)
	reordered := base
	reordered.Entries = map[string]map[string]parameter.Value{"item": {"value": parameter.MultiSelectValue([]string{"bc", "a"})}}
	assertDifferentDigest(t, left, reordered, 101)
	empty := base
	empty.Entries = map[string]map[string]parameter.Value{"item": {"value": parameter.TextValue("")}}
	missing := base
	missing.Entries = map[string]map[string]parameter.Value{"item": {}}
	assertDifferentDigest(t, empty, missing, 102)
}

func assertSameDigest(t *testing.T, left, right CreateRunCommand) {
	t.Helper()
	a, ea := CreateRunRequestDigest(left)
	b, eb := CreateRunRequestDigest(right)
	if ea != nil || eb != nil || a != b {
		t.Fatalf("digests differ: %q/%v %q/%v", a, ea, b, eb)
	}
}
func assertDifferentDigest(t *testing.T, left, right CreateRunCommand, index int) {
	t.Helper()
	a, ea := CreateRunRequestDigest(left)
	b, eb := CreateRunRequestDigest(right)
	if ea != nil || eb != nil || a == b {
		t.Fatalf("variant %d collided: %q/%v %q/%v", index, a, ea, b, eb)
	}
}

func TestCreateRunRequestDigestRejectsInvalidBeforeHashing(t *testing.T) {
	tests := []func(*CreateRunCommand){
		func(v *CreateRunCommand) { v.RunID = execution.InstanceID{} },
		func(v *CreateRunCommand) { v.FailurePolicy = "UNKNOWN" },
		func(v *CreateRunCommand) { v.ScreenshotPolicy.Version++ },
		func(v *CreateRunCommand) { v.HealerPolicy.Version++ },
		func(v *CreateRunCommand) { v.Entries["item-1"] = map[string]parameter.Value{"bad": {}} },
	}
	for index, edit := range tests {
		command := validCreateRunCommand()
		edit(&command)
		if digest, err := CreateRunRequestDigest(command); digest != "" || !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
			t.Fatalf("case %d digest=%q err=%v", index, digest, err)
		}
	}
}

func TestCreateRunRequestDigestIsCanonicalAndTyped(t *testing.T) {
	left := validCreateRunCommand()
	right := validCreateRunCommand()
	right.CommandID = "other-command"
	right.Entries = map[string]map[string]parameter.Value{"item-2": {}, "item-1": {}}
	leftDigest, err := CreateRunRequestDigest(left)
	if err != nil {
		t.Fatal(err)
	}
	rightDigest, err := CreateRunRequestDigest(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest != rightDigest {
		t.Fatalf("canonical digest mismatch: %q %q", leftDigest, rightDigest)
	}
	right.Entries["item-1"] = map[string]parameter.Value{"value": parameter.TextValue("true")}
	textDigest, _ := CreateRunRequestDigest(right)
	right.Entries["item-1"] = map[string]parameter.Value{"value": parameter.BooleanValue(true)}
	boolDigest, _ := CreateRunRequestDigest(right)
	if textDigest == boolDigest {
		t.Fatal("typed values collided")
	}
}

func TestBuildRunSnapshotRejectsResolverValueAndBindingDrift(t *testing.T) {
	command := validCreateRunCommand()
	tests := []struct {
		name   string
		mutate func(*CreateRunCommand, *ResolvedCreateRun)
	}{
		{"unknown root input", func(c *CreateRunCommand, _ *ResolvedCreateRun) {
			c.Entries["item-1"] = map[string]parameter.Value{"unknown": parameter.TextValue("x")}
		}},
		{"root invocation drift", func(_ *CreateRunCommand, r *ResolvedCreateRun) {
			r.Invocations[0].Values["unknown"] = parameter.TextValue("x")
		}},
		{"child extra value", func(_ *CreateRunCommand, r *ResolvedCreateRun) {
			r.Invocations[1].Values["extra"] = parameter.TextValue("x")
		}},
		{"child binding drift", func(_ *CreateRunCommand, r *ResolvedCreateRun) {
			r.Invocations[1].Bindings["extra"] = parameter.LiteralBinding(parameter.TextValue("x"))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := command
			current.Entries = map[string]map[string]parameter.Value{"item-1": {}, "item-2": {}}
			resolved := validResolvedCreateRun(t, current)
			test.mutate(&current, &resolved)
			if _, err := BuildRunSnapshot(current, resolved); err == nil {
				t.Fatal("resolver drift accepted")
			}
		})
	}
}

func TestBuildRunSnapshotResolvesRootDefaultsAndRejectsValueDrift(t *testing.T) {
	tests := []struct {
		name       string
		definition automation.ParameterDefinition
		supplied   map[string]parameter.Value
		resolved   map[string]parameter.Value
		wantError  bool
		wantText   string
	}{
		{name: "optional empty text default remains present", definition: automation.ParameterDefinition{Name: "value", DisplayName: "Value", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue(""))}, supplied: map[string]parameter.Value{}, resolved: map[string]parameter.Value{"value": parameter.TextValue("")}, wantText: ""},
		{name: "explicit empty text overrides default", definition: automation.ParameterDefinition{Name: "value", DisplayName: "Value", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue("fallback"))}, supplied: map[string]parameter.Value{"value": parameter.TextValue("")}, resolved: map[string]parameter.Value{"value": parameter.TextValue("")}, wantText: ""},
		{name: "required root missing", definition: automation.ParameterDefinition{Name: "value", DisplayName: "Value", Type: parameter.Text, Required: true}, supplied: map[string]parameter.Value{}, resolved: map[string]parameter.Value{}, wantError: true},
		{name: "root type mismatch", definition: automation.ParameterDefinition{Name: "value", DisplayName: "Value", Type: parameter.Boolean}, supplied: map[string]parameter.Value{"value": parameter.TextValue("true")}, resolved: map[string]parameter.Value{"value": parameter.TextValue("true")}, wantError: true},
		{name: "root select option violation", definition: automation.ParameterDefinition{Name: "value", DisplayName: "Value", Type: parameter.SingleSelect, Options: []string{"east"}}, supplied: map[string]parameter.Value{"value": parameter.SingleSelectValue("west")}, resolved: map[string]parameter.Value{"value": parameter.SingleSelectValue("west")}, wantError: true},
		{name: "resolver root value drift", definition: automation.ParameterDefinition{Name: "value", DisplayName: "Value", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue("expected"))}, supplied: map[string]parameter.Value{}, resolved: map[string]parameter.Value{"value": parameter.TextValue("different")}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := validCreateRunCommand()
			command.Entries["item-1"] = test.supplied
			resolved := validResolvedCreateRun(t, command)
			resolved.Plan.Workflows[0].Version.Definition.Parameters = []automation.ParameterDefinition{test.definition}
			resolved.Invocations[0].Values = test.resolved
			secondValues, resolveErr := automation.ResolveParameterValues([]automation.ParameterDefinition{test.definition}, command.Entries["item-2"])
			if resolveErr == nil {
				resolved.Invocations[2].Values = secondValues
			}
			snapshot, err := BuildRunSnapshot(command, resolved)
			if test.wantError {
				if err == nil {
					t.Fatal("invalid root resolution accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			value, exists := snapshot.Plan().Entries[0].Parameters.Values["value"]
			if !exists || value.Text() != test.wantText {
				t.Fatalf("resolved root value = %#v", value)
			}
		})
	}
}

func TestBuildRunSnapshotEnforcesConcreteNestedDefaultsAndBindings(t *testing.T) {
	tests := []struct {
		name       string
		definition automation.ParameterDefinition
		binding    parameter.Binding
		value      parameter.Value
		wantError  bool
	}{
		{name: "optional default only when unbound", definition: automation.ParameterDefinition{Name: "child", DisplayName: "Child", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue("default"))}, value: parameter.TextValue("default")},
		{name: "explicit empty text overrides default", definition: automation.ParameterDefinition{Name: "child", DisplayName: "Child", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue("default"))}, binding: parameter.LiteralBinding(parameter.TextValue("")), value: parameter.TextValue("")},
		{name: "missing required child", definition: automation.ParameterDefinition{Name: "child", DisplayName: "Child", Type: parameter.Text, Required: true}, wantError: true},
		{name: "child type violation", definition: automation.ParameterDefinition{Name: "child", DisplayName: "Child", Type: parameter.Boolean}, binding: parameter.LiteralBinding(parameter.TextValue("true")), value: parameter.TextValue("true"), wantError: true},
		{name: "child select violation", definition: automation.ParameterDefinition{Name: "child", DisplayName: "Child", Type: parameter.SingleSelect, Options: []string{"east"}}, binding: parameter.LiteralBinding(parameter.SingleSelectValue("west")), value: parameter.SingleSelectValue("west"), wantError: true},
		{name: "child values versus bindings mismatch", definition: automation.ParameterDefinition{Name: "child", DisplayName: "Child", Type: parameter.Text}, binding: parameter.LiteralBinding(parameter.TextValue("bound")), value: parameter.TextValue("different"), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := validCreateRunCommand()
			resolved := validResolvedCreateRun(t, command)
			resolved.Plan.Workflows[1].Version.Definition.Parameters = []automation.ParameterDefinition{test.definition}
			for index := range resolved.Invocations {
				if resolved.Invocations[index].ParentPath == (execution.InvocationPath{}) {
					continue
				}
				resolved.Invocations[index].Bindings = map[string]parameter.Binding{}
				resolved.Invocations[index].Values = map[string]parameter.Value{}
				if test.binding.Kind() != "" {
					resolved.Invocations[index].Bindings["child"] = test.binding
				}
				if test.value.Type() != "" {
					resolved.Invocations[index].Values["child"] = test.value
				}
			}
			snapshot, err := BuildRunSnapshot(command, resolved)
			if test.wantError {
				if err == nil || snapshot.Digest() != "" {
					t.Fatalf("invalid child graph accepted: %#v", snapshot)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			child := snapshot.Input().Invocations[1]
			if !child.Values["child"].Equal(test.value) {
				t.Fatalf("child value = %#v", child.Values["child"])
			}
			if test.name == "optional default only when unbound" {
				if _, exists := child.Bindings["child"]; exists {
					t.Fatal("omitted default binding was manufactured")
				}
			}
		})
	}
}

func TestBuildRunSnapshotPreservesExactTypedParentReferenceValues(t *testing.T) {
	number, err := parameter.NewNumberValue("1.20")
	if err != nil {
		t.Fatal(err)
	}
	definitions := []automation.ParameterDefinition{
		{Name: "number", DisplayName: "Number", Type: parameter.Number, Required: true},
		{Name: "boolean", DisplayName: "Boolean", Type: parameter.Boolean, Required: true},
		{Name: "single", DisplayName: "Single", Type: parameter.SingleSelect, Required: true, Options: []string{"east"}},
		{Name: "multi", DisplayName: "Multi", Type: parameter.MultiSelect, Required: true, Options: []string{"a,b", "c"}},
	}
	values := map[string]parameter.Value{"number": number, "boolean": parameter.BooleanValue(true), "single": parameter.SingleSelectValue("east"), "multi": parameter.MultiSelectValue([]string{"a,b", "c"})}
	command := validCreateRunCommand()
	command.Entries = map[string]map[string]parameter.Value{"item-1": cloneParameterValues(values), "item-2": cloneParameterValues(values)}
	resolved := validResolvedCreateRun(t, command)
	resolved.Plan.Workflows[0].Version.Definition.Parameters = definitions
	resolved.Plan.Workflows[1].Version.Definition.Parameters = definitions
	bindings := map[string]parameter.Binding{}
	for _, definition := range definitions {
		bindings[definition.Name] = parameter.ParentReferenceBinding(definition.Name)
	}
	resolved.Plan.Workflows[0].Version.Definition.Steps[0].Reference.ParameterBindings = bindings
	for index := range resolved.Plan.Version.Items {
		resolved.Plan.Version.Items[index].Parameters = cloneParameterValues(values)
	}
	for index := range resolved.Invocations {
		resolved.Invocations[index].Values = cloneParameterValues(values)
		if resolved.Invocations[index].ParentPath != (execution.InvocationPath{}) {
			resolved.Invocations[index].Bindings = bindings
		}
	}
	snapshot, err := BuildRunSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	child := snapshot.Input().Invocations[1]
	if child.Values["number"].Type() != parameter.Number || child.Values["number"].Number() != "1.2" || child.Values["boolean"].Type() != parameter.Boolean || !child.Values["boolean"].Boolean() || child.Values["single"].Type() != parameter.SingleSelect || child.Values["single"].SingleSelect() != "east" {
		t.Fatalf("typed parent values drifted: %#v", child.Values)
	}
	multi := child.Values["multi"]
	if multi.Type() != parameter.MultiSelect || len(multi.MultiSelect()) != 2 || multi.MultiSelect()[0] != "a,b" || multi.MultiSelect()[1] != "c" {
		t.Fatalf("multi-select boundaries drifted: %#v", multi.MultiSelect())
	}
	for name, binding := range child.Bindings {
		parentName, ok := binding.ParentName()
		if !ok || parentName != name {
			t.Fatalf("binding %q drifted: %#v", name, binding)
		}
	}
}

func TestBuildRunSnapshotMapsCatalogAndDefensivelyOwnsAssets(t *testing.T) {
	command := validCreateRunCommand()
	resolved := validResolvedCreateRun(t, command)
	snapshot, err := BuildRunSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Environment.Variables["Region"] = parameter.TextValue("west")
	if snapshot.Environment().Variables["Region"].Text() != "east" || snapshot.TestTaskVersionID() != "task-v1" || len(snapshot.Plan().Entries) != 2 {
		t.Fatal("snapshot did not freeze complete resolved create data")
	}
}

func mustCreateRunService(t *testing.T, store CreateRunStore) CreateRunService {
	t.Helper()
	service, err := NewCreateRunService(store)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestNewCreateRunServiceRejectsNilStore(t *testing.T) {
	var typedNil *createRunFake
	for _, store := range []CreateRunStore{nil, typedNil} {
		if _, err := NewCreateRunService(store); err == nil {
			t.Fatal("expected configuration error")
		}
	}
}

type createRunFake struct {
	stored                    *StoredCreateRunCommand
	resolved                  ResolvedCreateRun
	resolveCalls, insertCalls int
	transactionErr            error
	findErr                   error
	resolveErr                error
	insertErr                 error
	insertOutcome             InsertCreateRunOutcome
	mutateResolvedCommand     func(CreateRunCommand)
	mutateInsertOutcome       func(*InsertCreateRunOutcome)
	transactionCalls          int
}

func (f *createRunFake) InTransaction(ctx context.Context, callback func(CreateRunTx) error) error {
	f.transactionCalls++
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.transactionErr != nil {
		return f.transactionErr
	}
	return callback(f)
}
func (f *createRunFake) FindCommand(context.Context, string) (StoredCreateRunCommand, bool, error) {
	if f.findErr != nil {
		return StoredCreateRunCommand{}, false, f.findErr
	}
	if f.stored == nil {
		return StoredCreateRunCommand{}, false, nil
	}
	return *f.stored, true, nil
}
func (f *createRunFake) ResolveCreateRun(_ context.Context, command CreateRunCommand) (ResolvedCreateRun, error) {
	f.resolveCalls++
	if f.mutateResolvedCommand != nil {
		f.mutateResolvedCommand(command)
	}
	if f.resolveErr != nil {
		return ResolvedCreateRun{}, f.resolveErr
	}
	return f.resolved, nil
}
func (f *createRunFake) InsertCreateRun(_ context.Context, intent CreateRunIntent) (InsertCreateRunOutcome, error) {
	f.insertCalls++
	if f.insertErr != nil {
		return InsertCreateRunOutcome{}, f.insertErr
	}
	if f.insertOutcome.Status != "" {
		return f.insertOutcome, nil
	}
	entries := make([]execution.EntryID, len(intent.Entries))
	for i := range intent.Entries {
		entries[i] = intent.Entries[i].ID
	}
	outcome := InsertCreateRunOutcome{Status: InsertCreateRunApplied, CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: StoredCreateRunResult{Run: intent.Run, Snapshot: intent.Snapshot, SnapshotDigest: intent.Snapshot.Digest(), EntryIDs: entries}}
	if f.mutateInsertOutcome != nil {
		f.mutateInsertOutcome(&outcome)
	}
	return outcome, nil
}

func TestCreateRunServiceOwnsCommandAcrossResolverMutation(t *testing.T) {
	command := validCreateRunCommand()
	originalDigest, err := CreateRunRequestDigest(command)
	if err != nil {
		t.Fatal(err)
	}
	fake := &createRunFake{resolved: validResolvedCreateRun(t, command), mutateResolvedCommand: func(received CreateRunCommand) {
		received.Entries["item-1"]["injected"] = parameter.TextValue("mutated")
		delete(received.Entries, "item-2")
	}}
	result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.Digest() == "" || len(result.Snapshot.Plan().Entries) != 2 {
		t.Fatalf("snapshot was desynchronized: %#v", result)
	}
	afterDigest, _ := CreateRunRequestDigest(command)
	if afterDigest != originalDigest || len(command.Entries) != 2 || len(command.Entries["item-1"]) != 0 {
		t.Fatal("caller command was mutated")
	}
}

func TestCreateRunServiceRejectsMalformedAuthoritativeOutcomes(t *testing.T) {
	command := validCreateRunCommand()
	mutations := []struct {
		name   string
		mutate func(*InsertCreateRunOutcome)
	}{
		{"zero", func(v *InsertCreateRunOutcome) { *v = InsertCreateRunOutcome{} }},
		{"wrong command", func(v *InsertCreateRunOutcome) { v.CommandID = "other" }},
		{"wrong digest", func(v *InsertCreateRunOutcome) { v.RequestDigest = "sha256:" + strings.Repeat("0", 64) }},
		{"wrong run", func(v *InsertCreateRunOutcome) { v.Result.Run.ID = mustInstanceID("other") }},
		{"empty snapshot", func(v *InsertCreateRunOutcome) { v.Result.Snapshot = execution.InstanceSnapshot{} }},
		{"wrong entries", func(v *InsertCreateRunOutcome) { v.Result.EntryIDs = []execution.EntryID{mustEntryID("other")} }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			fake := &createRunFake{resolved: validResolvedCreateRun(t, command), mutateInsertOutcome: test.mutate}
			result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), command)
			if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || !isZeroCreateRunResult(result) {
				t.Fatalf("result/error=%#v/%v", result, err)
			}
		})
	}
}

func TestCreateRunServicePreflightRejectsOversizeBeforeStore(t *testing.T) {
	command := validCreateRunCommand()
	command.ExecutionFlowID = strings.Repeat("x", execution.MaxStringBytes+1)
	fake := &createRunFake{resolved: validResolvedCreateRun(t, validCreateRunCommand())}
	result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), command)
	if !fault.IsCode(err, CodeCreateInstanceCommandInvalid) || fake.transactionCalls != 0 || !isZeroCreateRunResult(result) {
		t.Fatalf("result/calls/error=%#v/%d/%v", result, fake.transactionCalls, err)
	}
}

func TestCreateRunRequestBudgetExactAndOneOverLimits(t *testing.T) {
	tests := []struct {
		name  string
		exact func(*createRunRequestBudget) error
		over  func(*createRunRequestBudget) error
	}{
		{"aggregate bytes", func(b *createRunRequestBudget) error {
			return b.addString(strings.Repeat("x", execution.MaxStringBytes))
		}, func(b *createRunRequestBudget) error {
			b.remainingBytes = execution.MaxStringBytes
			if err := b.addString(strings.Repeat("x", execution.MaxStringBytes)); err != nil {
				return err
			}
			return b.addString("x")
		}},
		{"aggregate parameters", func(b *createRunRequestBudget) error { return b.addParameters(execution.MaxAggregateParameters) }, func(b *createRunRequestBudget) error { return b.addParameters(execution.MaxAggregateParameters + 1) }},
		{"entry count", func(b *createRunRequestBudget) error { return b.addElements(execution.MaxAggregateCollectionElements) }, func(b *createRunRequestBudget) error {
			return b.addElements(execution.MaxAggregateCollectionElements + 1)
		}},
		{"multi-select item count", func(b *createRunRequestBudget) error { return b.addElements(execution.MaxAggregateCollectionElements) }, func(b *createRunRequestBudget) error {
			return b.addElements(execution.MaxAggregateCollectionElements + 1)
		}},
		{"aggregate elements", func(b *createRunRequestBudget) error {
			if err := b.addElements(1); err != nil {
				return err
			}
			return b.addElements(execution.MaxAggregateCollectionElements - 1)
		}, func(b *createRunRequestBudget) error {
			if err := b.addElements(1); err != nil {
				return err
			}
			return b.addElements(execution.MaxAggregateCollectionElements)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			budget := newCreateRunRequestBudget()
			if err := test.exact(&budget); err != nil {
				t.Fatalf("exact limit rejected: %v", err)
			}
			budget = newCreateRunRequestBudget()
			if err := test.over(&budget); !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
				t.Fatalf("one-over accepted: %v", err)
			}
		})
	}
	budget := newCreateRunRequestBudget()
	if err := budget.addElements(-1); !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
		t.Fatalf("negative/overflow-like count accepted: %v", err)
	}
	budget = newCreateRunRequestBudget()
	if err := budget.addParameters(-1); !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
		t.Fatalf("negative/overflow-like parameter count accepted: %v", err)
	}
}

func TestCreateRunPreflightStringBoundariesAndZeroStoreAccess(t *testing.T) {
	tests := []struct {
		name string
		edit func(*CreateRunCommand, int)
	}{
		{"identifier bytes", func(c *CreateRunCommand, n int) { c.ExecutionFlowID = strings.Repeat("r", n) }},
		{"destination bytes", func(c *CreateRunCommand, n int) { c.ScreenshotPolicy.Destination = strings.Repeat("d", n) }},
		{"entry identifier bytes", func(c *CreateRunCommand, n int) {
			c.Entries = map[string]map[string]parameter.Value{strings.Repeat("i", n): {}}
		}},
		{"parameter name bytes", func(c *CreateRunCommand, n int) {
			c.Entries["item-1"] = map[string]parameter.Value{strings.Repeat("n", n): parameter.TextValue("x")}
		}},
		{"text bytes", func(c *CreateRunCommand, n int) {
			c.Entries["item-1"] = map[string]parameter.Value{"value": parameter.TextValue(strings.Repeat("x", n))}
		}},
		{"single-select bytes", func(c *CreateRunCommand, n int) {
			c.Entries["item-1"] = map[string]parameter.Value{"value": parameter.SingleSelectValue(strings.Repeat("x", n))}
		}},
		{"multi-select element bytes", func(c *CreateRunCommand, n int) {
			c.Entries["item-1"] = map[string]parameter.Value{"value": parameter.MultiSelectValue([]string{strings.Repeat("x", n)})}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exact := validCreateRunCommand()
			test.edit(&exact, execution.MaxStringBytes)
			if err := preflightCreateRunCommand(exact); err != nil {
				t.Fatalf("exact limit rejected: %v", err)
			}
			over := validCreateRunCommand()
			test.edit(&over, execution.MaxStringBytes+1)
			if digest, err := CreateRunRequestDigest(over); digest != "" || !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
				t.Fatalf("digest/error=%q/%v", digest, err)
			}
			fake := &createRunFake{resolved: validResolvedCreateRun(t, validCreateRunCommand())}
			result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), over)
			if !fault.IsCode(err, CodeCreateInstanceCommandInvalid) || !isZeroCreateRunResult(result) || fake.transactionCalls != 0 {
				t.Fatalf("result/calls/error=%#v/%d/%v", result, fake.transactionCalls, err)
			}
		})
	}
}

func TestCreateRunPreflightNilAndEmptyEntriesAreEquivalent(t *testing.T) {
	nilCommand := validCreateRunCommand()
	nilCommand.Entries = nil
	emptyCommand := validCreateRunCommand()
	emptyCommand.Entries = map[string]map[string]parameter.Value{}
	if err := preflightCreateRunCommand(nilCommand); err != nil {
		t.Fatal(err)
	}
	if err := preflightCreateRunCommand(emptyCommand); err != nil {
		t.Fatal(err)
	}
	assertSameDigest(t, nilCommand, emptyCommand)
}

func TestCreateRunNormalizesEveryNegativeHealerZero(t *testing.T) {
	setters := []func(*execution.HealerPolicySnapshot, float64){
		func(p *execution.HealerPolicySnapshot, v float64) { p.ReviewCap = v },
		func(p *execution.HealerPolicySnapshot, v float64) { p.AppliedCap = v },
		func(p *execution.HealerPolicySnapshot, v float64) { p.Weights.Tag = v },
		func(p *execution.HealerPolicySnapshot, v float64) { p.Weights.ID = v },
		func(p *execution.HealerPolicySnapshot, v float64) { p.Weights.RoleName = v },
		func(p *execution.HealerPolicySnapshot, v float64) { p.Weights.Class = v },
		func(p *execution.HealerPolicySnapshot, v float64) { p.Weights.Attrs = v },
		func(p *execution.HealerPolicySnapshot, v float64) { p.Weights.Text = v },
		func(p *execution.HealerPolicySnapshot, v float64) { p.Weights.Index = v },
		func(p *execution.HealerPolicySnapshot, v float64) { p.Weights.Neighbor = v },
		func(p *execution.HealerPolicySnapshot, v float64) { p.Weights.LabelText = v },
		func(p *execution.HealerPolicySnapshot, v float64) { p.Weights.Container = v },
	}
	negativeZero := math.Copysign(0, -1)
	for index, set := range setters {
		positive, negative := validCreateRunCommand(), validCreateRunCommand()
		set(&positive.HealerPolicy, 0)
		set(&negative.HealerPolicy, negativeZero)
		assertSameDigest(t, positive, negative)
		if index == 1 { // AppliedCap == 0 is semantically invalid, but remains digest-canonical.
			continue
		}
		fake := &createRunFake{resolved: validResolvedCreateRun(t, positive)}
		service := mustCreateRunService(t, fake)
		created, err := service.CreateRun(context.Background(), positive)
		if err != nil {
			t.Fatalf("field %d create: %v", index, err)
		}
		digest, _ := CreateRunRequestDigest(positive)
		fake.stored = &StoredCreateRunCommand{CommandID: positive.CommandID, RequestDigest: digest, Result: StoredCreateRunResult{Run: created.Run, Snapshot: created.Snapshot, SnapshotDigest: created.Snapshot.Digest(), EntryIDs: created.EntryIDs}}
		if replayed, err := service.CreateRun(context.Background(), negative); err != nil || replayed.WasApplied {
			t.Fatalf("field %d negative-zero replay=%#v/%v", index, replayed, err)
		}
	}
}

func TestCreateRunDefaultsOmittedFailurePolicyToStop(t *testing.T) {
	command := validCreateRunCommand()
	command.FailurePolicy = ""
	resolved := validResolvedCreateRun(t, command)
	snapshot, err := BuildRunSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Input().FailurePolicy != execution.FailurePolicyStopOnFailure || snapshot.Plan().FailurePolicy != execution.FailurePolicyStopOnFailure {
		t.Fatalf("default policy was not sealed explicitly: %#v", snapshot.Input().FailurePolicy)
	}
	result, err := mustCreateRunService(t, &createRunFake{resolved: resolved}).CreateRun(context.Background(), command)
	if err != nil || result.Snapshot.Input().FailurePolicy != execution.FailurePolicyStopOnFailure {
		t.Fatalf("service default result/error=%#v/%v", result, err)
	}
}

func TestCreateRunRejectsInvalidNonzeroFailurePolicy(t *testing.T) {
	command := validCreateRunCommand()
	command.FailurePolicy = execution.FailurePolicy("RETRY_FOREVER")
	if _, err := BuildRunSnapshot(command, validResolvedCreateRun(t, command)); !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
		t.Fatalf("invalid policy error=%v", err)
	}
}

func TestCreateRunServiceReplaysSupportedV1StoredResult(t *testing.T) {
	command := validCreateRunCommand()
	resolved := validResolvedCreateRun(t, command)
	resolved.Environment.Variables = nil
	currentSnapshot, err := BuildRunSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	input := currentSnapshot.Input()
	input.SchemaVersion = execution.RunSnapshotSchemaV1
	input.Environment.Variables = nil
	input.Environment.Properties = map[string]string{"Region": "east"}
	snapshot, err := execution.SealInstanceSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	run, err := execution.NewRun(execution.Run{ID: command.RunID, ExecutionFlowID: command.ExecutionFlowID, TestTaskVersionID: command.TestTaskVersionID, EnvironmentID: command.EnvironmentID, Status: execution.Queued, CreatedAt: command.CreatedAt, QueuedAt: command.CreatedAt}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	entryIDs := make([]execution.EntryID, len(snapshot.Plan().Entries))
	for index, entry := range snapshot.Plan().Entries {
		entryIDs[index] = entry.ID
	}
	digest, err := CreateRunRequestDigest(command)
	if err != nil {
		t.Fatal(err)
	}
	fake := &createRunFake{stored: &StoredCreateRunCommand{CommandID: command.CommandID, RequestDigest: digest, Result: StoredCreateRunResult{Run: run, Snapshot: snapshot, SnapshotDigest: snapshot.Digest(), EntryIDs: entryIDs}}}

	result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.WasApplied || result.Snapshot.SchemaVersion() != execution.RunSnapshotSchemaV1 || fake.resolveCalls != 0 || fake.insertCalls != 0 {
		t.Fatalf("replayed result=%#v resolveCalls=%d insertCalls=%d", result, fake.resolveCalls, fake.insertCalls)
	}
}

func TestCreateRunServiceReturnsAuthoritativeDivergentReplayWinner(t *testing.T) {
	command := validCreateRunCommand()
	winnerResolved := validResolvedCreateRun(t, command)
	winnerResolved.Environment.Variables["Region"] = parameter.TextValue("winner")
	winnerSnapshot, err := BuildRunSnapshot(command, winnerResolved)
	if err != nil {
		t.Fatal(err)
	}
	winnerRun, err := execution.NewRun(execution.Run{ID: command.RunID, ExecutionFlowID: command.ExecutionFlowID, TestTaskVersionID: command.TestTaskVersionID, Status: execution.Queued, EnvironmentID: command.EnvironmentID, CreatedAt: command.CreatedAt, QueuedAt: command.CreatedAt}, winnerSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	winnerEntries := winnerSnapshot.Plan().Entries
	winnerIDs := make([]execution.EntryID, len(winnerEntries))
	for index := range winnerEntries {
		winnerIDs[index] = winnerEntries[index].ID
	}
	loserResolved := validResolvedCreateRun(t, command)
	fake := &createRunFake{resolved: loserResolved, mutateInsertOutcome: func(outcome *InsertCreateRunOutcome) {
		outcome.Status = InsertCreateRunReplayed
		outcome.Result = StoredCreateRunResult{Run: winnerRun, Snapshot: winnerSnapshot, SnapshotDigest: winnerSnapshot.Digest(), EntryIDs: winnerIDs}
	}}
	result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.WasApplied || result.Snapshot.Digest() != winnerSnapshot.Digest() || result.Snapshot.Environment().Variables["Region"].Text() != "winner" {
		t.Fatalf("authoritative winner was not returned: %#v", result)
	}
}

func TestCreateRunServiceRejectsReplayCommandAndSnapshotTampering(t *testing.T) {
	command := validCreateRunCommand()
	resolved := validResolvedCreateRun(t, command)
	snapshot, err := BuildRunSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	run, err := execution.NewRun(execution.Run{ID: command.RunID, ExecutionFlowID: command.ExecutionFlowID, TestTaskVersionID: command.TestTaskVersionID, EnvironmentID: command.EnvironmentID, Status: execution.Queued, CreatedAt: command.CreatedAt, QueuedAt: command.CreatedAt}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	entries := snapshot.Plan().Entries
	entryIDs := make([]execution.EntryID, len(entries))
	for index := range entries {
		entryIDs[index] = entries[index].ID
	}
	base := StoredCreateRunResult{Run: run, Snapshot: snapshot, SnapshotDigest: snapshot.Digest(), EntryIDs: entryIDs}
	tests := []struct {
		name   string
		mutate func(*StoredCreateRunResult)
	}{
		{"result digest", func(v *StoredCreateRunResult) { v.SnapshotDigest = "sha256:" + strings.Repeat("0", 64) }},
		{"environment", func(v *StoredCreateRunResult) { v.Run.EnvironmentID = "other" }},
		{"created at", func(v *StoredCreateRunResult) { v.Run.CreatedAt++ }},
		{"failure policy", func(v *StoredCreateRunResult) {
			input := v.Snapshot.Input()
			input.FailurePolicy = execution.FailurePolicyStopOnFailure
			input.Plan.FailurePolicy = execution.FailurePolicyStopOnFailure
			altered, sealErr := execution.SealInstanceSnapshot(input)
			if sealErr != nil {
				t.Fatal(sealErr)
			}
			v.Snapshot, v.SnapshotDigest = altered, altered.Digest()
			v.Run.SnapshotDigest = altered.Digest()
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stored := base
			stored.EntryIDs = append([]execution.EntryID(nil), base.EntryIDs...)
			test.mutate(&stored)
			fake := &createRunFake{resolved: resolved, mutateInsertOutcome: func(outcome *InsertCreateRunOutcome) {
				outcome.Status = InsertCreateRunReplayed
				outcome.Result = stored
			}}
			result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), command)
			if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || !isZeroCreateRunResult(result) {
				t.Fatalf("result/error=%#v/%v", result, err)
			}
		})
	}
}

func TestCreateRunServiceRejectsMalformedReplayWinner(t *testing.T) {
	command := validCreateRunCommand()
	fake := &createRunFake{resolved: validResolvedCreateRun(t, command), mutateInsertOutcome: func(outcome *InsertCreateRunOutcome) {
		outcome.Status = InsertCreateRunReplayed
		outcome.Result.EntryIDs[0] = mustEntryID("cross-run-entry")
	}}
	result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), command)
	if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || !isZeroCreateRunResult(result) {
		t.Fatalf("result/error=%#v/%v", result, err)
	}
}

func TestCreateRunServiceRejectsMalformedFindCommandReplay(t *testing.T) {
	command := validCreateRunCommand()
	seed := &createRunFake{resolved: validResolvedCreateRun(t, command)}
	created, err := mustCreateRunService(t, seed).CreateRun(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := CreateRunRequestDigest(command)
	mutations := []struct {
		name   string
		mutate func(*StoredCreateRunCommand)
	}{
		{"command identity", func(v *StoredCreateRunCommand) { v.CommandID = "other" }},
		{"run identity", func(v *StoredCreateRunCommand) { v.Result.Run.ID = mustInstanceID("other") }},
		{"task identity", func(v *StoredCreateRunCommand) { v.Result.Run.ExecutionFlowID = "other" }},
		{"snapshot seal", func(v *StoredCreateRunCommand) { v.Result.Run.SnapshotDigest = "sha256:" + strings.Repeat("0", 64) }},
		{"entry order", func(v *StoredCreateRunCommand) {
			v.Result.EntryIDs[0], v.Result.EntryIDs[1] = v.Result.EntryIDs[1], v.Result.EntryIDs[0]
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			stored := StoredCreateRunCommand{CommandID: command.CommandID, RequestDigest: digest, Result: StoredCreateRunResult{Run: created.Run, Snapshot: created.Snapshot, SnapshotDigest: created.Snapshot.Digest(), EntryIDs: append([]execution.EntryID(nil), created.EntryIDs...)}}
			test.mutate(&stored)
			fake := &createRunFake{stored: &stored, resolved: validResolvedCreateRun(t, command)}
			result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), command)
			if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || !isZeroCreateRunResult(result) {
				t.Fatalf("result/error=%#v/%v", result, err)
			}
		})
	}
}

func TestCreateRunServiceRejectsAppliedRunFieldDrift(t *testing.T) {
	command := validCreateRunCommand()
	mutations := []struct {
		name   string
		mutate func(*execution.Run)
	}{
		{"task", func(v *execution.Run) { v.ExecutionFlowID = "other" }}, {"version", func(v *execution.Run) { v.TestTaskVersionID = "other" }}, {"environment", func(v *execution.Run) { v.EnvironmentID = "other" }}, {"status", func(v *execution.Run) { v.Status = execution.Running }}, {"created", func(v *execution.Run) { v.CreatedAt++ }}, {"queued", func(v *execution.Run) { v.QueuedAt++ }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			fake := &createRunFake{resolved: validResolvedCreateRun(t, command), mutateInsertOutcome: func(outcome *InsertCreateRunOutcome) { test.mutate(&outcome.Result.Run) }}
			result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), command)
			if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || !isZeroCreateRunResult(result) {
				t.Fatalf("result/error=%#v/%v", result, err)
			}
		})
	}
	fake := &createRunFake{resolved: validResolvedCreateRun(t, command), mutateInsertOutcome: func(outcome *InsertCreateRunOutcome) { outcome.Result.Run.QueuePosition = 7 }}
	result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), command)
	if err != nil || result.Run.QueuePosition != 7 {
		t.Fatalf("adapter queue position rejected: %#v/%v", result, err)
	}
}

func TestBuildRunSnapshotKeepsRepeatedConcreteBindingsPathLocal(t *testing.T) {
	command := validCreateRunCommand()
	command.Entries["item-1"] = map[string]parameter.Value{"region": parameter.TextValue("east")}
	command.Entries["item-2"] = map[string]parameter.Value{"region": parameter.TextValue("west")}
	resolved := validResolvedCreateRun(t, command)
	definition := automation.ParameterDefinition{Name: "region", DisplayName: "Region", Type: parameter.Text, Required: true}
	resolved.Plan.Workflows[0].Version.Definition.Parameters = []automation.ParameterDefinition{definition}
	resolved.Plan.Workflows[1].Version.Definition.Parameters = []automation.ParameterDefinition{definition}
	resolved.Plan.Workflows[0].Version.Definition.Steps[0].Reference.ParameterBindings = map[string]parameter.Binding{"region": parameter.ParentReferenceBinding("region")}
	resolved.Plan.Version.Items[0].Parameters = map[string]parameter.Value{"region": parameter.TextValue("east")}
	resolved.Plan.Version.Items[1].Parameters = map[string]parameter.Value{"region": parameter.TextValue("west")}
	resolved.Invocations[0].Values = map[string]parameter.Value{"region": parameter.TextValue("east")}
	resolved.Invocations[1].Values = map[string]parameter.Value{"region": parameter.TextValue("east")}
	resolved.Invocations[1].Bindings = map[string]parameter.Binding{"region": parameter.ParentReferenceBinding("region")}
	resolved.Invocations[2].Values = map[string]parameter.Value{"region": parameter.TextValue("west")}
	resolved.Invocations[3].Values = map[string]parameter.Value{"region": parameter.TextValue("west")}
	resolved.Invocations[3].Bindings = map[string]parameter.Binding{"region": parameter.LiteralBinding(parameter.TextValue("west"))}

	snapshot, err := BuildRunSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	staticBinding := snapshot.Plan().Workflows[0].Steps[0].Reference.ParameterBindings["region"]
	if staticBinding.Kind() != parameter.ParentReferenceBindingKind {
		t.Fatal("concrete bindings overwrote static workflow authoring metadata")
	}
	first, firstOK := snapshot.Invocation(resolved.Invocations[1].Path)
	second, secondOK := snapshot.Invocation(resolved.Invocations[3].Path)
	if !firstOK || !secondOK || first.Bindings["region"].Kind() != parameter.ParentReferenceBindingKind || second.Bindings["region"].Kind() != parameter.LiteralBindingKind {
		t.Fatalf("path-local bindings lost: %#v/%#v", first.Bindings, second.Bindings)
	}
}

func TestResolvedCreateRunPreflightValidatesEnvironmentVariableNames(t *testing.T) {
	command := validCreateRunCommand()
	tests := []struct {
		name         string
		variableName string
		wantError    bool
	}{
		{name: "malformed UTF-8", variableName: string([]byte{0xff}), wantError: true},
		{name: "Unicode control character", variableName: "region" + string(rune(0x85)) + "name", wantError: true},
		{name: "Unicode format character", variableName: "region" + string(rune(0x202e)) + "name", wantError: true},
		{name: "over maximum bytes", variableName: strings.Repeat("x", parameter.MaxNameBytes+1), wantError: true},
		{name: "exact maximum bytes", variableName: strings.Repeat("x", parameter.MaxNameBytes)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved := validResolvedCreateRun(t, command)
			resolved.Environment.Variables = map[string]parameter.Value{test.variableName: parameter.TextValue("value")}

			snapshot, err := BuildRunSnapshot(command, resolved)

			if test.wantError {
				if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || snapshot.Digest() != "" {
					t.Fatalf("snapshot/error=%#v/%v", snapshot, err)
				}
				return
			}
			if err != nil || snapshot.Digest() == "" {
				t.Fatalf("snapshot/error=%#v/%v", snapshot, err)
			}
		})
	}
}

func TestResolvedCreateRunPreflightRejectsAdapterCollectionsBeforeBuild(t *testing.T) {
	command := validCreateRunCommand()
	tests := []struct {
		name   string
		mutate func(*ResolvedCreateRun)
	}{
		{"items", func(v *ResolvedCreateRun) {
			v.Plan.Version.Items = make([]automation.ExecutionFlowItem, execution.MaxAggregateCollectionElements+1)
		}},
		{"workflows", func(v *ResolvedCreateRun) {
			v.Plan.Workflows = make([]automation.FlowFragmentDependencySnapshot, execution.MaxDraftWorkflows+1)
		}},
		{"nodes", func(v *ResolvedCreateRun) {
			v.Plan.Nodes = make([]automation.ElementTargetDependencySnapshot, execution.MaxDraftNodes+1)
		}},
		{"references", func(v *ResolvedCreateRun) {
			v.Plan.References = make([]automation.FlowFragmentReferenceResolution, execution.MaxDraftReferences+1)
		}},
		{"string", func(v *ResolvedCreateRun) {
			v.Environment.DisplayName = strings.Repeat("x", execution.MaxStringBytes+1)
		}},
		{"step depth", func(v *ResolvedCreateRun) {
			steps := []automation.FlowFragmentStep{{ID: "leaf", DisplayName: "Leaf", Kind: automation.StepWait, WaitMS: 1}}
			for index := 0; index <= execution.MaxStepNestingDepth; index++ {
				steps = []automation.FlowFragmentStep{{ID: "repeat", DisplayName: "Repeat", Kind: automation.StepRepeat, RepeatCount: 1, Children: steps}}
			}
			v.Plan.Workflows[0].Version.Definition.Steps = steps
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved := validResolvedCreateRun(t, command)
			test.mutate(&resolved)
			snapshot, err := BuildRunSnapshot(command, resolved)
			if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || snapshot.Digest() != "" {
				t.Fatalf("snapshot/error=%#v/%v", snapshot, err)
			}
		})
	}
}

func TestResolvedCreateRunPreflightRejectsNestedAdapterPayloads(t *testing.T) {
	command := validCreateRunCommand()
	over := strings.Repeat("x", execution.MaxStringBytes+1)
	tests := []struct {
		name   string
		mutate func(*ResolvedCreateRun)
	}{
		{"item identity", func(v *ResolvedCreateRun) { v.Plan.Version.Items[0].FlowFragmentID = over }},
		{"item value", func(v *ResolvedCreateRun) {
			v.Plan.Version.Items[0].Parameters = map[string]parameter.Value{"value": parameter.TextValue(over)}
		}},
		{"workflow property", func(v *ResolvedCreateRun) {
			v.Plan.Workflows[0].FlowFragment.Properties = automation.Properties{over: "value"}
		}},
		{"parameter definition", func(v *ResolvedCreateRun) {
			v.Plan.Workflows[0].Version.Definition.Parameters = []automation.ParameterDefinition{{Name: "value", DisplayName: over, Type: parameter.Text, Required: true}}
		}},
		{"parameter option", func(v *ResolvedCreateRun) {
			v.Plan.Workflows[0].Version.Definition.Parameters = []automation.ParameterDefinition{{Name: "value", DisplayName: "Value", Type: parameter.SingleSelect, Required: true, Options: []string{over}}}
		}},
		{"step payload", func(v *ResolvedCreateRun) { v.Plan.Workflows[0].Version.Definition.Steps[0].Action = over }},
		{"reference binding", func(v *ResolvedCreateRun) {
			v.Plan.Workflows[0].Version.Definition.Steps[0].Reference.ParameterBindings = map[string]parameter.Binding{"child": parameter.LiteralBinding(parameter.TextValue(over))}
		}},
		{"selector", func(v *ResolvedCreateRun) { v.Plan.Nodes[0].Version.Selectors[0].Value = over }},
		{"fingerprint attribute", func(v *ResolvedCreateRun) {
			v.Plan.Nodes[0].Version.Fingerprint.Attributes = map[string]string{"name": over}
		}},
		{"reference resolution", func(v *ResolvedCreateRun) { v.Plan.References[0].StepID = over }},
		{"invocation value", func(v *ResolvedCreateRun) {
			v.Invocations[0].Values = map[string]parameter.Value{"value": parameter.TextValue(over)}
		}},
		{"invocation binding", func(v *ResolvedCreateRun) {
			v.Invocations[0].Bindings = map[string]parameter.Binding{"value": parameter.ParentReferenceBinding(over)}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved := validResolvedCreateRun(t, command)
			test.mutate(&resolved)
			snapshot, err := BuildRunSnapshot(command, resolved)
			if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || snapshot.Digest() != "" {
				t.Fatalf("snapshot/error=%#v/%v", snapshot, err)
			}
		})
	}
}

func TestCreateRunServiceAppliesReplaysAndConflicts(t *testing.T) {
	command := validCreateRunCommand()
	fake := &createRunFake{resolved: validResolvedCreateRun(t, command)}
	service := mustCreateRunService(t, fake)
	created, err := service.CreateRun(context.Background(), command)
	if err != nil || !created.WasApplied || fake.resolveCalls != 1 || fake.insertCalls != 1 {
		t.Fatalf("created=%#v calls=%d/%d err=%v", created, fake.resolveCalls, fake.insertCalls, err)
	}
	digest, _ := CreateRunRequestDigest(command)
	fake.stored = &StoredCreateRunCommand{CommandID: command.CommandID, RequestDigest: digest, Result: StoredCreateRunResult{Run: created.Run, Snapshot: created.Snapshot, SnapshotDigest: created.Snapshot.Digest(), EntryIDs: created.EntryIDs}}
	replayed, err := service.CreateRun(context.Background(), command)
	if err != nil || replayed.WasApplied || fake.resolveCalls != 1 || fake.insertCalls != 1 {
		t.Fatalf("replay=%#v calls=%d/%d err=%v", replayed, fake.resolveCalls, fake.insertCalls, err)
	}
	changed := command
	changed.EnvironmentID = "other"
	result, err := service.CreateRun(context.Background(), changed)
	descriptor, ok := fault.Describe(err)
	if !fault.IsCode(err, CodeCreateInstanceCommandConflict) || !ok ||
		descriptor.Kind() != fault.Conflict ||
		descriptor.Message() != "create-instance command conflicts with an existing request" ||
		len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 ||
		strings.Contains(err.Error(), command.CommandID) ||
		!isZeroCreateRunResult(result) {
		t.Fatalf("conflict result/error=%#v/%v", result, err)
	}
}

func TestCreateRunServicePreservesTypedErrorCategoriesAndReturnsNoResult(t *testing.T) {
	base := validCreateRunCommand()
	tests := []struct {
		name      string
		command   CreateRunCommand
		configure func(*createRunFake)
		target    error
		wantCode  fault.Code
	}{
		{"invalid command", func() CreateRunCommand { value := base; value.RunID = execution.InstanceID{}; return value }(), func(*createRunFake) {}, nil, CodeCreateInstanceCommandInvalid},
		{"find command", base, func(f *createRunFake) { f.findErr = errors.New("read failed") }, nil, CodeSchedulingAdapterUnavailable},
		{"build snapshot", base, func(f *createRunFake) { f.resolved.Environment.ID = "other" }, nil, ""},
		{"invalid insert outcome", base, func(f *createRunFake) { f.insertOutcome.Status = "UNKNOWN" }, nil, ""},
		{"catalog graph", base, func(f *createRunFake) {
			f.resolveErr = createRunCatalogGraphUnresolvableError(errors.New("missing child"))
		}, nil, CodeCreateInstanceCatalogGraphUnresolvable},
		{"retryable resolver", base, func(f *createRunFake) {
			f.resolveErr = createRunRetryableError(errors.New("serialization"))
		}, nil, CodeCreateInstanceRetryable},
		{"retryable insert", base, func(f *createRunFake) {
			f.insertErr = createRunRetryableError(errors.New("serialization"))
		}, nil, CodeCreateInstanceRetryable},
		{"retryable transaction", base, func(f *createRunFake) {
			f.transactionErr = createRunRetryableError(errors.New("serialization"))
		}, nil, CodeCreateInstanceRetryable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &createRunFake{resolved: validResolvedCreateRun(t, base)}
			test.configure(fake)
			result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), test.command)
			if err == nil || (test.target != nil && !errors.Is(err, test.target)) || (test.wantCode != "" && !fault.IsCode(err, test.wantCode)) || !isZeroCreateRunResult(result) {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if test.wantCode == CodeSchedulingAdapterUnavailable {
				descriptor, ok := fault.Describe(err)
				if !ok || strings.Contains(descriptor.Message(), "read failed") {
					t.Fatalf("public message = %#v (ok=%v), must not carry the adapter detail", descriptor, ok)
				}
				if cause := errors.Unwrap(err); cause == nil || !strings.Contains(cause.Error(), "read failed") {
					t.Fatalf("private cause = %v, want it to retain the adapter detail", cause)
				}
			}
		})
	}
}

func isZeroCreateRunResult(result CreateRunResult) bool {
	return result.Run.ID == (execution.InstanceID{}) && result.Snapshot.Digest() == "" && result.EntryIDs == nil && !result.WasApplied
}

func TestCreateRunServiceExposesNoResultWhenTransactionFails(t *testing.T) {
	command := validCreateRunCommand()
	fake := &createRunFake{resolved: validResolvedCreateRun(t, command), transactionErr: errors.New("commit failed")}
	result, err := mustCreateRunService(t, fake).CreateRun(context.Background(), command)
	if err == nil || result.Snapshot.Digest() != "" || result.WasApplied {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
