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
