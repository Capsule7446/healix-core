package contract_test

import "github.com/Capsule7446/healix-core/domain/execution"

// mustInstanceID spells an instance identity in a fixture where the value is a
// literal the test author already knows is well formed.
func mustInstanceID(value string) execution.InstanceID {
	id, err := execution.NewInstanceID(value)
	if err != nil {
		panic(err)
	}
	return id
}

// mustEntryID spells a coordinate in a fixture where the value is a literal the
// test author already knows is well formed.
func mustEntryID(value string) execution.EntryID {
	id, err := execution.NewEntryID(value)
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
