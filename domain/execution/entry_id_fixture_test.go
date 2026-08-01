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
