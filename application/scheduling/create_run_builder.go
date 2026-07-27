package scheduling

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func BuildRunSnapshot(command CreateRunCommand, resolved ResolvedCreateRun) (execution.RunSnapshot, error) {
	command = normalizeCreateRunCommand(command)
	if err := preflightResolvedCreateRun(resolved); err != nil {
		return execution.RunSnapshot{}, err
	}
	if err := validateCreateRunCommand(command); err != nil {
		return execution.RunSnapshot{}, err
	}
	if resolved.Plan.Task.ID != command.TestTaskID || resolved.Plan.Version.ID != command.TestTaskVersionID || resolved.Environment.ID != command.EnvironmentID {
		return execution.RunSnapshot{}, errors.New("resolved catalog assets do not match command selectors")
	}
	entries := make([]executionEntryInput, len(resolved.Plan.Version.Items))
	items := make([]execution.TestTaskVersionItemSnapshot, len(resolved.Plan.Version.Items))
	for index, item := range resolved.Plan.Version.Items {
		values, exists := command.Entries[item.ID]
		if !exists {
			return execution.RunSnapshot{}, fmt.Errorf("test-task item %q values are missing", item.ID)
		}
		executionID := concreteRootPath(command.RunID, item.ID)
		resolvedRoot, exists := invocationByPath(resolved.Invocations, executionID)
		if !exists || resolvedRoot.ParentPath != "" {
			return execution.RunSnapshot{}, fmt.Errorf("test-task item %q root invocation is missing", item.ID)
		}
		if err := validateSuppliedRootValues(values, resolvedRoot.WorkflowVersionID, resolved.Plan); err != nil {
			return execution.RunSnapshot{}, fmt.Errorf("test-task item %q values: %w", item.ID, err)
		}
		if err := validateResolvedRootValues(values, resolvedRoot, resolved.Plan); err != nil {
			return execution.RunSnapshot{}, fmt.Errorf("test-task item %q resolution: %w", item.ID, err)
		}
		parameterSnapshot := parameterSnapshotInput{}
		if len(resolvedRoot.Values) > 0 {
			parameterSnapshot = parameterSnapshotInput{IsPresent: true, ID: executionID + "/scope", SchemaVersion: 1, Values: cloneParameterValues(resolvedRoot.Values)}
		}
		entries[index] = executionEntryInput{ExecutionID: executionID, TestTaskItemID: item.ID, SequenceNumber: item.SequenceNumber, WorkflowID: item.WorkflowID, WorkflowVersionID: resolvedWorkflowVersion(item, resolved.Plan), ParameterSnapshot: parameterSnapshot}
		items[index] = execution.TestTaskVersionItemSnapshot{ID: item.ID, TestTaskVersionID: item.TestTaskVersionID, SequenceNumber: item.SequenceNumber, WorkflowID: item.WorkflowID, WorkflowVersionID: entries[index].WorkflowVersionID}
	}
	if len(command.Entries) != len(entries) {
		return execution.RunSnapshot{}, errors.New("command contains unknown test-task item values")
	}
	draft, err := buildExecutionDraft(buildExecutionPlanInput{RunID: command.RunID, Publication: resolved.Plan, Entries: entries})
	if err != nil {
		return execution.RunSnapshot{}, fmt.Errorf("build execution draft: %w", err)
	}
	draft.FailurePolicy = command.FailurePolicy
	invocations := cloneInvocationScopes(resolved.Invocations)
	input := execution.RunSnapshotInput{SchemaVersion: execution.RunSnapshotSchemaCurrent, RunID: command.RunID, TestTaskID: command.TestTaskID, TestTaskVersionID: command.TestTaskVersionID, TestTaskVersionNumber: resolved.Plan.Version.VersionNumber, TestTask: execution.TestTaskSnapshot{ID: resolved.Plan.Task.ID, CurrentVersionID: resolved.Plan.Task.CurrentVersionID}, TestTaskVersion: execution.TestTaskVersionSnapshot{ID: resolved.Plan.Version.ID, TestTaskID: resolved.Plan.Version.TestTaskID, VersionNumber: resolved.Plan.Version.VersionNumber, Items: items}, Plan: draft, Invocations: invocations, Environment: execution.EnvironmentSnapshot{ID: resolved.Environment.ID, DisplayName: resolved.Environment.DisplayName, BaseURL: resolved.Environment.BaseURL, Revision: uint64(resolved.Environment.Revision), Variables: cloneParameterValues(resolved.Environment.Variables)}, FailurePolicy: command.FailurePolicy, ScreenshotPolicy: command.ScreenshotPolicy, HealerPolicy: command.HealerPolicy}
	return execution.SealRunSnapshot(input)
}

func addResolvedValueBudget(budget *createRunRequestBudget, value parameter.Value) error {
	switch value.Type() {
	case parameter.Text:
		return budget.addString(value.Text())
	case parameter.Number:
		return budget.addString(value.Number())
	case parameter.SingleSelect:
		return budget.addString(value.SingleSelect())
	case parameter.MultiSelect:
		count, totalBytes, maxItemBytes, ok := value.MultiSelectMetrics()
		if !ok {
			return errors.New("invalid multi-select payload metrics")
		}
		if err := budget.addElements(count); err != nil {
			return err
		}
		return budget.addStringMetrics(totalBytes, maxItemBytes)
	default:
		return nil
	}
}

func addResolvedValuesBudget(budget *createRunRequestBudget, values map[string]parameter.Value) error {
	if err := budget.addParameters(len(values)); err != nil {
		return err
	}
	for name, value := range values {
		if err := budget.addString(name); err != nil {
			return err
		}
		if err := addResolvedValueBudget(budget, value); err != nil {
			return err
		}
	}
	return nil
}

func addResolvedBindingsBudget(budget *createRunRequestBudget, bindings map[string]parameter.Binding) error {
	if err := budget.addParameters(len(bindings)); err != nil {
		return err
	}
	for name, binding := range bindings {
		if err := budget.addString(name); err != nil {
			return err
		}
		if literal, ok := binding.Literal(); ok {
			if err := addResolvedValueBudget(budget, literal); err != nil {
				return err
			}
		}
		if parentName, ok := binding.ParentName(); ok {
			if err := budget.addString(parentName); err != nil {
				return err
			}
		}
	}
	return nil
}

func preflightResolvedCreateRun(resolved ResolvedCreateRun) error {
	budget := newCreateRunRequestBudget()
	invalid := func(reason string) error {
		return &CreateRunAdapterContractError{Operation: "preflight resolved create run", Reason: reason}
	}
	addStrings := func(values ...string) error {
		for _, value := range values {
			if err := budget.addString(value); err != nil {
				return invalid(err.Error())
			}
		}
		return nil
	}
	if err := budget.addElements(len(resolved.Plan.Version.Items)); err != nil {
		return invalid(err.Error())
	}
	if len(resolved.Plan.Workflows) > execution.MaxDraftWorkflows || len(resolved.Plan.Nodes) > execution.MaxDraftNodes || len(resolved.Plan.References) > execution.MaxDraftReferences {
		return invalid("top-level catalog collection limit exceeded")
	}
	if err := budget.addElements(len(resolved.Plan.Workflows) + len(resolved.Plan.Nodes) + len(resolved.Plan.References) + len(resolved.Invocations) + len(resolved.Environment.Variables)); err != nil {
		return invalid(err.Error())
	}
	if err := addStrings(resolved.Plan.Task.ID, resolved.Plan.Task.DisplayName, resolved.Plan.Task.CurrentVersionID, resolved.Plan.Version.ID, resolved.Plan.Version.TestTaskID, resolved.Environment.ID, resolved.Environment.DisplayName, resolved.Environment.BaseURL); err != nil {
		return err
	}
	for _, item := range resolved.Plan.Version.Items {
		if err := addStrings(item.ID, item.TestTaskVersionID, item.WorkflowID, item.WorkflowVersionID); err != nil {
			return err
		}
		if err := addResolvedValuesBudget(&budget, item.Parameters); err != nil {
			return invalid(err.Error())
		}
	}
	steps := 0
	var walk func([]automation.WorkflowStep, int) error
	walk = func(items []automation.WorkflowStep, depth int) error {
		if depth > execution.MaxStepNestingDepth {
			return invalid("step depth exceeded")
		}
		if len(items) > execution.MaxAggregateSteps-steps {
			return invalid("aggregate steps exceeded")
		}
		steps += len(items)
		if err := budget.addElements(len(items)); err != nil {
			return invalid(err.Error())
		}
		for _, step := range items {
			if err := addStrings(step.ID, step.DisplayName, string(step.Kind), step.Action, step.NodeID, step.NodeVersionID, step.Value, step.WaitKind); err != nil {
				return err
			}
			if err := budget.addElements(len(step.Values)); err != nil {
				return invalid(err.Error())
			}
			if err := addStrings(step.Values...); err != nil {
				return err
			}
			if step.Reference != nil {
				if err := addStrings(step.Reference.WorkflowID, step.Reference.WorkflowVersionID); err != nil {
					return err
				}
				if err := addResolvedBindingsBudget(&budget, step.Reference.ParameterBindings); err != nil {
					return invalid(err.Error())
				}
			}
			if step.Validation != nil {
				assertion := step.Validation.Assertion
				if err := addStrings(string(assertion.Kind), assertion.Expected, assertion.Attribute, step.Validation.Actual); err != nil {
					return err
				}
				if err := budget.addElements(len(assertion.ExpectedValues) + len(step.Validation.SupportedKinds)); err != nil {
					return invalid(err.Error())
				}
				if err := addStrings(assertion.ExpectedValues...); err != nil {
					return err
				}
				for _, kind := range step.Validation.SupportedKinds {
					if err := addStrings(string(kind)); err != nil {
						return err
					}
				}
			}
			if step.ValidationGroup != nil {
				if err := budget.addElements(len(step.ValidationGroup.Branches)); err != nil {
					return invalid(err.Error())
				}
				for _, branch := range step.ValidationGroup.Branches {
					if err := addStrings(branch.ID, branch.Name); err != nil {
						return err
					}
					if err := walk(branch.Steps, depth+1); err != nil {
						return err
					}
				}
			}
			if err := walk(step.Children, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, workflow := range resolved.Plan.Workflows {
		if err := addStrings(workflow.Workflow.ID, workflow.Workflow.DisplayName, workflow.Workflow.FolderID, workflow.Workflow.CurrentVersionID, workflow.Version.ID, workflow.Version.WorkflowID); err != nil {
			return err
		}
		if err := budget.addElements(len(workflow.Workflow.Properties)); err != nil {
			return invalid(err.Error())
		}
		for key, value := range workflow.Workflow.Properties {
			if err := addStrings(key, value); err != nil {
				return err
			}
		}
		if err := budget.addParameters(len(workflow.Version.Definition.Parameters)); err != nil {
			return invalid(err.Error())
		}
		for _, definition := range workflow.Version.Definition.Parameters {
			if err := addStrings(definition.Name, definition.DisplayName, definition.Description, string(definition.Type)); err != nil {
				return err
			}
			if err := budget.addElements(len(definition.Options)); err != nil {
				return invalid(err.Error())
			}
			if err := addStrings(definition.Options...); err != nil {
				return err
			}
			if value, present := definition.Default.Value(); present {
				if err := addResolvedValueBudget(&budget, value); err != nil {
					return invalid(err.Error())
				}
			}
		}
		if err := walk(workflow.Version.Definition.Steps, 0); err != nil {
			return err
		}
	}
	for _, node := range resolved.Plan.Nodes {
		fingerprint := node.Version.Fingerprint
		if err := addStrings(node.Node.ID, node.Node.DisplayName, node.Node.CurrentVersionID, node.Version.ID, node.Version.NodeID, node.Version.PageURL, node.Version.Origin, fingerprint.Tag, fingerprint.Text, fingerprint.ARIA.Role, fingerprint.ARIA.Name, fingerprint.Neighbors.Prev, fingerprint.Neighbors.Next, fingerprint.Neighbors.ParentTag, fingerprint.LabelText, fingerprint.FormID); err != nil {
			return err
		}
		if err := budget.addElements(len(node.Version.Selectors) + len(fingerprint.Attributes) + len(fingerprint.Path) + len(fingerprint.Framework)); err != nil {
			return invalid(err.Error())
		}
		for _, selector := range node.Version.Selectors {
			if err := addStrings(string(selector.Type), selector.Value); err != nil {
				return err
			}
		}
		for key, value := range fingerprint.Attributes {
			if err := addStrings(key, value); err != nil {
				return err
			}
		}
		if err := addStrings(fingerprint.Path...); err != nil {
			return err
		}
		for _, framework := range fingerprint.Framework {
			if err := addStrings(string(framework.Kind), framework.Version, string(framework.Evidence)); err != nil {
				return err
			}
		}
	}
	for _, reference := range resolved.Plan.References {
		if err := addStrings(reference.ParentWorkflowVersionID, reference.StepID, reference.WorkflowID, reference.WorkflowVersionID); err != nil {
			return err
		}
	}
	for name, value := range resolved.Environment.Variables {
		if strings.TrimSpace(name) == "" {
			return invalid("environment variable name is required")
		}
		if err := value.Validate(); err != nil {
			return invalid(fmt.Sprintf("environment variable %q: %v", name, err))
		}
		if err := budget.addString(name); err != nil {
			return invalid(err.Error())
		}
		if err := addResolvedValueBudget(&budget, value); err != nil {
			return invalid(err.Error())
		}
	}
	for _, invocation := range resolved.Invocations {
		if err := addStrings(invocation.Path, invocation.ParentPath, invocation.ParentVersionID, invocation.StepID, invocation.WorkflowID, invocation.WorkflowVersionID); err != nil {
			return err
		}
		if err := addResolvedValuesBudget(&budget, invocation.Values); err != nil {
			return invalid(err.Error())
		}
		if err := addResolvedBindingsBudget(&budget, invocation.Bindings); err != nil {
			return invalid(err.Error())
		}
	}
	return nil
}

func concreteRootPath(runID, itemID string) string {
	return fmt.Sprintf("%d:%s%d:%s", len(runID), runID, len(itemID), itemID)
}

func resolvedWorkflowVersion(item automation.TestTaskItem, plan automation.TestTaskVersionPlan) string {
	if item.VersionPolicy == automation.WorkflowVersionFixed {
		return item.WorkflowVersionID
	}
	for _, dependency := range plan.Workflows {
		if dependency.Workflow.ID == item.WorkflowID && dependency.ResolvedFromLatest {
			return dependency.Version.ID
		}
	}
	return ""
}

func normalizeCreateRunCommand(command CreateRunCommand) CreateRunCommand {
	if command.FailurePolicy == "" {
		command.FailurePolicy = execution.FailurePolicyStopOnFailure
	}
	normalizeZero := func(value *float64) {
		if *value == 0 {
			*value = 0
		}
	}
	for _, value := range []*float64{
		&command.HealerPolicy.ReviewCap, &command.HealerPolicy.AppliedCap,
		&command.HealerPolicy.Weights.Tag, &command.HealerPolicy.Weights.ID,
		&command.HealerPolicy.Weights.RoleName, &command.HealerPolicy.Weights.Class,
		&command.HealerPolicy.Weights.Attrs, &command.HealerPolicy.Weights.Text,
		&command.HealerPolicy.Weights.Index, &command.HealerPolicy.Weights.Neighbor,
		&command.HealerPolicy.Weights.LabelText, &command.HealerPolicy.Weights.Container,
	} {
		normalizeZero(value)
	}
	return command
}

func validateCreateRunCommand(command CreateRunCommand) (resultErr error) {
	defer func() {
		if resultErr != nil && !errors.Is(resultErr, ErrInvalidCreateRunCommand) {
			resultErr = errors.Join(ErrInvalidCreateRunCommand, resultErr)
		}
	}()
	for name, value := range map[string]string{"command id": command.CommandID, "run id": command.RunID, "test-task id": command.TestTaskID, "test-task version id": command.TestTaskVersionID, "environment id": command.EnvironmentID} {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("%s is required and must be normalized", name)
		}
	}
	if command.CreatedAt <= 0 || !command.FailurePolicy.IsValid() {
		return errors.New("created time and failure policy are required")
	}
	if command.ScreenshotPolicy.Version != execution.ScreenshotPolicyV1 || strings.TrimSpace(command.ScreenshotPolicy.Destination) == "" || command.HealerPolicy.Version != execution.HealerPolicyV1 {
		return errors.New("screenshot and healer policies are invalid")
	}
	for _, value := range []float64{command.HealerPolicy.ReviewCap, command.HealerPolicy.AppliedCap, command.HealerPolicy.Weights.Tag, command.HealerPolicy.Weights.ID, command.HealerPolicy.Weights.RoleName, command.HealerPolicy.Weights.Class, command.HealerPolicy.Weights.Attrs, command.HealerPolicy.Weights.Text, command.HealerPolicy.Weights.Index, command.HealerPolicy.Weights.Neighbor, command.HealerPolicy.Weights.LabelText, command.HealerPolicy.Weights.Container} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("healer policy contains a non-finite value")
		}
	}
	for itemID, values := range command.Entries {
		if strings.TrimSpace(itemID) == "" || itemID != strings.TrimSpace(itemID) {
			return errors.New("test-task item id is invalid")
		}
		for _, value := range values {
			if err := value.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSuppliedRootValues(values map[string]parameter.Value, versionID string, plan automation.TestTaskVersionPlan) error {
	definitions := map[string]automation.ParameterDefinition{}
	for _, workflow := range plan.Workflows {
		if workflow.Version.ID == versionID {
			for _, definition := range workflow.Version.Definition.Parameters {
				definitions[definition.Name] = definition
			}
			break
		}
	}
	for name, value := range values {
		definition, exists := definitions[name]
		if !exists {
			return fmt.Errorf("unknown parameter %q", name)
		}
		if err := (parameter.Constraint{Type: definition.Type, Options: append([]string(nil), definition.Options...)}).Validate(value); err != nil {
			return fmt.Errorf("parameter %q: %w", name, err)
		}
	}
	return nil
}

func validateResolvedRootValues(values map[string]parameter.Value, invocation execution.InvocationScopeSnapshot, plan automation.TestTaskVersionPlan) error {
	for _, workflow := range plan.Workflows {
		if workflow.Version.ID != invocation.WorkflowVersionID {
			continue
		}
		resolved, err := automation.ResolveParameterValues(workflow.Version.Definition.Parameters, values)
		if err != nil {
			return err
		}
		if !equalParameterValues(resolved, invocation.Values) {
			return errors.New("resolver values do not match supplied values and defaults")
		}
		return nil
	}
	return fmt.Errorf("workflow version %q is missing", invocation.WorkflowVersionID)
}

func invocationByPath(invocations []execution.InvocationScopeSnapshot, path string) (execution.InvocationScopeSnapshot, bool) {
	for _, invocation := range invocations {
		if invocation.Path == path {
			return invocation, true
		}
	}
	return execution.InvocationScopeSnapshot{}, false
}

func cloneProperties(values automation.Properties) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneInvocationScopes(values []execution.InvocationScopeSnapshot) []execution.InvocationScopeSnapshot {
	result := make([]execution.InvocationScopeSnapshot, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Values = cloneParameterValues(value.Values)
		result[index].Bindings = make(map[string]parameter.Binding, len(value.Bindings))
		for name, binding := range value.Bindings {
			result[index].Bindings[name] = binding.Clone()
		}
	}
	return result
}
