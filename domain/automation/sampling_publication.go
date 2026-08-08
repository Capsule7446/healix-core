package automation

import (
	"errors"
	"fmt"
	"strings"
)

// SamplingElementTargetPublication 描述采样节点发布所需的临时身份、解析策略和聚合权限。
type SamplingElementTargetPublication struct {
	TemporaryElementTargetID string
	ResolutionMode           string
	Aggregate                ElementTargetAggregate
	ExpectedRevision         Revision
	ExpectedCurrentVersionID string
	PublishVersion           bool
}

// SamplingPublication 保存采样得到的元素目标发布集合和流程片段聚合。
type SamplingPublication struct {
	Nodes        []SamplingElementTargetPublication
	FlowFragment FlowFragmentAggregate
}

// SamplingNodeMapping 记录临时元素目标到正式目标及版本的映射。
type SamplingNodeMapping struct {
	TemporaryElementTargetID string
	ElementTargetID          string
	ElementTargetVersionID   string
	ResolutionMode           string
}

// SamplingPublicationResult 返回发布后的流程片段版本和节点映射。
type SamplingPublicationResult struct {
	FlowFragmentID    string
	WorkflowVersionID string
	VersionNumber     int
	Nodes             []SamplingNodeMapping
}

// Clone 返回采样发布内容的深复制，不与原聚合或节点切片共享引用。
func (p SamplingPublication) Clone() SamplingPublication {
	cloned := SamplingPublication{FlowFragment: cloneWorkflowAggregate(p.FlowFragment)}
	cloned.Nodes = make([]SamplingElementTargetPublication, len(p.Nodes))
	for index, node := range p.Nodes {
		cloned.Nodes[index] = node
		cloned.Nodes[index].Aggregate = cloneNodeAggregate(node.Aggregate)
	}
	return cloned
}

// containsReferenceableElementTargetVersion 判断版本是否存在、未删除且可被引用。
func containsReferenceableElementTargetVersion(aggregate ElementTargetAggregate, versionID string) bool {
	if aggregate.Current.ID == versionID && aggregate.Current.DeletedAt == 0 {
		return true
	}
	for _, version := range aggregate.Versions {
		if version.ID == versionID && version.DeletedAt == 0 {
			return true
		}
	}
	return false
}

// Validate 在唯一导出边界归类发布内容校验错误；内部普通错误作为私有原因传递，身份值不会写入公共文本。
func (p SamplingPublication) Validate() error {
	return classifySamplingPublicationContent(p.validateContent())
}

// validateContent 校验流程片段、节点内容、解析策略及其并发权限。
func (p SamplingPublication) validateContent() error {
	if err := p.FlowFragment.Validate(); err != nil {
		// FlowFragmentAggregate.Validate 已返回 AUTOMATION_FLOW_FRAGMENT_INVALID，此处保持原错误码。
		return err
	}
	seen := make(map[string]struct{}, len(p.Nodes))
	formalNodes := make(map[string]struct{}, len(p.Nodes))
	formalVersions := make(map[string]struct{}, len(p.Nodes))
	decisions := make(map[string]struct{}, len(p.Nodes))
	// 违规使用调用方切片中的从零开始节点位置定位。临时/正式元素目标身份、选定版本身份和解析模式均为调用方数据，不写入错误文本。
	for index, node := range p.Nodes {
		if strings.TrimSpace(node.TemporaryElementTargetID) == "" {
			return fmt.Errorf("sampled node %d temporary id is required", index)
		}
		switch node.ResolutionMode {
		case "CREATE", "MERGE", "REUSE":
		default:
			return fmt.Errorf("sampled node %d has an unsupported resolution mode", index)
		}
		if _, ok := seen[node.TemporaryElementTargetID]; ok {
			return fmt.Errorf("duplicate sampled node at %d", index)
		}
		seen[node.TemporaryElementTargetID] = struct{}{}
		// CREATE、MERGE 和 REUSE 均校验内容，避免无选择器、零版本号或未知来源进入幂等摘要和事务。
		// REUSE 使用选定版本形式：Current 保存历史版本，
		// ElementTarget.CurrentVersionID 仍保存用于比较交换的实时指针，因此聚合的当前指针一致性规则不适用；
		// 下方权限检查覆盖指针，这里覆盖选择器、版本号和来源等内容。
		validateNode := node.Aggregate.Validate
		if node.ResolutionMode == "REUSE" {
			validateNode = func() error {
				return node.Aggregate.Current.ValidateFor(node.Aggregate.ElementTarget)
			}
		}
		if err := validateNode(); err != nil {
			// ElementTargetAggregate.Validate 已返回 AUTOMATION_ELEMENT_TARGET_INVALID，此处保持原错误码。
			return err
		}
		if strings.TrimSpace(node.Aggregate.ElementTarget.ID) == "" || node.Aggregate.Current.ElementTargetID != node.Aggregate.ElementTarget.ID || node.Aggregate.Current.DeletedAt != 0 {
			return fmt.Errorf("sampled node %d selected version is not referenceable", index)
		}
		if node.ResolutionMode != "REUSE" && !containsReferenceableElementTargetVersion(node.Aggregate, node.Aggregate.Current.ID) {
			return fmt.Errorf("sampled node %d selected version is not referenceable", index)
		}
		switch node.ResolutionMode {
		case "CREATE":
			if node.ExpectedRevision != 0 || node.ExpectedCurrentVersionID != "" || !node.PublishVersion || node.Aggregate.Current.VersionNumber != 1 {
				return fmt.Errorf("sampled node %d new ownership must publish version 1 without current-node authority", index)
			}
		case "MERGE":
			expectedNextRevision, err := node.ExpectedRevision.Next()
			if err != nil {
				// Revision.Next 已返回 AUTOMATION_REVISION_EXHAUSTED，此处保持原错误码。
				return err
			}
			if node.ExpectedRevision == 0 || node.ExpectedCurrentVersionID == "" || !node.PublishVersion || node.Aggregate.ElementTarget.Revision != expectedNextRevision {
				return fmt.Errorf("sampled node %d merge requires current revision and version authority", index)
			}
			if node.ExpectedCurrentVersionID == node.Aggregate.Current.ID {
				return fmt.Errorf("sampled node %d cannot publish the expected current version again", index)
			}
			if node.Aggregate.Current.VersionNumber < 2 {
				return fmt.Errorf("sampled node %d merge must publish version 2 or later", index)
			}
		case "REUSE":
			if node.ExpectedRevision == 0 || node.PublishVersion || node.ExpectedCurrentVersionID == "" || node.Aggregate.ElementTarget.CurrentVersionID != node.ExpectedCurrentVersionID || node.Aggregate.ElementTarget.Revision != node.ExpectedRevision || !containsReferenceableElementTargetVersion(node.Aggregate, node.Aggregate.Current.ID) {
				return fmt.Errorf("sampled node %d reuse must keep current aggregate authority and select a referenceable version", index)
			}
		}
		if _, ok := formalNodes[node.Aggregate.ElementTarget.ID]; ok {
			return fmt.Errorf("duplicate formal sampled node at %d", index)
		}
		formalNodes[node.Aggregate.ElementTarget.ID] = struct{}{}
		if _, ok := formalVersions[node.Aggregate.Current.ID]; ok {
			return fmt.Errorf("duplicate formal sampled node version at %d", index)
		}
		formalVersions[node.Aggregate.Current.ID] = struct{}{}
		decisions[node.Aggregate.ElementTarget.ID+"\x00"+node.Aggregate.Current.ID] = struct{}{}
	}
	var validateReferences func([]FlowFragmentStep) error
	validateReferences = func(steps []FlowFragmentStep) error {
		for _, step := range steps {
			if step.ElementTargetID != "" {
				if _, exists := decisions[step.ElementTargetID+"\x00"+step.ElementTargetVersionID]; !exists {
					// 步骤显示名和两个身份 ID 均为调用方数据；递归遍历没有扁平索引，
					// 因此错误文本不回显这些值，也不使用位置编号。
					return errors.New("a sampled workflow step has no matching node decision")
				}
			}
			if err := validateReferences(step.Children); err != nil {
				return err
			}
			if step.ValidationGroup != nil {
				for _, branch := range step.ValidationGroup.Branches {
					if err := validateReferences(branch.Steps); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := validateReferences(p.FlowFragment.Current.Definition.Steps); err != nil {
		return err
	}
	return nil
}
