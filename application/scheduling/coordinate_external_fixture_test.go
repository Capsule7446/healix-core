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
