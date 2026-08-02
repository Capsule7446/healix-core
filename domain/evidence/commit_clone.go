package evidence

// Clone is the one deep copy of a step transition commit.
//
// The application layer used to produce its owned copy by marshalling the
// commit to JSON and unmarshalling it back. That silently destroyed the thing
// the copy exists to protect. Every execution coordinate is a struct whose only
// field is unexported, so encoding/json wrote {} and read back a zero value: an
// owned commit came out with a blank step execution id, a blank entry id and
// blank identities on every observation. ValidationGroupTerminalObservation's
// unexported expectedMembers went the same way.
//
// The damage was invisible at the call site because Validate and the fence
// binding check both ran on the original, before the round trip, and nothing
// re-checked the copy on the way to the Host.
//
// A round trip through any encoding can only ever reproduce what that encoding
// can see, which makes it the wrong tool for copying a type that keeps state
// the caller is not allowed to reach. The type owns its copy instead.
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

// Clone carries the unexported expected-member list across. The exported fields
// are all values, so the group's own copy is the only place that list can be
// duplicated from outside this package.
func (o ValidationGroupTerminalObservation) Clone() ValidationGroupTerminalObservation {
	cloned := o
	cloned.expectedMembers = append([]ValidationMemberIdentity(nil), o.expectedMembers...)
	return cloned
}
