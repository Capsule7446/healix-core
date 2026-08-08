package node

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/interpolation"
)

// ValidationAssertion 表示框架无关的验证断言及其比较参数。
type ValidationAssertion struct {
	Kind           string
	Expected       string
	ExpectedValues []string
	Attribute      string
	IgnoreCase     bool
}

// ValidationObservation 记录一次验证轮询或终态结果，供基础设施映射到执行事实时间线。
type ValidationObservation struct {
	NodeID            string
	GroupID           string
	BranchID          string
	Assertion         ValidationAssertion
	Actual            string
	ActualValues      []string
	Passed            bool
	Reason            string
	BranchDisposition string
	Selector          fingerprint.Selector
	ObservedAtMS      int64
	Final             bool
}

// ValidationMemberIdentity 标识验证组中预期存在的分支成员。
type ValidationMemberIdentity struct {
	BranchID string
	NodeID   string
}

// ValidationGroupTerminalObservation 记录验证组终态、获胜分支和预期成员身份。
type ValidationGroupTerminalObservation struct {
	GroupID         string
	TerminalReason  string
	WinningBranchID string
	ExpectedMembers []ValidationMemberIdentity
	ObservedAtMS    int64
}

// ValidationStateReader 提供元素的值、状态和选项集合读取能力。
type ValidationStateReader interface {
	// ValidationState 返回元素当前的框架无关状态快照。
	ValidationState(context.Context) (ValidationState, error)
}

// ValidationState 保存元素的值以及常见交互状态的读取结果。
type ValidationState struct {
	Value          string
	Enabled        bool
	Checked        bool
	Mixed          bool
	Selected       bool
	Pressed        bool
	SelectedTexts  []string
	SelectedValues []string
}

// validationPollInterval 定义验证轮询器使用的默认轮询间隔。
const validationPollInterval = 200 * time.Millisecond

// ValidationNode 表示对单个元素执行断言并等待其稳定满足的节点。
type ValidationNode struct {
	NodeID string
	// 对于独立验证，GroupID 和 BranchID 为空。它们是执行身份，而不是持久的表达式模型，并让证据适配器将成员观察结果附加到其组 StepExecution。
	GroupID   string
	BranchID  string
	Target    fingerprint.ElementTargetSpec
	Assertion ValidationAssertion
	MaxWait   time.Duration
	Stability time.Duration
}

// ID 返回验证节点的执行标识。
func (v *ValidationNode) ID() string { return v.NodeID }

// Run 执行一次验证节点生命周期，并在成功前等待断言持续稳定。
func (v *ValidationNode) Run(ctx context.Context, rt *Runtime) (runErr error) {
	if err := rt.waitBeforeStep(ctx); err != nil {
		return classifyNodeFault(err)
	}
	execution := NewStepExecution(v.NodeID)
	if err := transitionValidation(ctx, rt, execution, v.NodeID, PhaseRunning); err != nil {
		return err
	}
	defer rt.releaseOccurrence(execution.nodeID, execution.occurrence)
	lifecycle, err := rt.beginLeafLifecycle(ctx, v.NodeID, "VALIDATION", execution.occurrence)
	if err != nil {
		return validationFail(ctx, rt, execution, v.NodeID, err)
	}
	defer func() { runErr = lifecycle.Complete(ctx, runErr) }()
	if err := transitionValidation(ctx, rt, execution, v.NodeID, PhaseValidating); err != nil {
		return validationFail(ctx, rt, execution, v.NodeID, err)
	}
	started := time.Now()
	validationErr := v.waitStable(ctx, rt)
	rt.observeOperationBestEffort(ctx, OperationObservation{InstanceID: rt.InstanceID, EntryID: rt.EntryID, Occurrence: rt.mustActiveOccurrence(v.NodeID), NodeID: v.NodeID, Operation: "validation", Attempt: 1, DurationMS: time.Since(started).Milliseconds(), Succeeded: validationErr == nil, FaultKind: nodeFaultKind(validationErr), FaultCode: nodeFaultCode(validationErr)})
	if validationErr != nil {
		return validationFail(ctx, rt, execution, v.NodeID, errors.Join(validationErr))
	}
	if err := transitionValidation(ctx, rt, execution, v.NodeID, PhaseSucceeded); err != nil {
		return validationFail(ctx, rt, execution, v.NodeID, err)
	}
	return nil
}

// waitStable 轮询断言直到其满足稳定窗口或达到最大等待时间，并记录终态证据。
func (v *ValidationNode) waitStable(parent context.Context, rt *Runtime) error {
	maxWait := v.MaxWait
	if maxWait <= 0 {
		maxWait = 10 * time.Second
	}
	stability := v.Stability
	if stability <= 0 {
		stability = 500 * time.Millisecond
	}
	var stableSince time.Time
	var lastActual string
	var lastActualValues []string
	observations := newValidationObservationRecorder()
	pollErr := rt.poller().Run(parent, maxWait, func(pollCtx context.Context) (bool, error) {
		var actualValues []string
		ok, actual, err := v.evaluateCollect(pollCtx, rt, &actualValues)
		lastActual = actual
		lastActualValues = append(lastActualValues[:0], actualValues...)
		if err != nil {
			if recordErr := observations.record(pollCtx, rt, v, false, actual, actualValues, "system_error", false); recordErr != nil {
				return false, recordErr
			}
			return false, err
		}
		if err := observations.record(pollCtx, rt, v, ok, actual, actualValues, validationReason(ok), false); err != nil {
			return false, err
		}
		if ok {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= stability {
				return true, observations.record(pollCtx, rt, v, true, actual, actualValues, "passed", true)
			}
		} else {
			stableSince = time.Time{}
		}
		return false, nil
	})
	if pollErr != nil {
		reason := "timeout"
		if !fault.IsCode(pollErr, CodeTimeout) {
			reason = "system_error"
		}
		if err := observations.record(context.WithoutCancel(parent), rt, v, false, lastActual, lastActualValues, reason, true); err != nil {
			return err
		}
		return fmt.Errorf("assertion was not continuously satisfied within %s: %w", maxWait, pollErr)
	}
	return nil
}

// evaluate 执行一轮验证读取并返回是否满足、实际值和错误。
func (v *ValidationNode) evaluate(ctx context.Context, rt *Runtime) (bool, string, error) {
	return v.evaluateCollect(ctx, rt, nil)
}

// evaluateCollect 执行一轮验证读取，并在集合断言中复制实际值到 actualValues。
func (v *ValidationNode) evaluateCollect(ctx context.Context, rt *Runtime, actualValues *[]string) (bool, string, error) {
	assertion, err := v.resolvedAssertion(rt)
	if err != nil {
		return false, "", err
	}
	el, absent, err := v.locate(ctx, rt)
	if err != nil {
		return false, "", fmt.Errorf("locate: %w", err)
	}
	if absent {
		if assertion.Kind == "not_exists" {
			return true, "<absent>", nil
		}
		// 删除故意不“不可见”；调用者必须为该语义断言 not_exists。
		return false, "<absent>", nil
	}
	exists, err := el.Exists(ctx)
	if err != nil {
		if fault.IsCode(err, CodeElementNotFound) {
			return assertion.Kind == "not_exists", "<absent>", nil
		}
		return false, "", err
	}
	if !exists {
		return assertion.Kind == "not_exists", "<absent>", nil
	}
	if assertion.Kind == "exists" {
		return true, "<present>", nil
	}
	if assertion.Kind == "not_exists" {
		return false, "<present>", nil
	}
	visible, err := rt.reader().Visible(ctx, el)
	if err != nil {
		return false, "", err
	}
	switch assertion.Kind {
	case "visible":
		return visible, fmt.Sprint(visible), nil
	case "not_visible":
		return !visible, fmt.Sprint(visible), nil
	}

	if assertion.Kind == "text_equals" || assertion.Kind == "text_contains" || assertion.Kind == "text_matches" {
		text, err := rt.reader().Text(ctx, el)
		if err != nil {
			return false, "", err
		}
		return compareText(assertion, text)
	}
	if assertion.Kind == "attribute_equals" || assertion.Kind == "attribute_contains" {
		value, present, err := rt.reader().Attribute(ctx, el, assertion.Attribute)
		if err != nil {
			return false, "", err
		}
		if !present {
			return false, "<undefined>", nil
		}
		return compareScalar(assertion, value, false)
	}

	reader, ok := el.(ValidationStateReader)
	if !ok {
		return false, "", unsupportedAssertionKindError(fmt.Errorf("driver element does not provide validation state for %q", assertion.Kind))
	}
	state, err := reader.ValidationState(ctx)
	if err != nil {
		return false, "", err
	}
	switch assertion.Kind {
	case "value_equals", "value_contains", "value_matches", "value_not_empty":
		if assertion.Kind == "value_not_empty" {
			return state.Value != "", state.Value, nil
		}
		return compareScalar(assertion, state.Value, false)
	case "enabled":
		return state.Enabled, fmt.Sprint(state.Enabled), nil
	case "disabled":
		return !state.Enabled, fmt.Sprint(state.Enabled), nil
	case "checked":
		return state.Checked && !state.Mixed, fmt.Sprint(state.Checked), nil
	case "unchecked":
		return !state.Checked && !state.Mixed, fmt.Sprint(state.Checked), nil
	case "mixed":
		return state.Mixed, fmt.Sprint(state.Mixed), nil
	case "selected":
		return state.Selected, fmt.Sprint(state.Selected), nil
	case "unselected":
		return !state.Selected, fmt.Sprint(state.Selected), nil
	case "pressed":
		return state.Pressed, fmt.Sprint(state.Pressed), nil
	case "unpressed":
		return !state.Pressed, fmt.Sprint(state.Pressed), nil
	case "selected_text_equals", "selected_text_contains":
		value := firstValue(state.SelectedTexts)
		return compareScalar(assertion, value, false)
	case "selected_value_equals", "selected_value_contains":
		value := firstValue(state.SelectedValues)
		return compareScalar(assertion, value, false)
	case "selected_set_equals", "selected_set_contains":
		if actualValues != nil {
			*actualValues = append([]string(nil), state.SelectedTexts...)
		}
		return compareSet(assertion, state.SelectedTexts)
	default:
		return false, "", unsupportedAssertionKindError(fmt.Errorf("unsupported validation assertion %q", assertion.Kind))
	}
}

// unsupportedAssertionKindError 将不支持的断言类型归类为断言配置错误。
func unsupportedAssertionKindError(cause error) error {
	return wrapStepConfigurationInvalidError(cause, mustViolation(fault.CodeFieldInvalid, "assertion.kind", "assertion kind is not supported"))
}

// resolvedAssertion 展开断言中的运行时变量，并深拷贝期望值集合。
func (v *ValidationNode) resolvedAssertion(rt *Runtime) (ValidationAssertion, error) {
	resolved := v.Assertion
	resolved.ExpectedValues = append([]string(nil), v.Assertion.ExpectedValues...)
	variables := runtimeVariables{rt: rt}
	expected, err := interpolation.Expand(resolved.Expected, variables)
	if err != nil {
		return ValidationAssertion{}, fmt.Errorf("validation %s expected: %w", v.NodeID, err)
	}
	resolved.Expected = expected
	for index, value := range resolved.ExpectedValues {
		expanded, err := interpolation.Expand(value, variables)
		if err != nil {
			return ValidationAssertion{}, fmt.Errorf("validation %s expected_values[%d]: %w", v.NodeID, index, err)
		}
		resolved.ExpectedValues[index] = expanded
	}
	return resolved, nil
}

// locate 按当前选择器定位元素，并在允许时执行修复、记录决定和安装选择器覆盖层。
func (v *ValidationNode) locate(ctx context.Context, rt *Runtime) (Element, bool, error) {
	target := rt.effectiveSpec(v.Target)
	el, err := rt.locator().Locate(ctx, target)
	if err == nil {
		return el, false, nil
	}
	if !isExclusiveElementNotFound(err) {
		return nil, false, err
	}
	if rt.Healing == nil && rt.Healer == nil {
		return nil, true, nil
	}
	snapshot, err := rt.Driver.Snapshot(ctx)
	if err != nil {
		return nil, false, classifyNodeFault(err)
	}
	decision, err := rt.healingPort().Recover(ctx, target, snapshot)
	if err != nil {
		return nil, false, err
	}
	if err := decision.Validate(); err != nil {
		return nil, false, classifyNodeFault(err)
	}
	if err := rt.recordHealSamples(ctx, HealSampleRecord{InstanceID: rt.InstanceID, NodeID: v.NodeID, SpecID: target.ID, OldSelector: firstSelector(target), Outcome: decision.Outcome, Samples: heal.SortSamples(decision.Samples(target.Fingerprint, rt.healingReviewCap()))}); err != nil {
		return nil, false, evidenceRecordFailedError(err)
	}
	if decision.Outcome == heal.OutcomeNoCandidate {
		if rt.Facts != nil {
			if err := rt.Facts.StageHealDecision(ctx, domainexecution.WorkerFence{InstanceID: rt.InstanceID, ClaimToken: rt.ClaimToken}, v.NodeID, target.ID, firstSelector(target), decision); err != nil {
				return nil, false, evidenceRecordFailedError(err)
			}
		}
		return nil, true, nil
	}
	assessment, err := heal.Assess(target, decision, rt.currentLocation(ctx), rt.HealingPolicy)
	if err != nil {
		return nil, false, classifyNodeFault(err)
	}
	if assessment.Disposition != heal.DispositionAllow {
		if assessment.Disposition == heal.DispositionBlock && decision.Outcome != heal.OutcomeNoCandidate {
			decision.Outcome = heal.OutcomeSafetyRejected
			decision.NeedsReview = false
		}
		if rt.Facts != nil {
			oldSelector := firstSelector(target)
			if recordErr := rt.Facts.StageHealDecision(ctx, domainexecution.WorkerFence{InstanceID: rt.InstanceID, ClaimToken: rt.ClaimToken}, v.NodeID, target.ID, oldSelector, decision); recordErr != nil {
				return nil, false, evidenceRecordFailedError(recordErr)
			}
		}
		return nil, false, healingRefusedError(fmt.Errorf("validation healing refused: %s", assessment.Explanation))
	}
	if decision.Outcome == heal.OutcomeNoCandidate || decision.Best == nil {
		if rt.Facts != nil {
			if recordErr := rt.Facts.StageHealDecision(ctx, domainexecution.WorkerFence{InstanceID: rt.InstanceID, ClaimToken: rt.ClaimToken}, v.NodeID, target.ID, firstSelector(target), decision); recordErr != nil {
				return nil, false, evidenceRecordFailedError(recordErr)
			}
		}
		return nil, true, nil
	}
	healed := promoteSelector(target, decision.Best.Selector)
	el, err = rt.Driver.Locate(ctx, healed)
	if err != nil {
		return nil, false, fmt.Errorf("re-locate after heal: %w", err)
	}
	if rt.Facts != nil {
		if recordErr := rt.Facts.StageHealDecision(ctx, domainexecution.WorkerFence{InstanceID: rt.InstanceID, ClaimToken: rt.ClaimToken}, v.NodeID, target.ID, firstSelector(target), decision); recordErr != nil {
			return nil, false, evidenceRecordFailedError(recordErr)
		}
	}
	rt.setSelectorOverlay(healed)
	return el, false, nil
}

// compareText 按文本断言类型比较实际文本，并在需要时规范化空白。
func compareText(assertion ValidationAssertion, actual string) (bool, string, error) {
	if assertion.Kind == "text_matches" {
		re, err := regexp.Compile(assertion.Expected)
		if err != nil {
			return false, actual, err
		}
		return re.MatchString(actual), actual, nil
	}
	return compareScalar(assertion, actual, true)
}

// compareScalar 比较单个字符串值，可选择规范化空白并忽略大小写。
func compareScalar(assertion ValidationAssertion, actual string, normalizeWhitespace bool) (bool, string, error) {
	expected := assertion.Expected
	if normalizeWhitespace {
		actual, expected = normalizeValidationText(actual), normalizeValidationText(expected)
	}
	if assertion.IgnoreCase {
		actual, expected = strings.ToLower(actual), strings.ToLower(expected)
	}
	switch assertion.Kind {
	case "text_equals", "value_equals", "selected_text_equals", "selected_value_equals", "attribute_equals":
		return actual == expected, actual, nil
	case "text_contains", "value_contains", "selected_text_contains", "selected_value_contains", "attribute_contains":
		return strings.Contains(actual, expected), actual, nil
	case "value_matches":
		re, err := regexp.Compile(expected)
		if err != nil {
			return false, actual, err
		}
		return re.MatchString(actual), actual, nil
	default:
		return false, actual, unsupportedAssertionKindError(fmt.Errorf("unsupported scalar validation %q", assertion.Kind))
	}
}

// compareSet 比较期望集合与实际集合，并复制输入后排序以避免修改调用方数据。
func compareSet(assertion ValidationAssertion, actual []string) (bool, string, error) {
	expected := append([]string(nil), assertion.ExpectedValues...)
	actual = append([]string(nil), actual...)
	sort.Strings(expected)
	sort.Strings(actual)
	if assertion.Kind == "selected_set_equals" {
		if len(expected) != len(actual) {
			return false, strings.Join(actual, ", "), nil
		}
		for index := range expected {
			if expected[index] != actual[index] {
				return false, strings.Join(actual, ", "), nil
			}
		}
		return true, strings.Join(actual, ", "), nil
	}
	available := make(map[string]int, len(actual))
	for _, value := range actual {
		available[value]++
	}
	for _, value := range expected {
		if available[value] == 0 {
			return false, strings.Join(actual, ", "), nil
		}
		available[value]--
	}
	return true, strings.Join(actual, ", "), nil
}

// normalizeValidationText 将连续空白折叠为单个 ASCII 空格。
func normalizeValidationText(value string) string { return strings.Join(strings.Fields(value), " ") }

// firstValue 返回切片首项；空切片返回空字符串。
func firstValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// ValidationBranch 描述验证组中的一个分支及其按同一轮求与的成员节点。
type ValidationBranch struct {
	ID    string
	Nodes []*ValidationNode
}

// ValidationGroupNode 表示包含多个候选验证分支的验证组节点。
type ValidationGroupNode struct {
	NodeID    string
	Branches  []ValidationBranch
	MaxWait   time.Duration
	Stability time.Duration
}

// ID 返回验证组节点的执行标识。
func (g *ValidationGroupNode) ID() string { return g.NodeID }

// Run 执行验证组生命周期，并在任一分支稳定满足后完成。
func (g *ValidationGroupNode) Run(ctx context.Context, rt *Runtime) (runErr error) {
	for branchIndex, branch := range g.Branches {
		if branch.ID == "" {
			return wrapStepConfigurationInvalidError(
				fmt.Errorf("validation group %s: branch %d requires an ID", g.NodeID, branchIndex),
				mustViolation(fault.CodeFieldRequired, fmt.Sprintf("branches.%d.id", branchIndex), "validation branch id is required"),
			)
		}
		for memberIndex, member := range branch.Nodes {
			if member == nil {
				return fmt.Errorf("validation group %s: branch %s member %d is nil", g.NodeID, branch.ID, memberIndex)
			}
		}
	}
	if err := rt.waitBeforeStep(ctx); err != nil {
		return fmt.Errorf("validation group %s: wait step interval: %w", g.NodeID, err)
	}
	execution := NewStepExecution(g.NodeID)
	if err := transitionValidation(ctx, rt, execution, g.NodeID, PhaseRunning); err != nil {
		return err
	}
	defer rt.releaseOccurrence(execution.nodeID, execution.occurrence)
	lifecycle, err := rt.beginLeafLifecycle(ctx, g.NodeID, "VALIDATION_GROUP", execution.occurrence)
	if err != nil {
		return validationFail(ctx, rt, execution, g.NodeID, err)
	}
	defer func() { runErr = lifecycle.Complete(ctx, runErr) }()
	if err := transitionValidation(ctx, rt, execution, g.NodeID, PhaseValidating); err != nil {
		return validationFail(ctx, rt, execution, g.NodeID, err)
	}
	if err := g.waitStable(ctx, rt); err != nil {
		return validationFail(ctx, rt, execution, g.NodeID, err)
	}
	if err := transitionValidation(ctx, rt, execution, g.NodeID, PhaseSucceeded); err != nil {
		return validationFail(ctx, rt, execution, g.NodeID, err)
	}
	return nil
}

// waitStable 并行轮询所有分支，等待至少一个分支在稳定窗口内满足。
func (g *ValidationGroupNode) waitStable(parent context.Context, rt *Runtime) error {
	maxWait, stability := g.MaxWait, g.Stability
	if maxWait <= 0 {
		maxWait = 10 * time.Second
	}
	if stability <= 0 {
		stability = 500 * time.Millisecond
	}
	observations := newValidationObservationRecorder()
	stableSince := make([]time.Time, len(g.Branches))
	last := make(map[validationObservationKey]validationObservationState)
	pollErr := rt.poller().Run(parent, maxWait, func(ctx context.Context) (bool, error) {
		branchPassed := make([]bool, len(g.Branches))
		for i, branch := range g.Branches {
			branchPassed[i] = len(branch.Nodes) > 0
			for _, member := range branch.Nodes {
				canonical := *member
				canonical.GroupID = g.NodeID
				canonical.BranchID = branch.ID
				var actualValues []string
				ok, actual, err := canonical.evaluateCollect(ctx, rt, &actualValues)
				if err != nil {
					if actual != "" || actualValues != nil {
						key := validationObservationKey{groupID: g.NodeID, branchID: branch.ID, nodeID: canonical.NodeID}
						last[key] = validationObservationState{actual: actual, actualValues: append([]string(nil), actualValues...), reason: "system_error"}
					}
					if recordErr := observations.record(ctx, rt, &canonical, false, actual, actualValues, "system_error", false); recordErr != nil {
						return false, recordErr
					}
					return false, fmt.Errorf("branch %s: %w", branch.ID, err)
				}
				key := validationObservationKey{groupID: g.NodeID, branchID: branch.ID, nodeID: canonical.NodeID}
				last[key] = validationObservationState{passed: ok, actual: actual, actualValues: actualValues, reason: validationReason(ok)}
				if err := observations.record(ctx, rt, &canonical, ok, actual, actualValues, validationReason(ok), false); err != nil {
					return false, err
				}
				if !ok {
					branchPassed[i] = false
				}
			}
		}
		now := time.Now()
		for i, passed := range branchPassed {
			if !passed {
				stableSince[i] = time.Time{}
				continue
			}
			if stableSince[i].IsZero() {
				stableSince[i] = now
			}
			if now.Sub(stableSince[i]) >= stability {
				winner := g.Branches[i].ID
				if err := observations.recordGroupFinal(ctx, rt, g, last, "passed", winner); err != nil {
					return false, err
				}
				return true, nil
			}
		}
		return false, nil
	})
	if pollErr != nil {
		ctx := context.WithoutCancel(parent)
		reason := validationTerminalReason(pollErr)
		if err := observations.recordGroupFinal(ctx, rt, g, last, reason, ""); err != nil {
			return err
		}
		return fmt.Errorf("no validation branch was continuously satisfied within %s: %w", maxWait, pollErr)
	}
	return nil
}

// validationObservationState 保存单个验证成员最近一次观测的结果。
type validationObservationState struct {
	passed       bool
	actual       string
	actualValues []string
	reason       string
}

// validationObservationStatesEqual 判断两次观测的判定、值和原因是否完全一致。
func validationObservationStatesEqual(left, right validationObservationState) bool {
	return left.passed == right.passed && left.actual == right.actual && left.reason == right.reason && slices.Equal(left.actualValues, right.actualValues)
}

// validationObservationKey 标识验证组、分支和成员节点的组合。
type validationObservationKey struct {
	groupID  string
	branchID string
	nodeID   string
}

// validationObservationRecorder 保存最近观测并负责写入验证事实。
type validationObservationRecorder struct {
	last map[validationObservationKey]validationObservationState
}

// newValidationObservationRecorder 创建按节点身份去重的验证观测记录器。
func newValidationObservationRecorder() *validationObservationRecorder {
	return &validationObservationRecorder{last: make(map[validationObservationKey]validationObservationState)}
}

// record 记录一次验证观测；非终态且内容未变化的观测会被去重。
func (r *validationObservationRecorder) record(ctx context.Context, rt *Runtime, validation *ValidationNode,
	passed bool, actual string, actualValues []string, reason string, final bool) error {
	if rt.Facts == nil {
		return nil
	}
	state := validationObservationState{passed: passed, actual: actual, actualValues: append([]string(nil), actualValues...), reason: reason}
	key := validationObservationKey{groupID: validation.GroupID, branchID: validation.BranchID, nodeID: validation.NodeID}
	previous, seen := r.last[key]
	if !final && seen && validationObservationStatesEqual(previous, state) {
		return nil
	}
	r.last[key] = state
	spec := rt.effectiveSpec(validation.Target)
	selector := fingerprint.Selector{}
	if len(spec.Selectors) > 0 {
		selector = spec.Selectors[0]
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalEventTimeout)
	defer cancel()
	assertion := validation.Assertion
	assertion.ExpectedValues = append([]string(nil), validation.Assertion.ExpectedValues...)
	if validationEvidenceIsSensitive(validation.Target, assertion) {
		actual = "••••••••"
		actualValues = nil
		if assertion.Expected != "" {
			assertion.Expected = "••••••••"
		}
		assertion.ExpectedValues = nil
	}
	return rt.Facts.StageValidationObservation(cleanupCtx, domainexecution.WorkerFence{InstanceID: rt.InstanceID, ClaimToken: rt.ClaimToken}, ValidationObservation{
		NodeID: validation.NodeID, GroupID: validation.GroupID, BranchID: validation.BranchID,
		Assertion: assertion, Actual: actual, ActualValues: append([]string(nil), actualValues...), Passed: passed, Reason: reason,
		Selector: selector, ObservedAtMS: time.Now().UnixMilli(), Final: final,
	})
}

// recordGroupFinal 为验证组的每个成员记录终态处置，并写入组终态事实。
func (r *validationObservationRecorder) recordGroupFinal(ctx context.Context, rt *Runtime, group *ValidationGroupNode, last map[validationObservationKey]validationObservationState, terminalReason, winningBranchID string) error {
	members := make([]ValidationMemberIdentity, 0)
	seen := make(map[ValidationMemberIdentity]struct{})
	for _, branch := range group.Branches {
		disposition := branchDisposition(group.NodeID, branch, last, terminalReason, winningBranchID)
		for _, member := range branch.Nodes {
			identity := ValidationMemberIdentity{BranchID: branch.ID, NodeID: member.NodeID}
			if _, exists := seen[identity]; exists {
				continue
			}
			seen[identity] = struct{}{}
			members = append(members, identity)
			canonical := *member
			canonical.GroupID = group.NodeID
			canonical.BranchID = branch.ID
			key := validationObservationKey{groupID: group.NodeID, branchID: branch.ID, nodeID: member.NodeID}
			state, observed := last[key]
			if !observed {
				state.reason = terminalReason
			}
			reason := state.reason
			if terminalReason != "passed" {
				reason = terminalReason
			} else if state.passed {
				reason = "passed"
			}
			if err := r.recordWithDisposition(ctx, rt, &canonical, state.passed, state.actual, state.actualValues, reason, disposition); err != nil {
				return err
			}
		}
	}
	if rt.Facts == nil {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalEventTimeout)
	defer cancel()
	return rt.Facts.StageValidationGroupTerminal(cleanupCtx, domainexecution.WorkerFence{InstanceID: rt.InstanceID, ClaimToken: rt.ClaimToken}, ValidationGroupTerminalObservation{
		GroupID: group.NodeID, TerminalReason: terminalReason, WinningBranchID: winningBranchID,
		ExpectedMembers: members, ObservedAtMS: time.Now().UnixMilli(),
	})
}

// recordWithDisposition 记录带分支处置结果的终态验证观测。
func (r *validationObservationRecorder) recordWithDisposition(ctx context.Context, rt *Runtime, validation *ValidationNode, passed bool, actual string, actualValues []string, reason, disposition string) error {
	if rt.Facts == nil {
		return nil
	}
	state := validationObservationState{passed: passed, actual: actual, actualValues: append([]string(nil), actualValues...), reason: reason}
	key := validationObservationKey{groupID: validation.GroupID, branchID: validation.BranchID, nodeID: validation.NodeID}
	r.last[key] = state
	spec := rt.effectiveSpec(validation.Target)
	selector := fingerprint.Selector{}
	if len(spec.Selectors) > 0 {
		selector = spec.Selectors[0]
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalEventTimeout)
	defer cancel()
	assertion := validation.Assertion
	assertion.ExpectedValues = append([]string(nil), validation.Assertion.ExpectedValues...)
	if validationEvidenceIsSensitive(validation.Target, assertion) {
		actual = "••••••••"
		actualValues = nil
		if assertion.Expected != "" {
			assertion.Expected = "••••••••"
		}
		assertion.ExpectedValues = nil
	}
	return rt.Facts.StageValidationObservation(cleanupCtx, domainexecution.WorkerFence{InstanceID: rt.InstanceID, ClaimToken: rt.ClaimToken}, ValidationObservation{
		NodeID: validation.NodeID, GroupID: validation.GroupID, BranchID: validation.BranchID,
		Assertion: assertion, Actual: actual, ActualValues: append([]string(nil), actualValues...), Passed: passed, Reason: reason, BranchDisposition: disposition,
		Selector: selector, ObservedAtMS: time.Now().UnixMilli(), Final: true,
	})
}

// branchDisposition 根据终态原因和成员观测计算分支处置标签。
func branchDisposition(groupID string, branch ValidationBranch, last map[validationObservationKey]validationObservationState, terminalReason, winningBranchID string) string {
	if branch.ID == winningBranchID {
		return "won"
	}
	observed := len(branch.Nodes) > 0
	satisfied := observed
	for _, member := range branch.Nodes {
		state, exists := last[validationObservationKey{groupID: groupID, branchID: branch.ID, nodeID: member.NodeID}]
		observed = observed && exists
		satisfied = satisfied && exists && state.passed
	}
	if terminalReason == "passed" && satisfied {
		return "satisfied_not_winner"
	}
	if observed {
		return "not_satisfied"
	}
	return "not_observed"
}

// validationTerminalReason 将轮询错误归类为取消、超时或系统错误。
func validationTerminalReason(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if fault.IsCode(err, CodeTimeout) {
		return "timeout"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	return "system_error"
}

// validationEvidenceIsSensitive 判断验证目标或断言是否可能包含敏感值。
func validationEvidenceIsSensitive(target fingerprint.ElementTargetSpec, assertion ValidationAssertion) bool {
	if !strings.HasPrefix(assertion.Kind, "value_") && !strings.HasPrefix(assertion.Kind, "selected_set_") && assertion.Kind != "attribute_equals" && assertion.Kind != "attribute_contains" {
		return false
	}
	attributes := target.Fingerprint.Attributes
	for _, value := range []string{attributes["type"], attributes["name"], attributes["id"], attributes["autocomplete"], target.Fingerprint.ARIA.Name} {
		value = strings.ToLower(value)
		if value == "password" || value == "file" || strings.Contains(value, "token") || strings.Contains(value, "secret") || strings.Contains(value, "password") || strings.Contains(value, "api_key") {
			return true
		}
	}
	return false
}

// validationReason 返回轮询观测使用的标准判定原因。
func validationReason(passed bool) string {
	if passed {
		return "satisfied"
	}
	return "normal_unsatisfied"
}

// transitionValidation 校验并发出验证节点阶段转换，然后更新本地执行状态。
func transitionValidation(ctx context.Context, rt *Runtime, execution *StepExecution, nodeID string, next Phase) error {
	if err := execution.CanTransition(next); err != nil {
		return err
	}
	if err := rt.emit(ctx, nodeID, next); err != nil {
		return err
	}
	if next == PhaseRunning {
		occurrence, err := rt.activeOccurrence(nodeID)
		if err != nil {
			return err
		}
		execution.occurrence = occurrence
	}
	return execution.Transition(next)
}

// validationFail 将验证失败转换到失败阶段，并保留原始错误。
func validationFail(ctx context.Context, rt *Runtime, execution *StepExecution, nodeID string, cause error) error {
	phase := failurePhase(ctx)
	if err := execution.CanTransition(phase); err != nil {
		return errors.Join(cause, err)
	}
	if err := rt.emitTerminal(ctx, nodeID, phase); err != nil {
		return errors.Join(cause, err)
	}
	if err := execution.Transition(phase); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}
