package automation

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/interpolation"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// Validate 校验执行流程的每个字段，并通过一个聚合错误按字段顺序返回所有违规。
// 字段路径保持逻辑且与语言环境无关，项目索引从零开始对应调用方传入的切片。
func (t ExecutionFlow) Validate() error {
	var violations []fault.Violation
	if strings.TrimSpace(t.ID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "id", "execution flow id is required"))
	}
	if strings.TrimSpace(t.DisplayName) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "displayName", "execution flow display name is required"))
	}
	if strings.TrimSpace(t.CurrentVersionID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "currentVersionId", "execution flow current version id is required"))
	}
	if t.CreatedAt <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "createdAt", "execution flow created timestamp is required"))
	}
	if t.UpdatedAt <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "updatedAt", "execution flow updated timestamp is required"))
	}
	if len(violations) > 0 {
		return executionFlowInvalidError(violations)
	}
	return nil
}

// Validate 校验执行流程版本的每个字段，并将子校验失败降级为该版本的违规而不嵌套错误。
// 身份、键和值域枚举不会写入公共文本。
func (v ExecutionFlowVersion) Validate() error {
	var violations []fault.Violation
	if strings.TrimSpace(v.ID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "id", "execution flow version id is required"))
	}
	if strings.TrimSpace(v.ExecutionFlowID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "executionFlowId", "execution flow version owner id is required"))
	}
	if v.VersionNumber < 1 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "versionNumber", "execution flow version number must be positive"))
	}
	if v.CreatedAt <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "createdAt", "execution flow version created timestamp is required"))
	}
	if !v.FailurePolicy.IsValid() {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "failurePolicy", "execution flow version failure policy is not supported"))
	}
	if len(v.Items) == 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "items", "execution flow version requires at least one item"))
	}
	seenEnvironmentKeys := map[string]bool{}
	for index, key := range v.RequiredEnvironmentKeys {
		field := fmt.Sprintf("requiredEnvironmentKeys.%d", index)
		switch {
		case strings.TrimSpace(key) == "":
			violations = append(violations, mustViolation(fault.CodeFieldRequired, field, "required environment key must not be blank"))
		case seenEnvironmentKeys[key]:
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field, "required environment key is duplicated"))
		}
		seenEnvironmentKeys[key] = true
	}
	seenIDs := map[string]bool{}
	seenSequences := map[int]bool{}
	for index, item := range v.Items {
		field := func(name string) string { return fmt.Sprintf("items.%d.%s", index, name) }
		switch {
		case strings.TrimSpace(item.ID) == "":
			violations = append(violations, mustViolation(fault.CodeFieldRequired, field("id"), "execution flow item id is required"))
		case seenIDs[item.ID]:
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field("id"), "execution flow item id is duplicated"))
		}
		seenIDs[item.ID] = true
		if item.TestTaskVersionID != v.ID {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field("executionFlowVersionId"), "execution flow item belongs to another version"))
		}
		if item.SequenceNumber != index+1 || seenSequences[item.SequenceNumber] {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, field("sequenceNumber"), "execution flow item sequence numbers must be unique and contiguous from 1"))
		}
		seenSequences[item.SequenceNumber] = true
		if strings.TrimSpace(item.FlowFragmentID) == "" {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, field("flowFragmentId"), "execution flow item flow fragment id is required"))
		}
		if item.VersionPolicy.Validate() != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, field("versionPolicy"), "execution flow item version policy is not supported"))
		}
		switch item.VersionPolicy {
		case FlowFragmentVersionFixed:
			if strings.TrimSpace(item.WorkflowVersionID) == "" {
				violations = append(violations, mustViolation(fault.CodeFieldRequired, field("flowFragmentVersionId"), "fixed version policy requires a flow fragment version id"))
			}
		case FlowFragmentVersionLatest:
			if strings.TrimSpace(item.WorkflowVersionID) != "" {
				violations = append(violations, mustViolation(fault.CodeFieldMismatch, field("flowFragmentVersionId"), "latest version policy must not persist a flow fragment version id"))
			}
		}
	}
	if len(violations) > 0 {
		return executionFlowInvalidError(violations)
	}
	return nil
}

// Validate 校验版本历史的顺序、唯一性和来源链。
// 单个版本的形状错误保持自身错误码并直接返回，不嵌套到历史信封；版本索引从零开始，版本身份不写入公共文本。
func (a ExecutionFlowAggregate) Validate() error {
	if err := a.Task.Validate(); err != nil {
		return err
	}
	// 单个版本的形状错误保持自身错误码并直接返回；先完成所有版本形状校验，再收集历史违规。
	// 这样可避免在后续版本返回错误时丢弃已经构建的历史违规列表。
	for _, version := range a.Versions {
		if err := version.Validate(); err != nil {
			return err
		}
	}
	var violations []fault.Violation
	if len(a.Versions) == 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "versions", "execution flow requires version history"))
	}
	seenIDs := map[string]bool{}
	seenNumbers := map[int]bool{}
	byID := map[string]ExecutionFlowVersion{}
	highest := ExecutionFlowVersion{}
	for index, version := range a.Versions {
		field := func(name string) string { return fmt.Sprintf("versions.%d.%s", index, name) }
		if version.ExecutionFlowID != a.Task.ID {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field("executionFlowId"), "version belongs to another execution flow"))
		}
		if seenIDs[version.ID] || seenNumbers[version.VersionNumber] {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, fmt.Sprintf("versions.%d", index), "version identity is duplicated in history"))
		}
		seenIDs[version.ID] = true
		seenNumbers[version.VersionNumber] = true
		byID[version.ID] = version
		if version.VersionNumber > highest.VersionNumber {
			highest = version
		}
	}
	for number := 1; number <= len(a.Versions); number++ {
		if !seenNumbers[number] {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "versions", "version numbers must be contiguous from 1"))
			break
		}
	}
	for index, version := range a.Versions {
		field := fmt.Sprintf("versions.%d.sourceVersionId", index)
		if version.VersionNumber == 1 {
			if version.SourceVersionID != "" {
				violations = append(violations, mustViolation(fault.CodeFieldMismatch, field, "the first version must not declare a source version"))
			}
			continue
		}
		source, ok := byID[version.SourceVersionID]
		if !ok || source.VersionNumber >= version.VersionNumber {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field, "a version's source must be an earlier version"))
		}
	}
	if a.Current.ID != a.Task.CurrentVersionID || a.Current.ID != highest.ID {
		violations = append(violations, mustViolation(fault.CodeFieldMismatch, "current.id", "current version must be the latest history version"))
	} else if !reflect.DeepEqual(a.Current, highest) {
		violations = append(violations, mustViolation(fault.CodeFieldMismatch, "current", "current version content must match history"))
	}
	if len(violations) != 0 {
		return executionFlowHistoryInvalidError(violations)
	}
	return nil
}

// ResolveParameterValues 解析参数值并通过一个依赖错误信封按顺序返回所有违规。
// 该函数可由宿主调用，不能返回未分类错误；参数名和输入映射键属于调用方数据，仅保留在私有原因中。
// 定义侧失败遵循定义切片顺序，未知键先排序后报告，确保结果不依赖映射迭代顺序。
func ResolveParameterValues(definitions []ParameterDefinition, supplied map[string]parameter.Value) (map[string]parameter.Value, error) {
	var violations []fault.Violation
	var details []string
	byName := make(map[string]ParameterDefinition, len(definitions))
	for index, definition := range definitions {
		field := fmt.Sprintf("parameters.definitions.%d", index)
		// 定义自身的失败降级为当前依赖信封而不嵌套；名称和选项值仅保留在私有详情中。
		if err := definition.Validate(); err != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, field, "parameter definition is invalid"))
			details = append(details, fmt.Sprintf("definition %d (%s): %v", index, definition.Name, err))
			continue
		}
		if _, duplicate := byName[definition.Name]; duplicate {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field, "parameter definition name is duplicated"))
			details = append(details, fmt.Sprintf("definition %d duplicates %q", index, definition.Name))
			continue
		}
		byName[definition.Name] = definition
	}
	unknown := make([]string, 0, len(supplied))
	for name := range supplied {
		if _, exists := byName[name]; !exists {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "parameters", "one or more supplied parameters are not declared"))
		details = append(details, fmt.Sprintf("undeclared parameters: %q", unknown))
	}
	resolved := make(map[string]parameter.Value, len(definitions))
	for index, definition := range definitions {
		field := fmt.Sprintf("parameters.definitions.%d", index)
		value, exists := supplied[definition.Name]
		if !exists {
			fallback, present := definition.Default.Value()
			if !present {
				violations = append(violations, mustViolation(fault.CodeFieldRequired, field, "a required parameter was not supplied"))
				details = append(details, fmt.Sprintf("parameter %q is required", definition.Name))
				continue
			}
			value = fallback
		}
		if err := definition.ValidateValue(value); err != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, field, "a supplied parameter value is incompatible with its declaration"))
			details = append(details, fmt.Sprintf("parameter %q: %v", definition.Name, err))
			continue
		}
		resolved[definition.Name] = value.Clone()
	}
	if len(violations) > 0 {
		return nil, wrapAutomationFault(
			errors.New(strings.Join(details, "; ")),
			fault.InvalidArgument,
			CodeExecutionFlowDependencyInvalid,
			"execution flow dependency resolution is invalid",
			fault.WithViolations(capViolations(violations)...),
		)
	}
	return resolved, nil
}

// validateReferenceBindings 校验流程片段引用的父子参数绑定及其类型一致性。
func validateReferenceBindings(parent, child []ParameterDefinition, bindings map[string]parameter.Binding) error {
	parents := map[string]ParameterDefinition{}
	for _, definition := range parent {
		parents[definition.Name] = definition
	}
	for _, definition := range child {
		binding, exists := bindings[definition.Name]
		if !exists {
			if _, hasDefault := definition.Default.Value(); hasDefault {
				continue
			}
			return fmt.Errorf("parameter %q is required", definition.Name)
		}
		if literal, ok := binding.Literal(); ok {
			if err := definition.ValidateValue(literal); err != nil {
				return fmt.Errorf("parameter %q: %w", definition.Name, err)
			}
			continue
		}
		name, ok := binding.ParentName()
		if !ok || name == "" {
			return fmt.Errorf("parameter %q has invalid binding", definition.Name)
		}
		parentDefinition, exists := parents[name]
		if !exists {
			return fmt.Errorf("parent parameter %q is missing", name)
		}
		if parentDefinition.Type != definition.Type {
			return fmt.Errorf("parameter %q parent reference type mismatch", definition.Name)
		}
	}
	// 按绑定名称排序后报告未知键，使结果不依赖映射迭代顺序。
	unknown := make([]string, 0, len(bindings))
	for name := range bindings {
		found := false
		for _, definition := range child {
			if name == definition.Name {
				found = true
				break
			}
		}
		if !found {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("parameter %q is unknown", unknown[0])
	}
	return nil
}

// Validate 校验已解析执行流程的版本、依赖快照、参数绑定和引用图。
func (p ResolvedExecutionFlow) Validate() error {
	if err := p.Task.Validate(); err != nil {
		return err
	}
	if err := p.Version.Validate(); err != nil {
		return err
	}
	if p.Version.ExecutionFlowID != p.Task.ID {
		return executionFlowDependencyInvalidError([]fault.Violation{
			mustViolation(fault.CodeFieldMismatch, "version.executionFlowId", "resolved version does not belong to the execution flow"),
		})
	}
	if p.Version.VersionNumber == 1 {
		if p.ExpectedExecutionFlowRevision != 0 || p.Version.SourceVersionID != "" {
			return executionFlowDependencyInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldMismatch, "expectedExecutionFlowRevision", "a first version must not expect a prior revision or declare a source version"),
			})
		}
	} else if err := p.ExpectedExecutionFlowRevision.ValidatePersisted(); err != nil || strings.TrimSpace(p.Version.SourceVersionID) == "" {
		return executionFlowDependencyInvalidError([]fault.Violation{
			mustViolation(fault.CodeFieldRequired, "version.sourceVersionId", "a subsequent version requires a source version and a persisted expected revision"),
		})
	}
	workflows := map[string]FlowFragmentDependencySnapshot{}
	for _, dependency := range p.Workflows {
		if dependency.FlowFragment.ID == "" || dependency.Version.ID == "" ||
			dependency.Version.FlowFragmentID != dependency.FlowFragment.ID || dependency.Version.VersionNumber < 1 {
			return executionFlowDependencyInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldInvalid, "workflows", "workflow dependency snapshot identity is invalid"),
			})
		}
		key := dependency.FlowFragment.ID + "\x00" + dependency.Version.ID
		if _, exists := workflows[key]; exists {
			return executionFlowDependencyInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldDuplicate, "workflows", "workflow dependency snapshot is duplicated"),
			})
		}
		workflows[key] = dependency
	}
	nodes := map[string]bool{}
	for _, dependency := range p.Nodes {
		if dependency.ElementTarget.ID == "" || dependency.Version.ID == "" ||
			dependency.Version.ElementTargetID != dependency.ElementTarget.ID || dependency.Version.VersionNumber < 1 {
			return executionFlowDependencyInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldInvalid, "nodes", "element target dependency snapshot identity is invalid"),
			})
		}
		key := ElementTargetDependencyIdentity(dependency.ElementTarget.ID, dependency.Version.ID)
		if nodes[key] {
			return executionFlowDependencyInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldDuplicate, "nodes", "element target dependency snapshot is duplicated"),
			})
		}
		nodes[key] = true
	}
	for _, item := range p.Version.Items {
		matched := false
		for _, dependency := range p.Workflows {
			if dependency.FlowFragment.ID != item.FlowFragmentID {
				continue
			}
			switch item.VersionPolicy {
			case FlowFragmentVersionFixed:
				matched = dependency.Version.ID == item.WorkflowVersionID
			case FlowFragmentVersionLatest:
				matched = dependency.ResolvedFromLatest
			}
			if matched {
				break
			}
		}
		if !matched {
			return executionFlowDependencyInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldInvalid, "version.items.flowFragmentId", "an item has no matching workflow dependency"),
			})
		}
		for _, dependency := range p.Workflows {
			if dependency.FlowFragment.ID == item.FlowFragmentID && (item.VersionPolicy == FlowFragmentVersionLatest && dependency.ResolvedFromLatest || item.VersionPolicy == FlowFragmentVersionFixed && dependency.Version.ID == item.WorkflowVersionID) {
				if _, err := ResolveParameterValues(dependency.Version.Definition.Parameters, item.Parameters); err != nil {
					// PARAMETER_* 错误比当前信封更精确，因此保持原码直接返回，不改写。
					if _, classified := fault.CodeOf(err); classified {
						return err
					}
					return executionFlowDependencyInvalidError([]fault.Violation{
						mustViolation(fault.CodeFieldInvalid, "version.items.parameters", "item parameters are incompatible with the dependency"),
					})
				}
				break
			}
		}
	}
	return p.validateDependencyGraph(nodes)
}

// validateDependencyGraph 校验工作流引用图及节点、工作流和解析结果的完整使用关系。
func (p ResolvedExecutionFlow) validateDependencyGraph(nodes map[string]bool) error {
	byVersion := map[string]FlowFragmentDependencySnapshot{}
	for _, dependency := range p.Workflows {
		if _, duplicate := byVersion[dependency.Version.ID]; duplicate {
			return executionFlowDependencyInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldDuplicate, "workflows", "workflow version dependency is duplicated"),
			})
		}
		byVersion[dependency.Version.ID] = dependency
	}
	references := map[string]FlowFragmentReferenceResolution{}
	for _, resolution := range p.References {
		key := resolution.ParentFlowFragmentVersionID + "\x00" + resolution.StepID
		if resolution.ParentFlowFragmentVersionID == "" || resolution.StepID == "" ||
			resolution.FlowFragmentID == "" || resolution.WorkflowVersionID == "" {
			return executionFlowDependencyInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldInvalid, "references", "workflow reference resolution identity is incomplete"),
			})
		}
		if _, duplicate := references[key]; duplicate {
			return executionFlowDependencyInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldDuplicate, "references", "workflow reference resolution is duplicated"),
			})
		}
		references[key] = resolution
	}
	usedWorkflows := map[string]bool{}
	usedReferences := map[string]bool{}
	usedNodes := map[string]bool{}
	visiting := map[string]bool{}
	var visit func(string) error
	visit = func(versionID string) error {
		dependency, exists := byVersion[versionID]
		if !exists {
			return executionFlowDependencyInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldInvalid, "dependencyGraph.workflowVersion", "a referenced workflow version is outside the resolved set"),
			})
		}
		if visiting[versionID] {
			return executionFlowDependencyInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldInvalid, "dependencyGraph.workflowVersion", "workflow dependencies contain a cycle"),
			})
		}
		if usedWorkflows[versionID] {
			return nil
		}
		visiting[versionID] = true
		defer delete(visiting, versionID)
		var walk func([]FlowFragmentStep) error
		walk = func(steps []FlowFragmentStep) error {
			for _, step := range steps {
				if step.ElementTargetID != "" {
					key := ElementTargetDependencyIdentity(step.ElementTargetID, step.ElementTargetVersionID)
					if !nodes[key] {
						return executionFlowDependencyInvalidError([]fault.Violation{
							mustViolation(fault.CodeFieldInvalid, "dependencyGraph.step.elementTarget", "a step references an element target version outside the resolved set"),
						})
					}
					usedNodes[key] = true
				}
				if step.Reference != nil {
					key := versionID + "\x00" + step.ID
					resolution, ok := references[key]
					if !ok {
						return executionFlowDependencyInvalidError([]fault.Violation{
							mustViolation(fault.CodeFieldInvalid, "dependencyGraph.step.reference", "a step has no matching workflow reference resolution"),
						})
					}
					target, ok := byVersion[resolution.WorkflowVersionID]
					if !ok || target.FlowFragment.ID != resolution.FlowFragmentID || resolution.FlowFragmentID != step.Reference.FlowFragmentID {
						return executionFlowDependencyInvalidError([]fault.Violation{
							mustViolation(fault.CodeFieldMismatch, "dependencyGraph.step.reference", "a step workflow reference target is inconsistent with its resolution"),
						})
					}
					if step.Reference.LatestPublished {
						if !resolution.ResolvedFromLatest {
							return executionFlowDependencyInvalidError([]fault.Violation{
								mustViolation(fault.CodeFieldMismatch, "dependencyGraph.step.reference.workflowVersionId", "a latest workflow reference was not resolved from the current version"),
							})
						}
					} else if resolution.ResolvedFromLatest || step.Reference.WorkflowVersionID != target.Version.ID {
						return executionFlowDependencyInvalidError([]fault.Violation{
							mustViolation(fault.CodeFieldMismatch, "dependencyGraph.step.reference.workflowVersionId", "a fixed workflow reference no longer matches the resolved version"),
						})
					}
					if err := validateReferenceBindings(dependency.Version.Definition.Parameters, target.Version.Definition.Parameters, step.Reference.ParameterBindings); err != nil {
						if _, classified := fault.CodeOf(err); classified {
							return err
						}
						return executionFlowDependencyInvalidError([]fault.Violation{
							mustViolation(fault.CodeFieldInvalid, "dependencyGraph.step.reference.parameterBindings", "workflow reference parameter bindings are incompatible"),
						})
					}
					usedReferences[key] = true
					if err := visit(target.Version.ID); err != nil {
						return err
					}
				}
				if err := walk(step.Children); err != nil {
					return err
				}
				if step.ValidationGroup != nil {
					for _, branch := range step.ValidationGroup.Branches {
						if err := walk(branch.Steps); err != nil {
							return err
						}
					}
				}
			}
			return nil
		}
		if err := walk(dependency.Version.Definition.Steps); err != nil {
			return err
		}
		usedWorkflows[versionID] = true
		return nil
	}
	for _, item := range p.Version.Items {
		for _, dependency := range p.Workflows {
			if dependency.FlowFragment.ID != item.FlowFragmentID {
				continue
			}
			if item.VersionPolicy == FlowFragmentVersionFixed && dependency.Version.ID != item.WorkflowVersionID {
				continue
			}
			if item.VersionPolicy == FlowFragmentVersionLatest && !dependency.ResolvedFromLatest {
				continue
			}
			if err := visit(dependency.Version.ID); err != nil {
				return err
			}
			break
		}
	}
	if len(usedWorkflows) != len(byVersion) || len(usedReferences) != len(references) || len(usedNodes) != len(nodes) {
		return executionFlowDependencyInvalidError([]fault.Violation{
			mustViolation(fault.CodeFieldInvalid, "dependencyGraph", "the dependency graph contains unused snapshots or resolutions"),
		})
	}
	return nil
}

// EnvironmentKeys 从插值表达式中提取 env. 前缀变量名并按字典序返回。
func EnvironmentKeys(values ...string) ([]string, error) {
	set := map[string]bool{}
	for _, value := range values {
		names, err := interpolation.Names(value)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			if strings.HasPrefix(name, "env.") && len(name) > len("env.") {
				set[strings.TrimPrefix(name, "env.")] = true
			}
		}
	}
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result, nil
}

// ElementTargetDependencyIdentity 以稳定分隔符拼接节点和版本身份作为依赖键。
func ElementTargetDependencyIdentity(nodeID, versionID string) string {
	return nodeID + "\x00" + versionID
}
