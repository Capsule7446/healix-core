package execution

// mustEntryID spells an entry identity in a fixture where the value is a
// literal the test author already knows is well formed. Tests that care about
// rejection call NewEntryID directly and assert on its error.
func mustEntryID(value string) EntryID {
	id, err := NewEntryID(value)
	if err != nil {
		panic(err)
	}
	return id
}

// mustInstanceID spells an instance identity in a fixture where the value is a
// literal the test author already knows is well formed.
func mustInstanceID(value string) InstanceID {
	id, err := NewInstanceID(value)
	if err != nil {
		panic(err)
	}
	return id
}
