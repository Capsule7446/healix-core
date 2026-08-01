package execution

import domainexecution "github.com/Capsule7446/healix-core/domain/execution"

// mustEntryID spells an entry identity in a fixture where the value is a
// literal the test author already knows is well formed.
func mustEntryID(value string) domainexecution.EntryID {
	id, err := domainexecution.NewEntryID(value)
	if err != nil {
		panic(err)
	}
	return id
}
