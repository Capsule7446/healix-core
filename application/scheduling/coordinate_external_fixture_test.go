package scheduling_test

import domain "github.com/Capsule7446/healix-core/domain/execution"

// mustInstanceID spells an instance identity in a fixture where the value is a
// literal the test author already knows is well formed.
func mustInstanceID(value string) domain.InstanceID {
	id, err := domain.NewInstanceID(value)
	if err != nil {
		panic(err)
	}
	return id
}

// mustEntryID spells a coordinate in a fixture where the value is a literal the
// test author already knows is well formed.
func mustEntryID(value string) domain.EntryID {
	id, err := domain.NewEntryID(value)
	if err != nil {
		panic(err)
	}
	return id
}

// mustStepExecutionID spells a coordinate in a fixture where the value is a literal the
// test author already knows is well formed.
func mustStepExecutionID(value string) domain.StepExecutionID {
	id, err := domain.NewStepExecutionID(value)
	if err != nil {
		panic(err)
	}
	return id
}
