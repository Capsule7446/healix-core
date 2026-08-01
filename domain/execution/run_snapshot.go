package execution

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

type RunSnapshotSchema int

const (
	RunSnapshotSchemaV1      RunSnapshotSchema = 1
	RunSnapshotSchemaV2      RunSnapshotSchema = 2
	RunSnapshotSchemaCurrent                   = RunSnapshotSchemaV2
	ScreenshotPolicyV1                         = 1
	HealerPolicyV1                             = 1
	MaxSnapshotStringBytes                     = 64 * 1024
)

type EnvironmentSnapshot struct {
	ID, DisplayName, BaseURL string
	Revision                 uint64
	Properties               map[string]string
	Variables                map[string]parameter.Value
}
type ScreenshotPolicySnapshot struct {
	Version     int
	Enabled     bool
	Destination string
}
type HealerWeightsSnapshot struct{ Tag, ID, RoleName, Class, Attrs, Text, Index, Neighbor, LabelText, Container float64 }
type HealerPolicySnapshot struct {
	Version               int
	ReviewCap, AppliedCap float64
	Weights               HealerWeightsSnapshot
}

func DefaultHealerPolicySnapshot() HealerPolicySnapshot {
	return HealerPolicySnapshot{Version: HealerPolicyV1, ReviewCap: .6, AppliedCap: .85, Weights: HealerWeightsSnapshot{Tag: .15, ID: .2, RoleName: .2, Class: .1, Attrs: .1, Text: .1, Index: .05, Neighbor: .1, LabelText: .15, Container: .1}}
}

type TestTaskSnapshot struct {
	ID string
}

type ExecutionFlowVersionItemSnapshot struct {
	ID                string
	TestTaskVersionID string
	SequenceNumber    int
	FlowFragmentID    string
	WorkflowVersionID string
}

type ExecutionFlowVersionSnapshot struct {
	ID              string
	ExecutionFlowID string
	VersionNumber   int
	Items           []ExecutionFlowVersionItemSnapshot
}

type InvocationEdgeKey struct {
	ParentPath InvocationPath
	StepID     string
}

type InvocationScopeSnapshot struct {
	Path               InvocationPath
	ParentPath         InvocationPath
	ParentVersionID    string
	StepID             string
	FlowFragmentID     string
	WorkflowVersionID  string
	ResolvedFromLatest bool
	Values             map[string]parameter.Value
	Bindings           map[string]parameter.Binding
}

type InstanceSnapshotInput struct {
	SchemaVersion                      RunSnapshotSchema
	RunID                              InstanceID
	ExecutionFlowID, TestTaskVersionID string
	TestTaskVersionNumber              int
	ExecutionFlow                      TestTaskSnapshot
	ExecutionFlowVersion               ExecutionFlowVersionSnapshot
	Plan                               PlanSnapshot
	Invocations                        []InvocationScopeSnapshot
	Environment                        EnvironmentSnapshot
	FailurePolicy                      FailurePolicy
	ScreenshotPolicy                   ScreenshotPolicySnapshot
	HealerPolicy                       HealerPolicySnapshot
}

type InstanceSnapshot struct {
	input  InstanceSnapshotInput
	digest string
}

func (s InstanceSnapshot) Digest() string                   { return s.digest }
func (s InstanceSnapshot) SchemaVersion() RunSnapshotSchema { return s.input.SchemaVersion }
func (s InstanceSnapshot) RunID() InstanceID                { return s.input.RunID }
func (s InstanceSnapshot) ExecutionFlowID() string          { return s.input.ExecutionFlowID }
func (s InstanceSnapshot) TestTaskVersionID() string        { return s.input.TestTaskVersionID }
func (s InstanceSnapshot) Input() InstanceSnapshotInput     { return cloneSnapshotInput(s.input) }
func (s InstanceSnapshot) Plan() PlanSnapshot               { return cloneDraft(s.input.Plan) }
func (s InstanceSnapshot) Invocations() []InvocationScopeSnapshot {
	return cloneInvocations(s.input.Invocations)
}
func (s InstanceSnapshot) Invocation(path InvocationPath) (InvocationScopeSnapshot, bool) {
	for _, invocation := range s.input.Invocations {
		if invocation.Path == path {
			return cloneInvocations([]InvocationScopeSnapshot{invocation})[0], true
		}
	}
	return InvocationScopeSnapshot{}, false
}
func (s InstanceSnapshot) Environment() EnvironmentSnapshot {
	result := cloneEnvironment(s.input.Environment)
	if s.input.SchemaVersion == RunSnapshotSchemaV1 {
		result.Variables = make(map[string]parameter.Value, len(result.Properties))
		for name, value := range result.Properties {
			result.Variables[name] = parameter.TextValue(value)
		}
	}
	return result
}

// SealInstanceSnapshot classifies its own validation failure at this exported
// boundary: an uncoded shape defect becomes
// EXECUTION_CREATE_INSTANCE_SNAPSHOT_INVALID, while a failure already
// classified by the execution plan, a workflow's step-shape envelope, or the
// environment/screenshot/healer envelope passes through unchanged.
func SealInstanceSnapshot(input InstanceSnapshotInput) (InstanceSnapshot, error) {
	sealed, err := sealRunSnapshotShape(input)
	if err != nil {
		return InstanceSnapshot{}, classifyCreateInstanceSnapshot(err)
	}
	return sealed, nil
}

func sealRunSnapshotShape(input InstanceSnapshotInput) (InstanceSnapshot, error) {
	if err := preflightRunSnapshot(input); err != nil {
		return InstanceSnapshot{}, err
	}
	input = cloneSnapshotInput(input)
	sort.Slice(input.Invocations, func(i, j int) bool {
		return input.Invocations[i].Path.String() < input.Invocations[j].Path.String()
	})
	normalizeHealerZeros(&input.HealerPolicy)
	if err := validateSnapshot(input); err != nil {
		return InstanceSnapshot{}, err
	}
	digester := sha256.New()
	encoder := canonicalEncoder{writer: digester}
	encodeSnapshot(&encoder, input)
	return InstanceSnapshot{input: input, digest: "sha256:" + hex.EncodeToString(digester.Sum(nil))}, nil
}

// HydrateInstanceSnapshot reuses EXECUTION_CREATE_INSTANCE_SNAPSHOT_CONFLICT — the
// code application/scheduling already publishes for the same remediation
// (re-read the authoritative instance before retrying) — rather than minting
// a second code for the same meaning.
func HydrateInstanceSnapshot(input InstanceSnapshotInput, storedDigest string) (InstanceSnapshot, error) {
	sealed, err := SealInstanceSnapshot(input)
	if err != nil {
		return InstanceSnapshot{}, err
	}
	if storedDigest != sealed.Digest() {
		return InstanceSnapshot{}, createInstanceSnapshotConflictError()
	}
	return sealed, nil
}

func cloneSnapshotInput(v InstanceSnapshotInput) InstanceSnapshotInput {
	v.Plan = cloneDraft(v.Plan)
	v.Invocations = cloneInvocations(v.Invocations)
	v.Environment = cloneEnvironment(v.Environment)
	v.ExecutionFlowVersion.Items = append([]ExecutionFlowVersionItemSnapshot(nil), v.ExecutionFlowVersion.Items...)
	return v
}
func cloneEnvironment(v EnvironmentSnapshot) EnvironmentSnapshot {
	properties := make(map[string]string, len(v.Properties))
	for name, value := range v.Properties {
		properties[name] = value
	}
	variables := make(map[string]parameter.Value, len(v.Variables))
	for name, value := range v.Variables {
		variables[name] = value.Clone()
	}
	v.Properties = properties
	v.Variables = variables
	return v
}
func cloneInvocations(source []InvocationScopeSnapshot) []InvocationScopeSnapshot {
	result := make([]InvocationScopeSnapshot, len(source))
	for index, invocation := range source {
		result[index] = invocation
		result[index].Values = cloneParameterValues(invocation.Values)
		result[index].Bindings = cloneBindings(invocation.Bindings)
	}
	return result
}

func normalizeHealerZeros(policy *HealerPolicySnapshot) {
	values := []*float64{&policy.ReviewCap, &policy.AppliedCap, &policy.Weights.Tag, &policy.Weights.ID, &policy.Weights.RoleName, &policy.Weights.Class, &policy.Weights.Attrs, &policy.Weights.Text, &policy.Weights.Index, &policy.Weights.Neighbor, &policy.Weights.LabelText, &policy.Weights.Container}
	for _, value := range values {
		if *value == 0 {
			*value = 0
		}
	}
}

const maxSnapshotElements = 100000
const maxSnapshotDepth = 64

func preflightRunSnapshot(input InstanceSnapshotInput) error {
	if err := validateAggregateInputBounds(input.Plan); err != nil {
		return fmt.Errorf("run snapshot plan bounds: %w", err)
	}
	remainingBytes := MaxAggregateStringBytes
	remainingElements := maxSnapshotElements
	return consumeSnapshotResources(reflect.ValueOf(input), &remainingBytes, &remainingElements, 0)
}
func consumeSnapshotResources(value reflect.Value, bytes, elements *int, depth int) error {
	if depth > maxSnapshotDepth {
		return errors.New("run snapshot exceeds resource depth limit")
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		return consumeSnapshotResources(value.Elem(), bytes, elements, depth+1)
	}
	if value.CanInterface() {
		switch typed := value.Interface().(type) {
		case parameter.Value:
			strings := []string{string(typed.Type())}
			switch typed.Type() {
			case parameter.Text:
				strings = append(strings, typed.Text())
			case parameter.Number:
				strings = append(strings, typed.Number())
			case parameter.SingleSelect:
				strings = append(strings, typed.SingleSelect())
			case parameter.MultiSelect:
				strings = append(strings, typed.MultiSelect()...)
			}
			return consumeStrings(strings, bytes, elements)
		case parameter.OptionalValue:
			if literal, present := typed.Value(); present {
				return consumeSnapshotResources(reflect.ValueOf(literal), bytes, elements, depth+1)
			}
			return nil
		case parameter.Binding:
			if err := consumeStrings([]string{string(typed.Kind())}, bytes, elements); err != nil {
				return err
			}
			if literal, ok := typed.Literal(); ok {
				return consumeSnapshotResources(reflect.ValueOf(literal), bytes, elements, depth+1)
			}
			if parent, ok := typed.ParentName(); ok {
				return consumeStrings([]string{parent}, bytes, elements)
			}
			return nil
		}
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		return consumeSnapshotResources(value.Elem(), bytes, elements, depth+1)
	}
	switch value.Kind() {
	case reflect.String:
		return consumeStrings([]string{value.String()}, bytes, elements)
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if err := consumeSnapshotResources(value.Field(i), bytes, elements, depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if err := consumeElements(value.Len(), elements); err != nil {
			return err
		}
		for i := 0; i < value.Len(); i++ {
			if err := consumeSnapshotResources(value.Index(i), bytes, elements, depth+1); err != nil {
				return err
			}
		}
	case reflect.Map:
		if err := consumeElements(value.Len(), elements); err != nil {
			return err
		}
		for _, key := range value.MapKeys() {
			if err := consumeSnapshotResources(key, bytes, elements, depth+1); err != nil {
				return err
			}
			if err := consumeSnapshotResources(value.MapIndex(key), bytes, elements, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
func consumeStrings(values []string, bytes, elements *int) error {
	if err := consumeElements(len(values), elements); err != nil {
		return err
	}
	for _, value := range values {
		if len(value) > MaxStringBytes || len(value) > *bytes {
			return errors.New("run snapshot aggregate string bytes exceed limit")
		}
		*bytes -= len(value)
	}
	return nil
}
func consumeElements(count int, remaining *int) error {
	if count < 0 || count > *remaining {
		return errors.New("run snapshot aggregate elements exceed limit")
	}
	*remaining -= count
	return nil
}

func validString(v string, required bool) bool {
	return (!required || strings.TrimSpace(v) != "") && len(v) <= MaxSnapshotStringBytes
}

func validateTestTaskVersionItemEntries(versionID string, items []ExecutionFlowVersionItemSnapshot, entries []Entry) error {
	itemsByID := make(map[string]ExecutionFlowVersionItemSnapshot, len(items))
	for index, item := range items {
		if !validString(item.ID, true) || item.TestTaskVersionID != versionID || item.SequenceNumber != index+1 || !validString(item.FlowFragmentID, true) || !validString(item.WorkflowVersionID, true) {
			return errors.New("test-task version item graph is inconsistent")
		}
		if _, exists := itemsByID[item.ID]; exists {
			return errors.New("duplicate test-task version item")
		}
		itemsByID[item.ID] = item
	}
	if len(entries) != len(items) {
		return errors.New("test-task items and execution entries must have equal count")
	}
	matchedItems := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if _, duplicate := matchedItems[entry.TestTaskItemID]; duplicate {
			return errors.New("duplicate execution entry item")
		}
		matchedItems[entry.TestTaskItemID] = struct{}{}
		item, found := itemsByID[entry.TestTaskItemID]
		if !found {
			return errors.New("test-task item execution entry is missing")
		}
		if entry.SequenceNumber != item.SequenceNumber || entry.FlowFragmentID != item.FlowFragmentID || entry.WorkflowVersionID != item.WorkflowVersionID {
			return errors.New("test-task item and execution entry identity mismatch")
		}
	}
	return nil
}

type referenceEdgeKey struct {
	ParentVersionID string
	StepID          string
}

type snapshotValidationIndexes struct {
	workflows         map[string]WorkflowSnapshot
	entriesByID       map[EntryID]Entry
	entriesByRootPath map[InvocationPath]Entry
	referenceSteps    map[referenceEdgeKey]Step
	referenceByEdge   map[referenceEdgeKey]ReferenceResolution
	stepsByWorkflowID map[string][]Step
}

func buildSnapshotValidationIndexes(plan PlanSnapshot) (snapshotValidationIndexes, error) {
	indexes := snapshotValidationIndexes{
		workflows:         make(map[string]WorkflowSnapshot, len(plan.Workflows)),
		entriesByID:       make(map[EntryID]Entry, len(plan.Entries)),
		entriesByRootPath: make(map[InvocationPath]Entry, len(plan.Entries)),
		referenceSteps:    make(map[referenceEdgeKey]Step),
		referenceByEdge:   make(map[referenceEdgeKey]ReferenceResolution, len(plan.References)),
		stepsByWorkflowID: make(map[string][]Step, len(plan.Workflows)),
	}
	for _, entry := range plan.Entries {
		if _, exists := indexes.entriesByID[entry.ID]; exists {
			return snapshotValidationIndexes{}, fmt.Errorf("duplicate execution entry %q", entry.ID)
		}
		indexes.entriesByID[entry.ID] = entry
		indexes.entriesByRootPath[RootInvocationPath(entry.ID)] = entry
	}
	for _, workflow := range plan.Workflows {
		if _, exists := indexes.workflows[workflow.VersionID]; exists {
			return snapshotValidationIndexes{}, errors.New("duplicate workflow version")
		}
		indexes.workflows[workflow.VersionID] = workflow
		steps := workflowReferenceSteps(workflow.Steps)
		indexes.stepsByWorkflowID[workflow.VersionID] = steps
		for _, step := range steps {
			key := referenceEdgeKey{ParentVersionID: workflow.VersionID, StepID: step.ID}
			if _, exists := indexes.referenceSteps[key]; exists {
				return snapshotValidationIndexes{}, errors.New("duplicate workflow reference step edge")
			}
			indexes.referenceSteps[key] = step
		}
	}
	for _, resolution := range plan.References {
		key := referenceEdgeKey{ParentVersionID: resolution.ParentVersionID, StepID: resolution.StepID}
		if _, exists := indexes.referenceByEdge[key]; exists {
			return snapshotValidationIndexes{}, errors.New("duplicate reference resolution edge")
		}
		indexes.referenceByEdge[key] = resolution
	}
	return indexes, nil
}

func validateSnapshot(v InstanceSnapshotInput) error {
	if v.SchemaVersion != RunSnapshotSchemaV1 && v.SchemaVersion != RunSnapshotSchemaV2 {
		return fmt.Errorf("unsupported run snapshot schema %d", v.SchemaVersion)
	}
	if !validString(v.RunID.String(), true) || !validString(v.ExecutionFlowID, true) || !validString(v.TestTaskVersionID, true) || v.TestTaskVersionNumber < 1 {
		return errors.New("run and test-task version identity is required")
	}
	if v.ExecutionFlow.ID != v.ExecutionFlowID ||
		v.ExecutionFlowVersion.ID != v.TestTaskVersionID || v.ExecutionFlowVersion.ExecutionFlowID != v.ExecutionFlowID ||
		v.ExecutionFlowVersion.VersionNumber != v.TestTaskVersionNumber {
		return errors.New("test-task snapshot graph identity is inconsistent")
	}
	if err := validateTestTaskVersionItemEntries(v.TestTaskVersionID, v.ExecutionFlowVersion.Items, v.Plan.Entries); err != nil {
		return err
	}
	if v.Plan.RunID != v.RunID || v.Plan.FailurePolicy != v.FailurePolicy {
		return errors.New("execution plan identity is inconsistent")
	}
	if err := v.Plan.Validate(); err != nil {
		return err
	}
	indexes, err := buildSnapshotValidationIndexes(v.Plan)
	if err != nil {
		return err
	}
	paths := make(map[InvocationPath]InvocationScopeSnapshot, len(v.Invocations))
	for _, invocation := range v.Invocations {
		if invocation.Path.Validate() != nil || !validString(invocation.FlowFragmentID, true) || !validString(invocation.WorkflowVersionID, true) {
			return errors.New("invocation identity is invalid")
		}
		if _, exists := paths[invocation.Path]; exists {
			return errors.New("duplicate invocation path")
		}
		paths[invocation.Path] = invocation
	}
	states := make(map[InvocationPath]uint8, len(paths))
	var visit func(InvocationPath) error
	visit = func(path InvocationPath) error {
		switch states[path] {
		case 1:
			return errors.New("invocation parent cycle")
		case 2:
			return nil
		}
		states[path] = 1
		parentPath := paths[path].ParentPath
		if parentPath != (InvocationPath{}) {
			if _, exists := paths[parentPath]; !exists {
				return errors.New("invocation parent path is missing")
			}
			if err := visit(parentPath); err != nil {
				return err
			}
		}
		states[path] = 2
		return nil
	}
	for path := range paths {
		if err := visit(path); err != nil {
			return err
		}
	}
	for _, invocation := range v.Invocations {
		if invocation.ParentPath == (InvocationPath{}) {
			if len(invocation.Bindings) != 0 {
				return errors.New("root invocation cannot have bindings")
			}
			if invocation.ParentVersionID != "" || invocation.StepID != "" {
				return errors.New("root invocation cannot identify a reference edge")
			}
			// An entry and the root call site inside it are different things that
			// happen to be spelled alike. The lookup goes through the derivation
			// rather than through a conversion, so nothing here has to claim that a
			// path and an entry id are the same value.
			entry, exists := indexes.entriesByRootPath[invocation.Path]
			if !exists || entry.FlowFragmentID != invocation.FlowFragmentID || entry.WorkflowVersionID != invocation.WorkflowVersionID || !equalValues(entry.Parameters.Values, invocation.Values) {
				return errors.New("root invocation and execution entry scope diverge")
			}
		} else {
			parent, exists := paths[invocation.ParentPath]
			if !exists {
				return errors.New("invocation parent path is missing")
			}
			expectedPath, pathErr := invocation.ParentPath.Child(invocation.StepID)
			if pathErr != nil || invocation.Path != expectedPath {
				return fmt.Errorf("invocation %s path is not canonical for parent %s and step %s", invocation.Path, invocation.ParentPath, invocation.StepID)
			}
			key := referenceEdgeKey{ParentVersionID: invocation.ParentVersionID, StepID: invocation.StepID}
			resolution, exists := indexes.referenceByEdge[key]
			step, stepExists := indexes.referenceSteps[key]
			if !exists || !stepExists || step.Reference == nil || parent.WorkflowVersionID != invocation.ParentVersionID || resolution.FlowFragmentID != invocation.FlowFragmentID || resolution.WorkflowVersionID != invocation.WorkflowVersionID || resolution.ResolvedFromLatest != invocation.ResolvedFromLatest {
				return errors.New("invocation reference edge is inconsistent")
			}
			resolvedValues := make(map[string]parameter.Value, len(invocation.Bindings))
			for name, binding := range invocation.Bindings {
				resolved, err := binding.Resolve(parent.Values)
				if err != nil {
					return wrapOrPropagate(err, func(cause error) error {
						return fmt.Errorf("invocation %s binding %s: %w", invocation.Path, name, cause)
					})
				}
				resolvedValues[name] = resolved
			}
			target, targetExists := indexes.workflows[invocation.WorkflowVersionID]
			if !targetExists {
				return errors.New("invocation workflow version is missing")
			}
			for _, definition := range target.Parameters {
				if _, exists := resolvedValues[definition.Name]; exists {
					continue
				}
				if value, present := definition.Default.Value(); present {
					resolvedValues[definition.Name] = value
				}
			}
			if err := validateSnapshotValues(target.Parameters, resolvedValues); err != nil {
				return wrapOrPropagate(err, func(cause error) error {
					return fmt.Errorf("invocation %s parameter values: %w", invocation.Path, cause)
				})
			}
			if !equalValues(resolvedValues, invocation.Values) {
				return errors.New("invocation values and bindings diverge")
			}
		}
		for _, value := range invocation.Values {
			if err := value.Validate(); err != nil {
				return err
			}
		}
	}
	roots := 0
	childrenByEdge := make(map[InvocationEdgeKey][]InvocationScopeSnapshot)
	for _, invocation := range v.Invocations {
		if invocation.ParentPath == (InvocationPath{}) {
			roots++
			continue
		}
		key := InvocationEdgeKey{ParentPath: invocation.ParentPath, StepID: invocation.StepID}
		childrenByEdge[key] = append(childrenByEdge[key], invocation)
	}
	if roots != len(v.Plan.Entries) {
		return errors.New("one root invocation per execution entry is required")
	}
	for _, parent := range v.Invocations {
		workflow, exists := indexes.workflows[parent.WorkflowVersionID]
		if !exists {
			return errors.New("invocation workflow version is missing")
		}
		for _, step := range indexes.stepsByWorkflowID[workflow.VersionID] {
			children := childrenByEdge[InvocationEdgeKey{ParentPath: parent.Path, StepID: step.ID}]
			if len(children) != 1 {
				return errors.New("one child invocation per concrete reference edge is required")
			}
		}
	}
	if !v.FailurePolicy.IsValid() {
		return errors.New("invalid failure policy")
	}
	if err := validateEnvironmentSnapshot(v.SchemaVersion, v.Environment, v.ScreenshotPolicy, v.HealerPolicy); err != nil {
		return err
	}
	return nil
}

func equalBindings(left, right map[string]parameter.Binding) bool {
	if len(left) != len(right) {
		return false
	}
	for name, binding := range left {
		other, exists := right[name]
		if !exists || binding.Kind() != other.Kind() {
			return false
		}
		if literal, ok := binding.Literal(); ok {
			otherLiteral, otherOK := other.Literal()
			if !otherOK || !literal.Equal(otherLiteral) {
				return false
			}
			continue
		}
		parent, ok := binding.ParentName()
		otherParent, otherOK := other.ParentName()
		if !ok || !otherOK || parent != otherParent {
			return false
		}
	}
	return true
}

func workflowReferenceSteps(steps []Step) []Step {
	var result []Step
	for _, step := range steps {
		if step.Kind == FlowFragmentReference {
			result = append(result, step)
		}
		result = append(result, workflowReferenceSteps(step.Children)...)
		if step.ValidationGroup != nil {
			for _, branch := range step.ValidationGroup.Branches {
				result = append(result, workflowReferenceSteps(branch.Steps)...)
			}
		}
	}
	return result
}

func equalValues(left, right map[string]parameter.Value) bool {
	if len(left) != len(right) {
		return false
	}
	for name, value := range left {
		other, exists := right[name]
		if !exists || !value.Equal(other) {
			return false
		}
	}
	return true
}

type canonicalEncoder struct{ writer hash.Hash }

func (e *canonicalEncoder) raw(value []byte) { _, _ = e.writer.Write(value) }
func (e *canonicalEncoder) u64(v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	e.raw(b[:])
}
func (e *canonicalEncoder) str(v string) { e.u64(uint64(len(v))); e.raw([]byte(v)) }
func (e *canonicalEncoder) boolean(v bool) {
	if v {
		e.raw([]byte{1})
	} else {
		e.raw([]byte{0})
	}
}
func encodeSnapshot(e *canonicalEncoder, v InstanceSnapshotInput) {
	e.str("healix.run-snapshot")
	e.u64(uint64(v.SchemaVersion))
	e.str(v.RunID.String())
	e.str(v.ExecutionFlowID)
	e.str(v.TestTaskVersionID)
	e.u64(uint64(v.TestTaskVersionNumber))
	e.str(v.ExecutionFlow.ID)
	e.str(v.ExecutionFlowVersion.ID)
	e.str(v.ExecutionFlowVersion.ExecutionFlowID)
	e.u64(uint64(v.ExecutionFlowVersion.VersionNumber))
	e.u64(uint64(len(v.ExecutionFlowVersion.Items)))
	for _, item := range v.ExecutionFlowVersion.Items {
		e.str(item.ID)
		e.str(item.TestTaskVersionID)
		e.u64(uint64(item.SequenceNumber))
		e.str(item.FlowFragmentID)
		e.str(item.WorkflowVersionID)
	}
	encodeCanonical(e, reflect.ValueOf(v.Plan))
	encodeCanonical(e, reflect.ValueOf(v.Invocations))
	e.str(v.Environment.ID)
	e.str(v.Environment.DisplayName)
	e.str(v.Environment.BaseURL)
	e.u64(v.Environment.Revision)
	if v.SchemaVersion == RunSnapshotSchemaV1 {
		encodeStrings(e, v.Environment.Properties)
	} else {
		encodeParameterValues(e, v.Environment.Variables)
	}
	e.str(string(v.FailurePolicy))
	e.u64(uint64(v.ScreenshotPolicy.Version))
	e.boolean(v.ScreenshotPolicy.Enabled)
	e.str(v.ScreenshotPolicy.Destination)
	e.u64(uint64(v.HealerPolicy.Version))
	e.u64(math.Float64bits(v.HealerPolicy.ReviewCap))
	e.u64(math.Float64bits(v.HealerPolicy.AppliedCap))
	for _, x := range []float64{v.HealerPolicy.Weights.Tag, v.HealerPolicy.Weights.ID, v.HealerPolicy.Weights.RoleName, v.HealerPolicy.Weights.Class, v.HealerPolicy.Weights.Attrs, v.HealerPolicy.Weights.Text, v.HealerPolicy.Weights.Index, v.HealerPolicy.Weights.Neighbor, v.HealerPolicy.Weights.LabelText, v.HealerPolicy.Weights.Container} {
		e.u64(math.Float64bits(x))
	}
}
func encodeCanonical(e *canonicalEncoder, value reflect.Value) {
	if value.Kind() == reflect.Interface {
		value = value.Elem()
	}
	if value.CanInterface() {
		switch typed := value.Interface().(type) {
		// Execution coordinates encode as the bare identity string they replaced.
		// The snapshot digest is a persisted contract, and the generic struct arm
		// below writes a field count ahead of the fields, so letting these fall
		// through would give every stored snapshot a new digest the moment a plain
		// string field became a value object. That is a storage migration, not a
		// rename, and nothing the snapshot means actually changed.
		case InstanceID:
			e.str(typed.String())
			return
		case EntryID:
			e.str(typed.String())
			return
		case StepExecutionID:
			e.str(typed.String())
			return
		case InvocationPath:
			e.str(typed.String())
			return
		case parameter.Value:
			encodeValue(e, typed)
			return
		case parameter.OptionalValue:
			literal, present := typed.Value()
			e.boolean(present)
			if present {
				encodeValue(e, literal)
			}
			return
		case parameter.Binding:
			e.str(string(typed.Kind()))
			if literal, ok := typed.Literal(); ok {
				encodeValue(e, literal)
			} else if name, ok := typed.ParentName(); ok {
				e.str(name)
			}
			return
		}
	}
	if value.Kind() == reflect.Pointer {
		e.boolean(!value.IsNil())
		if !value.IsNil() {
			encodeCanonical(e, value.Elem())
		}
		return
	}
	switch value.Kind() {
	case reflect.Struct:
		e.u64(uint64(value.NumField()))
		for index := 0; index < value.NumField(); index++ {
			encodeCanonical(e, value.Field(index))
		}
	case reflect.Slice:
		e.u64(uint64(value.Len()))
		for index := 0; index < value.Len(); index++ {
			encodeCanonical(e, value.Index(index))
		}
	case reflect.Map:
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		e.u64(uint64(len(keys)))
		for _, key := range keys {
			e.str(key.String())
			encodeCanonical(e, value.MapIndex(key))
		}
	case reflect.String:
		e.str(value.String())
	case reflect.Bool:
		e.boolean(value.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.u64(uint64(value.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		e.u64(value.Uint())
	case reflect.Float64:
		f := value.Float()
		if f == 0 {
			f = 0
		}
		e.u64(math.Float64bits(f))
	default:
		e.str(fmt.Sprint(value.Interface()))
	}
}

func sortedKeys[V any](m map[string]V) []string {
	k := make([]string, 0, len(m))
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}
func encodeStrings(e *canonicalEncoder, m map[string]string) {
	e.u64(uint64(len(m)))
	for _, k := range sortedKeys(m) {
		e.str(k)
		e.str(m[k])
	}
}
func encodeParameterValues(e *canonicalEncoder, values map[string]parameter.Value) {
	e.u64(uint64(len(values)))
	for _, name := range sortedKeys(values) {
		e.str(name)
		encodeValue(e, values[name])
	}
}
func encodeValue(e *canonicalEncoder, v parameter.Value) {
	e.str(string(v.Type()))
	switch v.Type() {
	case parameter.Text:
		e.str(v.Text())
	case parameter.Number:
		e.str(v.Number())
	case parameter.Boolean:
		e.boolean(v.Boolean())
	case parameter.SingleSelect:
		e.str(v.SingleSelect())
	case parameter.MultiSelect:
		items := v.MultiSelect()
		e.u64(uint64(len(items)))
		for _, x := range items {
			e.str(x)
		}
	}
}
