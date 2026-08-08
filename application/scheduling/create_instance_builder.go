package scheduling

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// BuildInstanceSnapshot 将命令与同一目录视图解析结果组装为封存的 execution.InstanceSnapshot。
func BuildInstanceSnapshot(command CreateInstanceCommand, resolved ResolvedCreateInstance) (execution.InstanceSnapshot, error) {
	command = normalizeCreateInstanceCommand(command)
	if err := preflightResolvedCreateInstance(resolved); err != nil {
		return execution.InstanceSnapshot{}, err
	}
	if err := validateCreateInstanceCommand(command); err != nil {
		return execution.InstanceSnapshot{}, err
	}
	if resolved.Plan.Task.ID != command.ExecutionFlowID || resolved.Plan.Version.ID != command.TestTaskVersionID || resolved.Environment.ID != command.EnvironmentID {
		return execution.InstanceSnapshot{}, createInstanceCatalogGraphUnresolvableError(errors.New("resolved catalog assets do not match command selectors"))
	}
	entries := make([]executionEntryInput, len(resolved.Plan.Version.Items))
	items := make([]execution.ExecutionFlowVersionItemSnapshot, len(resolved.Plan.Version.Items))
	for index, item := range resolved.Plan.Version.Items {
		values, exists := command.Entries[item.ID]
		if !exists {
			// item ID 仅保留在私有 cause 中，不进入公共文本。
			return execution.InstanceSnapshot{}, createInstanceCatalogGraphUnresolvableError(fmt.Errorf("test-task item %q values are missing", item.ID))
		}
		// 入口身份由实例 ID 和条目 ID 推导，按构造应始终有效；此处失败表示 concreteRootPath 自身错误，
		// 而非调用方输入错误。
		spelledEntry := concreteRootPath(command.InstanceID.String(), item.ID)
		entryID, err := execution.NewEntryID(spelledEntry)
		if err != nil {
			return execution.InstanceSnapshot{}, createInstanceCatalogGraphUnresolvableError(err)
		}
		resolvedRoot, exists := invocationByPath(resolved.Invocations, execution.RootInvocationPath(entryID))
		if !exists || resolvedRoot.ParentPath != (execution.InvocationPath{}) {
			return execution.InstanceSnapshot{}, createInstanceCatalogGraphUnresolvableError(fmt.Errorf("test-task item %q root invocation is missing", item.ID))
		}
		if err := validateSuppliedRootValues(values, resolvedRoot.WorkflowVersionID, resolved.Plan); err != nil {
			// Constraint.Validate 已返回 PARAMETER_CONSTRAINT_UNSATISFIED 或 PARAMETER_VALUE_INVALID；
			// 此处保持原错误，不在包装中回显条目 ID。
			return execution.InstanceSnapshot{}, err
		}
		if err := validateResolvedRootValues(values, resolvedRoot, resolved.Plan); err != nil {
			return execution.InstanceSnapshot{}, fmt.Errorf("test-task item %q resolution: %w", item.ID, err)
		}
		parameterSnapshot := parameterSnapshotInput{}
		if len(resolvedRoot.Values) > 0 {
			parameterSnapshot = parameterSnapshotInput{IsPresent: true, ID: spelledEntry + "/scope", SchemaVersion: 1, Values: cloneParameterValues(resolvedRoot.Values)}
		}
		entries[index] = executionEntryInput{EntryID: entryID, TestTaskItemID: item.ID, SequenceNumber: item.SequenceNumber, FlowFragmentID: item.FlowFragmentID, WorkflowVersionID: resolvedWorkflowVersion(item, resolved.Plan), ParameterSnapshot: parameterSnapshot}
		items[index] = execution.ExecutionFlowVersionItemSnapshot{ID: item.ID, TestTaskVersionID: item.TestTaskVersionID, SequenceNumber: item.SequenceNumber, FlowFragmentID: item.FlowFragmentID, WorkflowVersionID: entries[index].WorkflowVersionID}
	}
	if len(command.Entries) != len(entries) {
		return execution.InstanceSnapshot{}, createInstanceCatalogGraphUnresolvableError(errors.New("command contains unknown test-task item values"))
	}
	draft, err := buildExecutionDraft(buildExecutionPlanInput{InstanceID: command.InstanceID, Publication: resolved.Plan, Entries: entries})
	if err != nil {
		// 这是整个草稿构建树的边界。内部不变量检查按契约保持普通 Go 错误，在此处统一分类，而不是
		// 在内部十余个位置分别分类。已有错误码的错误原样通过，避免嵌套两个 fault 并迫使宿主解包分类。
		return execution.InstanceSnapshot{}, classifyCatalogGraphFailure(err)
	}
	draft.FailurePolicy = command.FailurePolicy
	invocations := cloneInvocationScopes(resolved.Invocations)
	input := execution.InstanceSnapshotInput{SchemaVersion: execution.InstanceSnapshotSchemaCurrent, InstanceID: command.InstanceID, ExecutionFlowID: command.ExecutionFlowID, TestTaskVersionID: command.TestTaskVersionID, TestTaskVersionNumber: resolved.Plan.Version.VersionNumber, ExecutionFlow: execution.TestTaskSnapshot{ID: resolved.Plan.Task.ID}, ExecutionFlowVersion: execution.ExecutionFlowVersionSnapshot{ID: resolved.Plan.Version.ID, ExecutionFlowID: resolved.Plan.Version.ExecutionFlowID, VersionNumber: resolved.Plan.Version.VersionNumber, Items: items}, Plan: draft, Invocations: invocations, Environment: execution.EnvironmentSnapshot{ID: resolved.Environment.ID, DisplayName: resolved.Environment.DisplayName, BaseURL: resolved.Environment.BaseURL, Revision: uint64(resolved.Environment.Revision), Variables: cloneParameterValues(resolved.Environment.Variables)}, FailurePolicy: command.FailurePolicy, ScreenshotPolicy: command.ScreenshotPolicy, HealerPolicy: command.HealerPolicy}
	return execution.SealInstanceSnapshot(input)
}

// addResolvedValueBudget 按参数类型从预算中扣除解析值的字符串和元素大小。
func addResolvedValueBudget(budget *createInstanceRequestBudget, value parameter.Value) error {
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

// addResolvedValuesBudget 扣除参数映射及其排序后的名称和值预算。
func addResolvedValuesBudget(budget *createInstanceRequestBudget, values map[string]parameter.Value) error {
	if err := budget.addParameters(len(values)); err != nil {
		return err
	}
	for _, name := range sortedKeys(values) {
		if err := budget.addString(name); err != nil {
			return err
		}
		if err := addResolvedValueBudget(budget, values[name]); err != nil {
			return err
		}
	}
	return nil
}

// addResolvedBindingsBudget 扣除绑定映射、名称、字面值和父参数名预算。
func addResolvedBindingsBudget(budget *createInstanceRequestBudget, bindings map[string]parameter.Binding) error {
	if err := budget.addParameters(len(bindings)); err != nil {
		return err
	}
	for _, name := range sortedKeys(bindings) {
		binding := bindings[name]
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

// preflightResolvedCreateInstance 预先遍历解析目录，校验集合、深度、字符串和参数预算。
func preflightResolvedCreateInstance(resolved ResolvedCreateInstance) error {
	budget := newCreateInstanceRequestBudget()
	invalid := func(reason string) error {
		return createInstanceAdapterContractViolationError(errors.New(reason))
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
	if err := addStrings(resolved.Plan.Task.ID, resolved.Plan.Task.DisplayName, resolved.Plan.Version.ID, resolved.Plan.Version.ExecutionFlowID, resolved.Environment.ID, resolved.Environment.DisplayName, resolved.Environment.BaseURL); err != nil {
		return err
	}
	for _, item := range resolved.Plan.Version.Items {
		if err := addStrings(item.ID, item.TestTaskVersionID, item.FlowFragmentID, item.WorkflowVersionID); err != nil {
			return err
		}
		if err := addResolvedValuesBudget(&budget, item.Parameters); err != nil {
			return invalid(err.Error())
		}
	}
	steps := 0
	var walk func([]automation.FlowFragmentStep, int) error
	walk = func(items []automation.FlowFragmentStep, depth int) error {
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
			if err := addStrings(step.ID, step.DisplayName, string(step.Kind), step.Action, step.ElementTargetID, step.ElementTargetVersionID, step.Value, step.WaitKind); err != nil {
				return err
			}
			if err := budget.addElements(len(step.Values)); err != nil {
				return invalid(err.Error())
			}
			if err := addStrings(step.Values...); err != nil {
				return err
			}
			if step.Reference != nil {
				if err := addStrings(step.Reference.FlowFragmentID, step.Reference.WorkflowVersionID); err != nil {
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
		if err := addStrings(workflow.FlowFragment.ID, workflow.FlowFragment.DisplayName, workflow.FlowFragment.FolderID, workflow.FlowFragment.CurrentVersionID, workflow.Version.ID, workflow.Version.FlowFragmentID); err != nil {
			return err
		}
		if err := budget.addElements(len(workflow.FlowFragment.Properties)); err != nil {
			return invalid(err.Error())
		}
		for _, key := range sortedKeys(workflow.FlowFragment.Properties) {
			if err := addStrings(key, workflow.FlowFragment.Properties[key]); err != nil {
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
		if err := addStrings(node.ElementTarget.ID, node.ElementTarget.DisplayName, node.ElementTarget.CurrentVersionID, node.Version.ID, node.Version.ElementTargetID, node.Version.PageURL, node.Version.Origin, fingerprint.Tag, fingerprint.Text, fingerprint.ARIA.Role, fingerprint.ARIA.Name, fingerprint.Neighbors.Prev, fingerprint.Neighbors.Next, fingerprint.Neighbors.ParentTag, fingerprint.LabelText, fingerprint.FormID); err != nil {
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
		for _, key := range sortedKeys(fingerprint.Attributes) {
			if err := addStrings(key, fingerprint.Attributes[key]); err != nil {
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
		if err := addStrings(reference.ParentFlowFragmentVersionID, reference.StepID, reference.FlowFragmentID, reference.WorkflowVersionID); err != nil {
			return err
		}
	}
	for _, name := range sortedKeys(resolved.Environment.Variables) {
		if err := parameter.ValidateName(name); err != nil {
			return invalid(fmt.Sprintf("environment variable name: %v", err))
		}
		if err := resolved.Environment.Variables[name].Validate(); err != nil {
			return invalid(fmt.Sprintf("environment variable %q: %v", name, err))
		}
		if err := budget.addString(name); err != nil {
			return invalid(err.Error())
		}
		if err := addResolvedValueBudget(&budget, resolved.Environment.Variables[name]); err != nil {
			return invalid(err.Error())
		}
	}
	for _, invocation := range resolved.Invocations {
		if err := addStrings(invocation.Path.String(), invocation.ParentPath.String(), invocation.ParentVersionID, invocation.StepID, invocation.FlowFragmentID, invocation.WorkflowVersionID); err != nil {
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

// concreteRootPath 构造实例条目根调用的稳定路径。
func concreteRootPath(instanceID, itemID string) string {
	return fmt.Sprintf("%d:%s%d:%s", len(instanceID), instanceID, len(itemID), itemID)
}

// resolvedWorkflowVersion 解析条目使用的工作流版本 ID，保留锁定的版本选择。
func resolvedWorkflowVersion(item automation.ExecutionFlowItem, plan automation.ResolvedExecutionFlow) string {
	if item.VersionPolicy == automation.FlowFragmentVersionFixed {
		return item.WorkflowVersionID
	}
	for _, dependency := range plan.Workflows {
		if dependency.FlowFragment.ID == item.FlowFragmentID && dependency.ResolvedFromLatest {
			return dependency.Version.ID
		}
	}
	return ""
}

// normalizeCreateInstanceCommand 规范化创建命令中的策略和参数映射表示。
func normalizeCreateInstanceCommand(command CreateInstanceCommand) CreateInstanceCommand {
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
		&command.HealerPolicy.Weights.Framework,
	} {
		normalizeZero(value)
	}
	return command
}

// validateCreateInstanceCommand 按固定顺序校验创建命令身份、策略和条目参数，并统一分类错误。
func validateCreateInstanceCommand(command CreateInstanceCommand) (resultErr error) {
	defer func() {
		if resultErr != nil && !fault.IsCode(resultErr, CodeCreateInstanceCommandInvalid) {
			resultErr = createInstanceCommandInvalidError(resultErr)
		}
	}()
	if command.InstanceID.Validate() != nil {
		return errors.New("instance id is required and must be normalized")
	}
	// 使用有序切片而非 map，使多个空身份始终按固定顺序报告，诊断日志不会因 map 遍历顺序变化而变化。
	for _, required := range []struct{ name, value string }{
		{"command id", command.CommandID},
		{"test-task id", command.ExecutionFlowID},
		{"test-task version id", command.TestTaskVersionID},
		{"environment id", command.EnvironmentID},
	} {
		if strings.TrimSpace(required.value) == "" || required.value != strings.TrimSpace(required.value) {
			return fmt.Errorf("%s is required and must be normalized", required.name)
		}
	}
	if command.CreatedAt <= 0 || !command.FailurePolicy.IsValid() {
		return errors.New("created time and failure policy are required")
	}
	if command.ScreenshotPolicy.Version != execution.ScreenshotPolicyV1 || strings.TrimSpace(command.ScreenshotPolicy.Destination) == "" || command.HealerPolicy.Version != execution.HealerPolicyV1 {
		return errors.New("screenshot and healer policies are invalid")
	}
	for _, value := range []float64{command.HealerPolicy.ReviewCap, command.HealerPolicy.AppliedCap, command.HealerPolicy.Weights.Tag, command.HealerPolicy.Weights.ID, command.HealerPolicy.Weights.RoleName, command.HealerPolicy.Weights.Class, command.HealerPolicy.Weights.Attrs, command.HealerPolicy.Weights.Text, command.HealerPolicy.Weights.Index, command.HealerPolicy.Weights.Neighbor, command.HealerPolicy.Weights.LabelText, command.HealerPolicy.Weights.Container, command.HealerPolicy.Weights.Framework} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("healer policy contains a non-finite value")
		}
	}
	// 以下两个分支返回不同 KIND 的错误：条目 ID 形状错误是未分类错误，参数值错误携带参数自身错误码。
	// 外层包装都使用 CodeCreateInstanceCommandInvalid，但 fault.IsCode 会遍历错误链；若两种错误同时
	// 存在，调用方看到的 parameter.CodeValueInvalid 必须不受遍历顺序影响。
	for _, itemID := range sortedKeys(command.Entries) {
		if strings.TrimSpace(itemID) == "" || itemID != strings.TrimSpace(itemID) {
			return errors.New("test-task item id is invalid")
		}
		values := command.Entries[itemID]
		for _, name := range sortedKeys(values) {
			if err := values[name].Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateSuppliedRootValues 按排序后的名称校验调用方参数存在且符合工作流定义约束。
func validateSuppliedRootValues(values map[string]parameter.Value, versionID string, plan automation.ResolvedExecutionFlow) error {
	definitions := map[string]automation.ParameterDefinition{}
	for _, workflow := range plan.Workflows {
		if workflow.Version.ID == versionID {
			for _, definition := range workflow.Version.Definition.Parameters {
				definitions[definition.Name] = definition
			}
			break
		}
	}
	// 按名称排序而非 map 顺序。以下两个分支在遇到首个违规时返回，且返回不同 KIND 的错误：未知名称是
	// 未分类错误，约束失败携带参数自身错误码。即使同时存在两种错误，字节相同的输入也必须得到稳定
	// 错误码；下游 validateSnapshotValues 和 ResolveParameterValues 已有固定顺序，此处也保持一致。
	for _, name := range sortedKeys(values) {
		definition, exists := definitions[name]
		if !exists {
			return fmt.Errorf("unknown parameter %q", name)
		}
		if err := (parameter.Constraint{Type: definition.Type, Options: append([]string(nil), definition.Options...)}).Validate(values[name]); err != nil {
			return fmt.Errorf("parameter %q: %w", name, err)
		}
	}
	return nil
}

// validateResolvedRootValues 校验解析器根据调用方值和默认值生成的结果与事务解析值一致。
func validateResolvedRootValues(values map[string]parameter.Value, invocation execution.InvocationScopeSnapshot, plan automation.ResolvedExecutionFlow) error {
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

// invocationByPath 按调用路径查找解析后的调用范围快照。
func invocationByPath(invocations []execution.InvocationScopeSnapshot, path execution.InvocationPath) (execution.InvocationScopeSnapshot, bool) {
	for _, invocation := range invocations {
		if invocation.Path == path {
			return invocation, true
		}
	}
	return execution.InvocationScopeSnapshot{}, false
}

// cloneProperties 复制自动化属性映射，返回调用方独立拥有的 map。
func cloneProperties(values automation.Properties) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

// cloneInvocationScopes 深拷贝调用范围及其参数值、参数绑定。
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
