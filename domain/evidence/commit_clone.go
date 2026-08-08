package evidence

// Clone 返回步骤迁移提交的深拷贝，复制所有事实切片及校验组的私有成员列表，隔离调用方所有权。
func (c StepTransitionCommit) Clone() StepTransitionCommit {
	cloned := c
	cloned.FinalValidations = append([]ValidationObservation(nil), c.FinalValidations...)
	cloned.HealObservations = append([]HealObservation(nil), c.HealObservations...)
	cloned.OriginalSelectorResets = append([]HealCandidateReset(nil), c.OriginalSelectorResets...)
	cloned.FinalValidationGroups = make([]ValidationGroupTerminalObservation, len(c.FinalValidationGroups))
	for index, group := range c.FinalValidationGroups {
		cloned.FinalValidationGroups[index] = group.Clone()
	}
	return cloned
}

// Clone 复制验证组终态观测及其未导出的 expectedMembers 列表。
func (o ValidationGroupTerminalObservation) Clone() ValidationGroupTerminalObservation {
	cloned := o
	cloned.expectedMembers = append([]ValidationMemberIdentity(nil), o.expectedMembers...)
	return cloned
}
