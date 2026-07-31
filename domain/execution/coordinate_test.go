package execution

import "testing"

func TestExecutionIdentityValuesRejectBlankAndOversizedValues(t *testing.T) {
	tests := []struct {
		name string
		make func(string) error
	}{
		{"instance", func(value string) error { _, err := NewInstanceID(value); return err }},
		{"entry", func(value string) error { _, err := NewEntryID(value); return err }},
		{"invocation", func(value string) error { _, err := ParseInvocationPath(value); return err }},
		{"step execution", func(value string) error { _, err := NewStepExecutionID(value); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, value := range []string{"", " \t", string(make([]byte, MaxStringBytes+1))} {
				if err := test.make(value); err == nil {
					t.Fatalf("accepted invalid value length %d", len(value))
				}
			}
		})
	}
}

func TestInvocationPathConstructorsPreserveCanonicalEncoding(t *testing.T) {
	entry, err := NewEntryID("entry")
	if err != nil {
		t.Fatal(err)
	}
	root := RootInvocationPath(entry)
	child, err := root.Child("a/b")
	if err != nil {
		t.Fatal(err)
	}
	if root.String() != "entry" || child.String() != "entry/3:a/b" {
		t.Fatalf("paths = %q %q", root.String(), child.String())
	}
	if _, err := ParseInvocationPath("entry/1:a/b"); err == nil {
		t.Fatal("accepted noncanonical invocation path")
	}
}

func TestExecutionIdentityValuesRoundTrip(t *testing.T) {
	instance, err := NewInstanceID("run-1")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := NewEntryID("entry-1")
	if err != nil {
		t.Fatal(err)
	}
	step, err := NewStepExecutionID("step-1")
	if err != nil {
		t.Fatal(err)
	}
	if instance.String() != "run-1" || entry.String() != "entry-1" || step.String() != "step-1" {
		t.Fatalf("identity round trip failed: %q %q %q", instance.String(), entry.String(), step.String())
	}
}
