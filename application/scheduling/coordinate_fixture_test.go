package scheduling

import "github.com/Capsule7446/healix-core/domain/execution"

// mustEntryID spells an entry identity in a fixture where the value is a
// literal the test author already knows is well formed.
func mustEntryID(value string) execution.EntryID {
	id, err := execution.NewEntryID(value)
	if err != nil {
		panic(err)
	}
	return id
}

// mustInstanceID spells an instance identity in a fixture where the value is a
// literal the test author already knows is well formed.
func mustInstanceID(value string) execution.InstanceID {
	id, err := execution.NewInstanceID(value)
	if err != nil {
		panic(err)
	}
	return id
}

// mustStepExecutionID spells a coordinate in a fixture where the value is a literal the
// test author already knows is well formed.
func mustStepExecutionID(value string) execution.StepExecutionID {
	id, err := execution.NewStepExecutionID(value)
	if err != nil {
		panic(err)
	}
	return id
}

// mustInvocationPath spells a call-site path in a fixture where the value is a
// literal the test author already knows is canonical.
func mustInvocationPath(value string) execution.InvocationPath {
	path, err := execution.ParseInvocationPath(value)
	if err != nil {
		panic(err)
	}
	return path
}

// optionalInvocationPath maps the fixture spelling of "this is a root call
// site" — an empty string — onto the unset path rather than failing to parse.
func optionalInvocationPath(value string) execution.InvocationPath {
	if value == "" {
		return execution.InvocationPath{}
	}
	return mustInvocationPath(value)
}
