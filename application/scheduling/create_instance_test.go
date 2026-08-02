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

func validCreateInstanceCommand() CreateInstanceCommand {
	return CreateInstanceCommand{CommandID: "command-1", InstanceID: mustInstanceID("run-1"), ExecutionFlowID: "task", TestTaskVersionID: "task-v1", EnvironmentID: "env", Entries: map[string]map[string]parameter.Value{"item-1": {}, "item-2": {}}, FailurePolicy: execution.FailurePolicyContinueOnFailure, CreatedAt: 10, ScreenshotPolicy: execution.ScreenshotPolicySnapshot{Version: execution.ScreenshotPolicyV1, Enabled: true, Destination: "artifacts"}, HealerPolicy: execution.DefaultHealerPolicySnapshot()}
}

func validResolvedCreateInstance(t *testing.T, command CreateInstanceCommand) ResolvedCreateInstance {
	t.Helper()
	source := validMapperSource()
	source.InstanceID = command.InstanceID
	roots := make([]execution.InvocationScopeSnapshot, 0, len(source.Entries)*2)
	for _, entry := range source.Entries {
		entry.EntryID = mustEntryID(concreteRootPath(command.InstanceID.String(), entry.TestTaskItemID))
		roots = append(roots,
			execution.InvocationScopeSnapshot{Path: execution.RootInvocationPath(entry.EntryID), FlowFragmentID: entry.FlowFragmentID, WorkflowVersionID: entry.WorkflowVersionID, Values: map[string]parameter.Value{}},
			execution.InvocationScopeSnapshot{Path: mustInvocationPath(entry.EntryID.String() + "/10:call-child"), ParentPath: execution.RootInvocationPath(entry.EntryID), ParentVersionID: "root-v1", StepID: "call-child", FlowFragmentID: "child", WorkflowVersionID: "child-v1", ResolvedFromLatest: true, Values: map[string]parameter.Value{}, Bindings: map[string]parameter.Binding{}},
		)
	}
	return ResolvedCreateInstance{Plan: source.Publication, Environment: automation.Environment{ID: "env", DisplayName: "Environment", BaseURL: "https://example.test", Variables: automation.EnvironmentVariables{"Region": parameter.TextValue("east")}, Revision: 1}, Invocations: roots}
}

func TestCreateInstanceRequestDigestMatrix(t *testing.T) {
	base := validCreateInstanceCommand()
	base.Entries = map[string]map[string]parameter.Value{"item": {"value": parameter.TextValue("x")}}
	digest, err := CreateInstanceRequestDigest(base)
	if err != nil || len(digest) != 71 || !strings.HasPrefix(digest, "sha256:") || digest != strings.ToLower(digest) {
		t.Fatalf("digest=%q err=%v", digest, err)
	}
	again, _ := CreateInstanceRequestDigest(base)
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
	variants := []CreateInstanceCommand{}
	add := func(edit func(*CreateInstanceCommand)) {
		changed := base
		changed.Entries = map[string]map[string]parameter.Value{"item": {"value": parameter.TextValue("x")}}
		edit(&changed)
		variants = append(variants, changed)
	}
	add(func(v *CreateInstanceCommand) { v.InstanceID = mustInstanceID("other") })
	add(func(v *CreateInstanceCommand) { v.ExecutionFlowID = "other" })
	add(func(v *CreateInstanceCommand) { v.TestTaskVersionID = "other" })
	add(func(v *CreateInstanceCommand) { v.EnvironmentID = "other" })
	add(func(v *CreateInstanceCommand) { v.CreatedAt++ })
	add(func(v *CreateInstanceCommand) { v.FailurePolicy = execution.FailurePolicyStopOnFailure })
	add(func(v *CreateInstanceCommand) { v.ScreenshotPolicy.Enabled = !v.ScreenshotPolicy.Enabled })
	add(func(v *CreateInstanceCommand) { v.ScreenshotPolicy.Destination = "other" })
	add(func(v *CreateInstanceCommand) { v.HealerPolicy.ReviewCap += .01 })
	add(func(v *CreateInstanceCommand) { v.HealerPolicy.AppliedCap += .01 })
	weightEdits := []func(*execution.HealerWeightsSnapshot){func(w *execution.HealerWeightsSnapshot) { w.Tag += .01 }, func(w *execution.HealerWeightsSnapshot) { w.ID += .01 }, func(w *execution.HealerWeightsSnapshot) { w.RoleName += .01 }, func(w *execution.HealerWeightsSnapshot) { w.Class += .01 }, func(w *execution.HealerWeightsSnapshot) { w.Attrs += .01 }, func(w *execution.HealerWeightsSnapshot) { w.Text += .01 }, func(w *execution.HealerWeightsSnapshot) { w.Index += .01 }, func(w *execution.HealerWeightsSnapshot) { w.Neighbor += .01 }, func(w *execution.HealerWeightsSnapshot) { w.LabelText += .01 }, func(w *execution.HealerWeightsSnapshot) { w.Container += .01 }}
	for _, edit := range weightEdits {
		edit := edit
		add(func(v *CreateInstanceCommand) { edit(&v.HealerPolicy.Weights) })
	}
	add(func(v *CreateInstanceCommand) {
		v.Entries = map[string]map[string]parameter.Value{"other": {"value": parameter.TextValue("x")}}
	})
	add(func(v *CreateInstanceCommand) { v.Entries["item"]["value"] = parameter.TextValue("y") })
	for index, variant := range variants {
		assertDifferentDigest(t, base, variant, index)
	}
}

func TestCreateInstanceRequestDigestPreservesTypedAndMultiBoundaries(t *testing.T) {
	base := validCreateInstanceCommand()
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

func assertSameDigest(t *testing.T, left, right CreateInstanceCommand) {
	t.Helper()
	a, ea := CreateInstanceRequestDigest(left)
	b, eb := CreateInstanceRequestDigest(right)
	if ea != nil || eb != nil || a != b {
		t.Fatalf("digests differ: %q/%v %q/%v", a, ea, b, eb)
	}
}
func assertDifferentDigest(t *testing.T, left, right CreateInstanceCommand, index int) {
	t.Helper()
	a, ea := CreateInstanceRequestDigest(left)
	b, eb := CreateInstanceRequestDigest(right)
	if ea != nil || eb != nil || a == b {
		t.Fatalf("variant %d collided: %q/%v %q/%v", index, a, ea, b, eb)
	}
}

func TestCreateInstanceRequestDigestRejectsInvalidBeforeHashing(t *testing.T) {
	tests := []func(*CreateInstanceCommand){
		func(v *CreateInstanceCommand) { v.InstanceID = execution.InstanceID{} },
		func(v *CreateInstanceCommand) { v.FailurePolicy = "UNKNOWN" },
		func(v *CreateInstanceCommand) { v.ScreenshotPolicy.Version++ },
		func(v *CreateInstanceCommand) { v.HealerPolicy.Version++ },
		func(v *CreateInstanceCommand) { v.Entries["item-1"] = map[string]parameter.Value{"bad": {}} },
	}
	for index, edit := range tests {
		command := validCreateInstanceCommand()
		edit(&command)
		if digest, err := CreateInstanceRequestDigest(command); digest != "" || !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
			t.Fatalf("case %d digest=%q err=%v", index, digest, err)
		}
	}
}

func TestCreateInstanceRequestDigestIsCanonicalAndTyped(t *testing.T) {
	left := validCreateInstanceCommand()
	right := validCreateInstanceCommand()
	right.CommandID = "other-command"
	right.Entries = map[string]map[string]parameter.Value{"item-2": {}, "item-1": {}}
	leftDigest, err := CreateInstanceRequestDigest(left)
	if err != nil {
		t.Fatal(err)
	}
	rightDigest, err := CreateInstanceRequestDigest(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest != rightDigest {
		t.Fatalf("canonical digest mismatch: %q %q", leftDigest, rightDigest)
	}
	right.Entries["item-1"] = map[string]parameter.Value{"value": parameter.TextValue("true")}
	textDigest, _ := CreateInstanceRequestDigest(right)
	right.Entries["item-1"] = map[string]parameter.Value{"value": parameter.BooleanValue(true)}
	boolDigest, _ := CreateInstanceRequestDigest(right)
	if textDigest == boolDigest {
		t.Fatal("typed values collided")
	}
}

func TestBuildInstanceSnapshotRejectsResolverValueAndBindingDrift(t *testing.T) {
	command := validCreateInstanceCommand()
	tests := []struct {
		name   string
		mutate func(*CreateInstanceCommand, *ResolvedCreateInstance)
	}{
		{"unknown root input", func(c *CreateInstanceCommand, _ *ResolvedCreateInstance) {
			c.Entries["item-1"] = map[string]parameter.Value{"unknown": parameter.TextValue("x")}
		}},
		{"root invocation drift", func(_ *CreateInstanceCommand, r *ResolvedCreateInstance) {
			r.Invocations[0].Values["unknown"] = parameter.TextValue("x")
		}},
		{"child extra value", func(_ *CreateInstanceCommand, r *ResolvedCreateInstance) {
			r.Invocations[1].Values["extra"] = parameter.TextValue("x")
		}},
		{"child binding drift", func(_ *CreateInstanceCommand, r *ResolvedCreateInstance) {
			r.Invocations[1].Bindings["extra"] = parameter.LiteralBinding(parameter.TextValue("x"))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := command
			current.Entries = map[string]map[string]parameter.Value{"item-1": {}, "item-2": {}}
			resolved := validResolvedCreateInstance(t, current)
			test.mutate(&current, &resolved)
			if _, err := BuildInstanceSnapshot(current, resolved); err == nil {
				t.Fatal("resolver drift accepted")
			}
		})
	}
}

func TestBuildInstanceSnapshotResolvesRootDefaultsAndRejectsValueDrift(t *testing.T) {
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
			command := validCreateInstanceCommand()
			command.Entries["item-1"] = test.supplied
			resolved := validResolvedCreateInstance(t, command)
			resolved.Plan.Workflows[0].Version.Definition.Parameters = []automation.ParameterDefinition{test.definition}
			resolved.Invocations[0].Values = test.resolved
			secondValues, resolveErr := automation.ResolveParameterValues([]automation.ParameterDefinition{test.definition}, command.Entries["item-2"])
			if resolveErr == nil {
				resolved.Invocations[2].Values = secondValues
			}
			snapshot, err := BuildInstanceSnapshot(command, resolved)
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

func TestBuildInstanceSnapshotEnforcesConcreteNestedDefaultsAndBindings(t *testing.T) {
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
			command := validCreateInstanceCommand()
			resolved := validResolvedCreateInstance(t, command)
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
			snapshot, err := BuildInstanceSnapshot(command, resolved)
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

func TestBuildInstanceSnapshotPreservesExactTypedParentReferenceValues(t *testing.T) {
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
	command := validCreateInstanceCommand()
	command.Entries = map[string]map[string]parameter.Value{"item-1": cloneParameterValues(values), "item-2": cloneParameterValues(values)}
	resolved := validResolvedCreateInstance(t, command)
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
	snapshot, err := BuildInstanceSnapshot(command, resolved)
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

func TestBuildInstanceSnapshotMapsCatalogAndDefensivelyOwnsAssets(t *testing.T) {
	command := validCreateInstanceCommand()
	resolved := validResolvedCreateInstance(t, command)
	snapshot, err := BuildInstanceSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Environment.Variables["Region"] = parameter.TextValue("west")
	if snapshot.Environment().Variables["Region"].Text() != "east" || snapshot.TestTaskVersionID() != "task-v1" || len(snapshot.Plan().Entries) != 2 {
		t.Fatal("snapshot did not freeze complete resolved create data")
	}
}

func mustCreateInstanceService(t *testing.T, store CreateInstanceStore) CreateInstanceService {
	t.Helper()
	service, err := NewCreateInstanceService(store)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestNewCreateInstanceServiceRejectsNilStore(t *testing.T) {
	var typedNil *createInstanceFake
	for _, store := range []CreateInstanceStore{nil, typedNil} {
		if _, err := NewCreateInstanceService(store); err == nil {
			t.Fatal("expected configuration error")
		}
	}
}

type createInstanceFake struct {
	stored                    *StoredCreateInstanceCommand
	resolved                  ResolvedCreateInstance
	resolveCalls, insertCalls int
	transactionErr            error
	findErr                   error
	resolveErr                error
	insertErr                 error
	insertOutcome             InsertCreateInstanceOutcome
	mutateResolvedCommand     func(CreateInstanceCommand)
	mutateInsertOutcome       func(*InsertCreateInstanceOutcome)
	transactionCalls          int
}

func (f *createInstanceFake) InTransaction(ctx context.Context, callback func(CreateInstanceTx) error) error {
	f.transactionCalls++
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.transactionErr != nil {
		return f.transactionErr
	}
	return callback(f)
}
func (f *createInstanceFake) FindCommand(context.Context, string) (StoredCreateInstanceCommand, bool, error) {
	if f.findErr != nil {
		return StoredCreateInstanceCommand{}, false, f.findErr
	}
	if f.stored == nil {
		return StoredCreateInstanceCommand{}, false, nil
	}
	return *f.stored, true, nil
}
func (f *createInstanceFake) ResolveCreateInstance(_ context.Context, command CreateInstanceCommand) (ResolvedCreateInstance, error) {
	f.resolveCalls++
	if f.mutateResolvedCommand != nil {
		f.mutateResolvedCommand(command)
	}
	if f.resolveErr != nil {
		return ResolvedCreateInstance{}, f.resolveErr
	}
	return f.resolved, nil
}
func (f *createInstanceFake) InsertCreateInstance(_ context.Context, intent CreateInstanceIntent) (InsertCreateInstanceOutcome, error) {
	f.insertCalls++
	if f.insertErr != nil {
		return InsertCreateInstanceOutcome{}, f.insertErr
	}
	if f.insertOutcome.Status != "" {
		return f.insertOutcome, nil
	}
	entries := make([]execution.EntryID, len(intent.Entries))
	for i := range intent.Entries {
		entries[i] = intent.Entries[i].ID
	}
	outcome := InsertCreateInstanceOutcome{Status: InsertCreateInstanceApplied, CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: StoredCreateInstanceResult{Run: intent.Run, Snapshot: intent.Snapshot, SnapshotDigest: intent.Snapshot.Digest(), EntryIDs: entries}}
	if f.mutateInsertOutcome != nil {
		f.mutateInsertOutcome(&outcome)
	}
	return outcome, nil
}

func TestCreateInstanceServiceOwnsCommandAcrossResolverMutation(t *testing.T) {
	command := validCreateInstanceCommand()
	originalDigest, err := CreateInstanceRequestDigest(command)
	if err != nil {
		t.Fatal(err)
	}
	fake := &createInstanceFake{resolved: validResolvedCreateInstance(t, command), mutateResolvedCommand: func(received CreateInstanceCommand) {
		received.Entries["item-1"]["injected"] = parameter.TextValue("mutated")
		delete(received.Entries, "item-2")
	}}
	result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.Digest() == "" || len(result.Snapshot.Plan().Entries) != 2 {
		t.Fatalf("snapshot was desynchronized: %#v", result)
	}
	afterDigest, _ := CreateInstanceRequestDigest(command)
	if afterDigest != originalDigest || len(command.Entries) != 2 || len(command.Entries["item-1"]) != 0 {
		t.Fatal("caller command was mutated")
	}
}

func TestCreateInstanceServiceRejectsMalformedAuthoritativeOutcomes(t *testing.T) {
	command := validCreateInstanceCommand()
	mutations := []struct {
		name   string
		mutate func(*InsertCreateInstanceOutcome)
	}{
		{"zero", func(v *InsertCreateInstanceOutcome) { *v = InsertCreateInstanceOutcome{} }},
		{"wrong command", func(v *InsertCreateInstanceOutcome) { v.CommandID = "other" }},
		{"wrong digest", func(v *InsertCreateInstanceOutcome) { v.RequestDigest = "sha256:" + strings.Repeat("0", 64) }},
		{"wrong run", func(v *InsertCreateInstanceOutcome) { v.Result.Run.ID = mustInstanceID("other") }},
		{"empty snapshot", func(v *InsertCreateInstanceOutcome) { v.Result.Snapshot = execution.InstanceSnapshot{} }},
		{"wrong entries", func(v *InsertCreateInstanceOutcome) { v.Result.EntryIDs = []execution.EntryID{mustEntryID("other")} }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			fake := &createInstanceFake{resolved: validResolvedCreateInstance(t, command), mutateInsertOutcome: test.mutate}
			result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), command)
			if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || !isZeroCreateInstanceResult(result) {
				t.Fatalf("result/error=%#v/%v", result, err)
			}
		})
	}
}

func TestCreateInstanceServicePreflightRejectsOversizeBeforeStore(t *testing.T) {
	command := validCreateInstanceCommand()
	command.ExecutionFlowID = strings.Repeat("x", execution.MaxStringBytes+1)
	fake := &createInstanceFake{resolved: validResolvedCreateInstance(t, validCreateInstanceCommand())}
	result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), command)
	if !fault.IsCode(err, CodeCreateInstanceCommandInvalid) || fake.transactionCalls != 0 || !isZeroCreateInstanceResult(result) {
		t.Fatalf("result/calls/error=%#v/%d/%v", result, fake.transactionCalls, err)
	}
}

func TestCreateInstanceRequestBudgetExactAndOneOverLimits(t *testing.T) {
	fillByteBudget := func(b *createInstanceRequestBudget, remaining int) error {
		chunk := strings.Repeat("x", execution.MaxStringBytes)
		for remaining >= len(chunk) {
			if err := b.addString(chunk); err != nil {
				return err
			}
			remaining -= len(chunk)
		}
		if remaining == 0 {
			return nil
		}
		return b.addString(chunk[:remaining])
	}
	tests := []struct {
		name  string
		exact func(*createInstanceRequestBudget) error
		over  func(*createInstanceRequestBudget) error
	}{
		{"single string bytes", func(b *createInstanceRequestBudget) error {
			return b.addString(strings.Repeat("x", execution.MaxStringBytes))
		}, func(b *createInstanceRequestBudget) error {
			return b.addString(strings.Repeat("x", execution.MaxStringBytes+1))
		}},
		{"multi-select item bytes", func(b *createInstanceRequestBudget) error {
			return b.addStringMetrics(execution.MaxStringBytes, execution.MaxStringBytes)
		}, func(b *createInstanceRequestBudget) error {
			return b.addStringMetrics(execution.MaxStringBytes+1, execution.MaxStringBytes+1)
		}},
		{"aggregate string bytes", func(b *createInstanceRequestBudget) error {
			return fillByteBudget(b, execution.MaxAggregateStringBytes)
		}, func(b *createInstanceRequestBudget) error {
			if err := fillByteBudget(b, execution.MaxAggregateStringBytes); err != nil {
				return err
			}
			return b.addString("x")
		}},
		{"aggregate parameters", func(b *createInstanceRequestBudget) error { return b.addParameters(execution.MaxAggregateParameters) }, func(b *createInstanceRequestBudget) error {
			return b.addParameters(execution.MaxAggregateParameters + 1)
		}},
		{"entry count", func(b *createInstanceRequestBudget) error {
			return b.addElements(execution.MaxAggregateCollectionElements)
		}, func(b *createInstanceRequestBudget) error {
			return b.addElements(execution.MaxAggregateCollectionElements + 1)
		}},
		{"multi-select item count", func(b *createInstanceRequestBudget) error {
			return b.addElements(execution.MaxAggregateCollectionElements)
		}, func(b *createInstanceRequestBudget) error {
			return b.addElements(execution.MaxAggregateCollectionElements + 1)
		}},
		{"aggregate elements", func(b *createInstanceRequestBudget) error {
			if err := b.addElements(1); err != nil {
				return err
			}
			return b.addElements(execution.MaxAggregateCollectionElements - 1)
		}, func(b *createInstanceRequestBudget) error {
			if err := b.addElements(1); err != nil {
				return err
			}
			return b.addElements(execution.MaxAggregateCollectionElements)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			budget := newCreateInstanceRequestBudget()
			if err := test.exact(&budget); err != nil {
				t.Fatalf("exact limit rejected: %v", err)
			}
			budget = newCreateInstanceRequestBudget()
			if err := test.over(&budget); !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
				t.Fatalf("one-over accepted: %v", err)
			}
		})
	}
	budget := newCreateInstanceRequestBudget()
	if err := budget.addElements(-1); !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
		t.Fatalf("negative/overflow-like count accepted: %v", err)
	}
	budget = newCreateInstanceRequestBudget()
	if err := budget.addParameters(-1); !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
		t.Fatalf("negative/overflow-like parameter count accepted: %v", err)
	}
}

func TestCreateInstancePreflightStringBoundariesAndZeroStoreAccess(t *testing.T) {
	tests := []struct {
		name string
		edit func(*CreateInstanceCommand, int)
	}{
		{"identifier bytes", func(c *CreateInstanceCommand, n int) { c.ExecutionFlowID = strings.Repeat("r", n) }},
		{"destination bytes", func(c *CreateInstanceCommand, n int) { c.ScreenshotPolicy.Destination = strings.Repeat("d", n) }},
		{"entry identifier bytes", func(c *CreateInstanceCommand, n int) {
			c.Entries = map[string]map[string]parameter.Value{strings.Repeat("i", n): {}}
		}},
		{"parameter name bytes", func(c *CreateInstanceCommand, n int) {
			c.Entries = map[string]map[string]parameter.Value{"": {strings.Repeat("n", n): parameter.TextValue("")}}
		}},
		{"text bytes", func(c *CreateInstanceCommand, n int) {
			c.Entries = map[string]map[string]parameter.Value{"": {"": parameter.TextValue(strings.Repeat("x", n))}}
		}},
		{"single-select bytes", func(c *CreateInstanceCommand, n int) {
			c.Entries = map[string]map[string]parameter.Value{"": {"": parameter.SingleSelectValue(strings.Repeat("x", n))}}
		}},
		{"multi-select element bytes", func(c *CreateInstanceCommand, n int) {
			c.Entries = map[string]map[string]parameter.Value{"": {"": parameter.MultiSelectValue([]string{strings.Repeat("x", n)})}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// A zero-value command isolates the target string. Command semantics are
			// checked after preflight, while this test pins its byte boundaries without
			// spending aggregate budget on unrelated fixture fields.
			exact := CreateInstanceCommand{}
			test.edit(&exact, execution.MaxStringBytes)
			if err := preflightCreateInstanceCommand(exact); err != nil {
				t.Fatalf("exact single-string limit rejected: %v", err)
			}

			over := CreateInstanceCommand{}
			test.edit(&over, execution.MaxStringBytes+1)
			if digest, err := CreateInstanceRequestDigest(over); digest != "" || !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
				t.Fatalf("digest/error=%q/%v", digest, err)
			}
			fake := &createInstanceFake{resolved: validResolvedCreateInstance(t, validCreateInstanceCommand())}
			result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), over)
			if !fault.IsCode(err, CodeCreateInstanceCommandInvalid) || !isZeroCreateInstanceResult(result) || fake.transactionCalls != 0 {
				t.Fatalf("result/calls/error=%#v/%d/%v", result, fake.transactionCalls, err)
			}
		})
	}
}

func TestCreateInstancePreflightNilAndEmptyEntriesAreEquivalent(t *testing.T) {
	nilCommand := validCreateInstanceCommand()
	nilCommand.Entries = nil
	emptyCommand := validCreateInstanceCommand()
	emptyCommand.Entries = map[string]map[string]parameter.Value{}
	if err := preflightCreateInstanceCommand(nilCommand); err != nil {
		t.Fatal(err)
	}
	if err := preflightCreateInstanceCommand(emptyCommand); err != nil {
		t.Fatal(err)
	}
	assertSameDigest(t, nilCommand, emptyCommand)
}

func TestCreateInstanceNormalizesEveryNegativeHealerZero(t *testing.T) {
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
		positive, negative := validCreateInstanceCommand(), validCreateInstanceCommand()
		set(&positive.HealerPolicy, 0)
		set(&negative.HealerPolicy, negativeZero)
		assertSameDigest(t, positive, negative)
		if index == 1 { // AppliedCap == 0 is semantically invalid, but remains digest-canonical.
			continue
		}
		fake := &createInstanceFake{resolved: validResolvedCreateInstance(t, positive)}
		service := mustCreateInstanceService(t, fake)
		created, err := service.CreateInstance(context.Background(), positive)
		if err != nil {
			t.Fatalf("field %d create: %v", index, err)
		}
		digest, _ := CreateInstanceRequestDigest(positive)
		fake.stored = &StoredCreateInstanceCommand{CommandID: positive.CommandID, RequestDigest: digest, Result: StoredCreateInstanceResult{Run: created.Run, Snapshot: created.Snapshot, SnapshotDigest: created.Snapshot.Digest(), EntryIDs: created.EntryIDs}}
		if replayed, err := service.CreateInstance(context.Background(), negative); err != nil || replayed.WasApplied {
			t.Fatalf("field %d negative-zero replay=%#v/%v", index, replayed, err)
		}
	}
}

func TestCreateInstanceDefaultsOmittedFailurePolicyToStop(t *testing.T) {
	command := validCreateInstanceCommand()
	command.FailurePolicy = ""
	resolved := validResolvedCreateInstance(t, command)
	snapshot, err := BuildInstanceSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Input().FailurePolicy != execution.FailurePolicyStopOnFailure || snapshot.Plan().FailurePolicy != execution.FailurePolicyStopOnFailure {
		t.Fatalf("default policy was not sealed explicitly: %#v", snapshot.Input().FailurePolicy)
	}
	result, err := mustCreateInstanceService(t, &createInstanceFake{resolved: resolved}).CreateInstance(context.Background(), command)
	if err != nil || result.Snapshot.Input().FailurePolicy != execution.FailurePolicyStopOnFailure {
		t.Fatalf("service default result/error=%#v/%v", result, err)
	}
}

func TestCreateInstanceRejectsInvalidNonzeroFailurePolicy(t *testing.T) {
	command := validCreateInstanceCommand()
	command.FailurePolicy = execution.FailurePolicy("RETRY_FOREVER")
	if _, err := BuildInstanceSnapshot(command, validResolvedCreateInstance(t, command)); !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
		t.Fatalf("invalid policy error=%v", err)
	}
}

func TestCreateInstanceServiceReplaysSupportedV1StoredResult(t *testing.T) {
	command := validCreateInstanceCommand()
	resolved := validResolvedCreateInstance(t, command)
	resolved.Environment.Variables = nil
	currentSnapshot, err := BuildInstanceSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	input := currentSnapshot.Input()
	input.SchemaVersion = execution.InstanceSnapshotSchemaV1
	input.Environment.Variables = nil
	input.Environment.Properties = map[string]string{"Region": "east"}
	snapshot, err := execution.SealInstanceSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	run, err := execution.NewInstance(execution.Instance{ID: command.InstanceID, ExecutionFlowID: command.ExecutionFlowID, TestTaskVersionID: command.TestTaskVersionID, EnvironmentID: command.EnvironmentID, Status: execution.Queued, CreatedAt: command.CreatedAt, QueuedAt: command.CreatedAt}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	entryIDs := make([]execution.EntryID, len(snapshot.Plan().Entries))
	for index, entry := range snapshot.Plan().Entries {
		entryIDs[index] = entry.ID
	}
	digest, err := CreateInstanceRequestDigest(command)
	if err != nil {
		t.Fatal(err)
	}
	fake := &createInstanceFake{stored: &StoredCreateInstanceCommand{CommandID: command.CommandID, RequestDigest: digest, Result: StoredCreateInstanceResult{Run: run, Snapshot: snapshot, SnapshotDigest: snapshot.Digest(), EntryIDs: entryIDs}}}

	result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.WasApplied || result.Snapshot.SchemaVersion() != execution.InstanceSnapshotSchemaV1 || fake.resolveCalls != 0 || fake.insertCalls != 0 {
		t.Fatalf("replayed result=%#v resolveCalls=%d insertCalls=%d", result, fake.resolveCalls, fake.insertCalls)
	}
}

func TestCreateInstanceServiceReturnsAuthoritativeDivergentReplayWinner(t *testing.T) {
	command := validCreateInstanceCommand()
	winnerResolved := validResolvedCreateInstance(t, command)
	winnerResolved.Environment.Variables["Region"] = parameter.TextValue("winner")
	winnerSnapshot, err := BuildInstanceSnapshot(command, winnerResolved)
	if err != nil {
		t.Fatal(err)
	}
	winnerInstance, err := execution.NewInstance(execution.Instance{ID: command.InstanceID, ExecutionFlowID: command.ExecutionFlowID, TestTaskVersionID: command.TestTaskVersionID, Status: execution.Queued, EnvironmentID: command.EnvironmentID, CreatedAt: command.CreatedAt, QueuedAt: command.CreatedAt}, winnerSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	winnerEntries := winnerSnapshot.Plan().Entries
	winnerIDs := make([]execution.EntryID, len(winnerEntries))
	for index := range winnerEntries {
		winnerIDs[index] = winnerEntries[index].ID
	}
	loserResolved := validResolvedCreateInstance(t, command)
	fake := &createInstanceFake{resolved: loserResolved, mutateInsertOutcome: func(outcome *InsertCreateInstanceOutcome) {
		outcome.Status = InsertCreateInstanceReplayed
		outcome.Result = StoredCreateInstanceResult{Run: winnerInstance, Snapshot: winnerSnapshot, SnapshotDigest: winnerSnapshot.Digest(), EntryIDs: winnerIDs}
	}}
	result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.WasApplied || result.Snapshot.Digest() != winnerSnapshot.Digest() || result.Snapshot.Environment().Variables["Region"].Text() != "winner" {
		t.Fatalf("authoritative winner was not returned: %#v", result)
	}
}

func TestCreateInstanceServiceRejectsReplayCommandAndSnapshotTampering(t *testing.T) {
	command := validCreateInstanceCommand()
	resolved := validResolvedCreateInstance(t, command)
	snapshot, err := BuildInstanceSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	run, err := execution.NewInstance(execution.Instance{ID: command.InstanceID, ExecutionFlowID: command.ExecutionFlowID, TestTaskVersionID: command.TestTaskVersionID, EnvironmentID: command.EnvironmentID, Status: execution.Queued, CreatedAt: command.CreatedAt, QueuedAt: command.CreatedAt}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	entries := snapshot.Plan().Entries
	entryIDs := make([]execution.EntryID, len(entries))
	for index := range entries {
		entryIDs[index] = entries[index].ID
	}
	base := StoredCreateInstanceResult{Run: run, Snapshot: snapshot, SnapshotDigest: snapshot.Digest(), EntryIDs: entryIDs}
	tests := []struct {
		name   string
		mutate func(*StoredCreateInstanceResult)
	}{
		{"result digest", func(v *StoredCreateInstanceResult) { v.SnapshotDigest = "sha256:" + strings.Repeat("0", 64) }},
		{"environment", func(v *StoredCreateInstanceResult) { v.Run.EnvironmentID = "other" }},
		{"created at", func(v *StoredCreateInstanceResult) { v.Run.CreatedAt++ }},
		{"failure policy", func(v *StoredCreateInstanceResult) {
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
			fake := &createInstanceFake{resolved: resolved, mutateInsertOutcome: func(outcome *InsertCreateInstanceOutcome) {
				outcome.Status = InsertCreateInstanceReplayed
				outcome.Result = stored
			}}
			result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), command)
			if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || !isZeroCreateInstanceResult(result) {
				t.Fatalf("result/error=%#v/%v", result, err)
			}
		})
	}
}

func TestCreateInstanceServiceRejectsMalformedReplayWinner(t *testing.T) {
	command := validCreateInstanceCommand()
	fake := &createInstanceFake{resolved: validResolvedCreateInstance(t, command), mutateInsertOutcome: func(outcome *InsertCreateInstanceOutcome) {
		outcome.Status = InsertCreateInstanceReplayed
		outcome.Result.EntryIDs[0] = mustEntryID("cross-run-entry")
	}}
	result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), command)
	if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || !isZeroCreateInstanceResult(result) {
		t.Fatalf("result/error=%#v/%v", result, err)
	}
}

func TestCreateInstanceServiceRejectsMalformedFindCommandReplay(t *testing.T) {
	command := validCreateInstanceCommand()
	seed := &createInstanceFake{resolved: validResolvedCreateInstance(t, command)}
	created, err := mustCreateInstanceService(t, seed).CreateInstance(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := CreateInstanceRequestDigest(command)
	mutations := []struct {
		name   string
		mutate func(*StoredCreateInstanceCommand)
	}{
		{"command identity", func(v *StoredCreateInstanceCommand) { v.CommandID = "other" }},
		{"run identity", func(v *StoredCreateInstanceCommand) { v.Result.Run.ID = mustInstanceID("other") }},
		{"task identity", func(v *StoredCreateInstanceCommand) { v.Result.Run.ExecutionFlowID = "other" }},
		{"snapshot seal", func(v *StoredCreateInstanceCommand) {
			v.Result.Run.SnapshotDigest = "sha256:" + strings.Repeat("0", 64)
		}},
		{"entry order", func(v *StoredCreateInstanceCommand) {
			v.Result.EntryIDs[0], v.Result.EntryIDs[1] = v.Result.EntryIDs[1], v.Result.EntryIDs[0]
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			stored := StoredCreateInstanceCommand{CommandID: command.CommandID, RequestDigest: digest, Result: StoredCreateInstanceResult{Run: created.Run, Snapshot: created.Snapshot, SnapshotDigest: created.Snapshot.Digest(), EntryIDs: append([]execution.EntryID(nil), created.EntryIDs...)}}
			test.mutate(&stored)
			fake := &createInstanceFake{stored: &stored, resolved: validResolvedCreateInstance(t, command)}
			result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), command)
			if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || !isZeroCreateInstanceResult(result) {
				t.Fatalf("result/error=%#v/%v", result, err)
			}
		})
	}
}

func TestCreateInstanceServiceRejectsAppliedRunFieldDrift(t *testing.T) {
	command := validCreateInstanceCommand()
	mutations := []struct {
		name   string
		mutate func(*execution.Instance)
	}{
		{"task", func(v *execution.Instance) { v.ExecutionFlowID = "other" }}, {"version", func(v *execution.Instance) { v.TestTaskVersionID = "other" }}, {"environment", func(v *execution.Instance) { v.EnvironmentID = "other" }}, {"status", func(v *execution.Instance) { v.Status = execution.Running }}, {"created", func(v *execution.Instance) { v.CreatedAt++ }}, {"queued", func(v *execution.Instance) { v.QueuedAt++ }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			fake := &createInstanceFake{resolved: validResolvedCreateInstance(t, command), mutateInsertOutcome: func(outcome *InsertCreateInstanceOutcome) { test.mutate(&outcome.Result.Run) }}
			result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), command)
			if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || !isZeroCreateInstanceResult(result) {
				t.Fatalf("result/error=%#v/%v", result, err)
			}
		})
	}
	fake := &createInstanceFake{resolved: validResolvedCreateInstance(t, command), mutateInsertOutcome: func(outcome *InsertCreateInstanceOutcome) { outcome.Result.Run.QueuePosition = 7 }}
	result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), command)
	if err != nil || result.Run.QueuePosition != 7 {
		t.Fatalf("adapter queue position rejected: %#v/%v", result, err)
	}
}

func TestBuildInstanceSnapshotKeepsRepeatedConcreteBindingsPathLocal(t *testing.T) {
	command := validCreateInstanceCommand()
	command.Entries["item-1"] = map[string]parameter.Value{"region": parameter.TextValue("east")}
	command.Entries["item-2"] = map[string]parameter.Value{"region": parameter.TextValue("west")}
	resolved := validResolvedCreateInstance(t, command)
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

	snapshot, err := BuildInstanceSnapshot(command, resolved)
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

func TestResolvedCreateInstancePreflightValidatesEnvironmentVariableNames(t *testing.T) {
	command := validCreateInstanceCommand()
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
			resolved := validResolvedCreateInstance(t, command)
			resolved.Environment.Variables = map[string]parameter.Value{test.variableName: parameter.TextValue("value")}

			snapshot, err := BuildInstanceSnapshot(command, resolved)

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

func TestResolvedCreateInstancePreflightRejectsAdapterCollectionsBeforeBuild(t *testing.T) {
	command := validCreateInstanceCommand()
	tests := []struct {
		name   string
		mutate func(*ResolvedCreateInstance)
	}{
		{"items", func(v *ResolvedCreateInstance) {
			v.Plan.Version.Items = make([]automation.ExecutionFlowItem, execution.MaxAggregateCollectionElements+1)
		}},
		{"workflows", func(v *ResolvedCreateInstance) {
			v.Plan.Workflows = make([]automation.FlowFragmentDependencySnapshot, execution.MaxDraftWorkflows+1)
		}},
		{"nodes", func(v *ResolvedCreateInstance) {
			v.Plan.Nodes = make([]automation.ElementTargetDependencySnapshot, execution.MaxDraftNodes+1)
		}},
		{"references", func(v *ResolvedCreateInstance) {
			v.Plan.References = make([]automation.FlowFragmentReferenceResolution, execution.MaxDraftReferences+1)
		}},
		{"string", func(v *ResolvedCreateInstance) {
			v.Environment.DisplayName = strings.Repeat("x", execution.MaxStringBytes+1)
		}},
		{"step depth", func(v *ResolvedCreateInstance) {
			steps := []automation.FlowFragmentStep{{ID: "leaf", DisplayName: "Leaf", Kind: automation.StepWait, WaitMS: 1}}
			for index := 0; index <= execution.MaxStepNestingDepth; index++ {
				steps = []automation.FlowFragmentStep{{ID: "repeat", DisplayName: "Repeat", Kind: automation.StepRepeat, RepeatCount: 1, Children: steps}}
			}
			v.Plan.Workflows[0].Version.Definition.Steps = steps
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved := validResolvedCreateInstance(t, command)
			test.mutate(&resolved)
			snapshot, err := BuildInstanceSnapshot(command, resolved)
			if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || snapshot.Digest() != "" {
				t.Fatalf("snapshot/error=%#v/%v", snapshot, err)
			}
		})
	}
}

func TestResolvedCreateInstancePreflightRejectsNestedAdapterPayloads(t *testing.T) {
	command := validCreateInstanceCommand()
	over := strings.Repeat("x", execution.MaxStringBytes+1)
	tests := []struct {
		name   string
		mutate func(*ResolvedCreateInstance)
	}{
		{"item identity", func(v *ResolvedCreateInstance) { v.Plan.Version.Items[0].FlowFragmentID = over }},
		{"item value", func(v *ResolvedCreateInstance) {
			v.Plan.Version.Items[0].Parameters = map[string]parameter.Value{"value": parameter.TextValue(over)}
		}},
		{"workflow property", func(v *ResolvedCreateInstance) {
			v.Plan.Workflows[0].FlowFragment.Properties = automation.Properties{over: "value"}
		}},
		{"parameter definition", func(v *ResolvedCreateInstance) {
			v.Plan.Workflows[0].Version.Definition.Parameters = []automation.ParameterDefinition{{Name: "value", DisplayName: over, Type: parameter.Text, Required: true}}
		}},
		{"parameter option", func(v *ResolvedCreateInstance) {
			v.Plan.Workflows[0].Version.Definition.Parameters = []automation.ParameterDefinition{{Name: "value", DisplayName: "Value", Type: parameter.SingleSelect, Required: true, Options: []string{over}}}
		}},
		{"step payload", func(v *ResolvedCreateInstance) { v.Plan.Workflows[0].Version.Definition.Steps[0].Action = over }},
		{"reference binding", func(v *ResolvedCreateInstance) {
			v.Plan.Workflows[0].Version.Definition.Steps[0].Reference.ParameterBindings = map[string]parameter.Binding{"child": parameter.LiteralBinding(parameter.TextValue(over))}
		}},
		{"selector", func(v *ResolvedCreateInstance) { v.Plan.Nodes[0].Version.Selectors[0].Value = over }},
		{"fingerprint attribute", func(v *ResolvedCreateInstance) {
			v.Plan.Nodes[0].Version.Fingerprint.Attributes = map[string]string{"name": over}
		}},
		{"reference resolution", func(v *ResolvedCreateInstance) { v.Plan.References[0].StepID = over }},
		{"invocation value", func(v *ResolvedCreateInstance) {
			v.Invocations[0].Values = map[string]parameter.Value{"value": parameter.TextValue(over)}
		}},
		{"invocation binding", func(v *ResolvedCreateInstance) {
			v.Invocations[0].Bindings = map[string]parameter.Binding{"value": parameter.ParentReferenceBinding(over)}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved := validResolvedCreateInstance(t, command)
			test.mutate(&resolved)
			snapshot, err := BuildInstanceSnapshot(command, resolved)
			if !fault.IsCode(err, CodeCreateInstanceAdapterContractViolation) || snapshot.Digest() != "" {
				t.Fatalf("snapshot/error=%#v/%v", snapshot, err)
			}
		})
	}
}

func TestCreateInstanceServiceAppliesReplaysAndConflicts(t *testing.T) {
	command := validCreateInstanceCommand()
	fake := &createInstanceFake{resolved: validResolvedCreateInstance(t, command)}
	service := mustCreateInstanceService(t, fake)
	created, err := service.CreateInstance(context.Background(), command)
	if err != nil || !created.WasApplied || fake.resolveCalls != 1 || fake.insertCalls != 1 {
		t.Fatalf("created=%#v calls=%d/%d err=%v", created, fake.resolveCalls, fake.insertCalls, err)
	}
	digest, _ := CreateInstanceRequestDigest(command)
	fake.stored = &StoredCreateInstanceCommand{CommandID: command.CommandID, RequestDigest: digest, Result: StoredCreateInstanceResult{Run: created.Run, Snapshot: created.Snapshot, SnapshotDigest: created.Snapshot.Digest(), EntryIDs: created.EntryIDs}}
	replayed, err := service.CreateInstance(context.Background(), command)
	if err != nil || replayed.WasApplied || fake.resolveCalls != 1 || fake.insertCalls != 1 {
		t.Fatalf("replay=%#v calls=%d/%d err=%v", replayed, fake.resolveCalls, fake.insertCalls, err)
	}
	changed := command
	changed.EnvironmentID = "other"
	result, err := service.CreateInstance(context.Background(), changed)
	descriptor, ok := fault.Describe(err)
	if !fault.IsCode(err, CodeCreateInstanceCommandConflict) || !ok ||
		descriptor.Kind() != fault.Conflict ||
		descriptor.Message() != "create-instance command conflicts with an existing request" ||
		len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 ||
		strings.Contains(err.Error(), command.CommandID) ||
		!isZeroCreateInstanceResult(result) {
		t.Fatalf("conflict result/error=%#v/%v", result, err)
	}
}

func TestCreateInstanceServicePreservesTypedErrorCategoriesAndReturnsNoResult(t *testing.T) {
	base := validCreateInstanceCommand()
	tests := []struct {
		name      string
		command   CreateInstanceCommand
		configure func(*createInstanceFake)
		target    error
		wantCode  fault.Code
	}{
		{"invalid command", func() CreateInstanceCommand { value := base; value.InstanceID = execution.InstanceID{}; return value }(), func(*createInstanceFake) {}, nil, CodeCreateInstanceCommandInvalid},
		{"find command", base, func(f *createInstanceFake) { f.findErr = errors.New("read failed") }, nil, CodeSchedulingAdapterUnavailable},
		{"build snapshot", base, func(f *createInstanceFake) { f.resolved.Environment.ID = "other" }, nil, ""},
		{"invalid insert outcome", base, func(f *createInstanceFake) { f.insertOutcome.Status = "UNKNOWN" }, nil, ""},
		{"catalog graph", base, func(f *createInstanceFake) {
			f.resolveErr = createInstanceCatalogGraphUnresolvableError(errors.New("missing child"))
		}, nil, CodeCreateInstanceCatalogGraphUnresolvable},
		{"retryable resolver", base, func(f *createInstanceFake) {
			f.resolveErr = createInstanceRetryableError(errors.New("serialization"))
		}, nil, CodeCreateInstanceRetryable},
		{"retryable insert", base, func(f *createInstanceFake) {
			f.insertErr = createInstanceRetryableError(errors.New("serialization"))
		}, nil, CodeCreateInstanceRetryable},
		{"retryable transaction", base, func(f *createInstanceFake) {
			f.transactionErr = createInstanceRetryableError(errors.New("serialization"))
		}, nil, CodeCreateInstanceRetryable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &createInstanceFake{resolved: validResolvedCreateInstance(t, base)}
			test.configure(fake)
			result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), test.command)
			if err == nil || (test.target != nil && !errors.Is(err, test.target)) || (test.wantCode != "" && !fault.IsCode(err, test.wantCode)) || !isZeroCreateInstanceResult(result) {
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

func isZeroCreateInstanceResult(result CreateInstanceResult) bool {
	return result.Run.ID == (execution.InstanceID{}) && result.Snapshot.Digest() == "" && result.EntryIDs == nil && !result.WasApplied
}

func TestCreateInstanceServiceExposesNoResultWhenTransactionFails(t *testing.T) {
	command := validCreateInstanceCommand()
	fake := &createInstanceFake{resolved: validResolvedCreateInstance(t, command), transactionErr: errors.New("commit failed")}
	result, err := mustCreateInstanceService(t, fake).CreateInstance(context.Background(), command)
	if err == nil || result.Snapshot.Digest() != "" || result.WasApplied {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
