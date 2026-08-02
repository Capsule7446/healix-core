package scheduling

import (
	"errors"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestBuildInstanceSnapshotRejectsMissingAndExtraCommandGraphEdges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CreateInstanceCommand, *ResolvedCreateInstance)
		want   string
	}{
		{name: "missing item values", mutate: func(command *CreateInstanceCommand, _ *ResolvedCreateInstance) {
			delete(command.Entries, "item-1")
			command.Entries["unknown"] = map[string]parameter.Value{}
		}, want: "values are missing"},
		{name: "missing root invocation", mutate: func(_ *CreateInstanceCommand, resolved *ResolvedCreateInstance) {
			for index := range resolved.Invocations {
				if resolved.Invocations[index].ParentPath == (execution.InvocationPath{}) {
					resolved.Invocations[index].ParentPath = mustInvocationPath("unexpected-parent")
					break
				}
			}
		}, want: "root invocation is missing"},
		{name: "extra item values", mutate: func(command *CreateInstanceCommand, _ *ResolvedCreateInstance) {
			command.Entries["unknown"] = map[string]parameter.Value{}
		}, want: "unknown test-task item values"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := validCreateInstanceCommand()
			resolved := validResolvedCreateInstance(t, command)
			test.mutate(&command, &resolved)
			_, err := BuildInstanceSnapshot(command, resolved)
			if err == nil {
				t.Fatal("BuildInstanceSnapshot() accepted an invalid command graph")
			}
			// The detail is now a private cause and the public code is what a host
			// switches on, so both are asserted: the classification, and that the
			// detail survives for diagnostics without being public text.
			if !fault.IsCode(err, CodeCreateInstanceCatalogGraphUnresolvable) {
				t.Fatalf("BuildInstanceSnapshot() error = %v, want code %s", err, CodeCreateInstanceCatalogGraphUnresolvable)
			}
			if descriptor, ok := fault.Describe(err); !ok || strings.Contains(descriptor.Message(), test.want) {
				t.Fatalf("public message must not carry the detail: %#v", descriptor)
			}
			if cause := errors.Unwrap(err); cause == nil || !strings.Contains(cause.Error(), test.want) {
				t.Fatalf("private cause = %v, want it to retain %q", cause, test.want)
			}
		})
	}
}

func TestCreateInstanceCommandInvalidErrorExposesSafeStableContract(t *testing.T) {
	cause := errors.New("command-sensitive-id=cmd-secret value=credential-secret")
	err := createInstanceCommandInvalidError(cause)
	descriptor, ok := fault.Describe(err)
	if !ok ||
		descriptor.Code() != CodeCreateInstanceCommandInvalid ||
		descriptor.Kind() != fault.InvalidArgument ||
		descriptor.Message() != "create-instance command is invalid" ||
		len(descriptor.Params()) != 0 ||
		len(descriptor.Violations()) != 0 ||
		!errors.Is(err, cause) {
		t.Fatalf("descriptor/error = %#v/%v", descriptor, err)
	}
	for _, sensitive := range []string{"cmd-secret", "credential-secret", cause.Error()} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("public error leaked %q: %q", sensitive, err.Error())
		}
	}
}

func TestCreateInstanceCommandConflictErrorExposesSafeStableContract(t *testing.T) {
	err := createInstanceCommandConflictError()
	descriptor, ok := fault.Describe(err)
	if !ok ||
		descriptor.Code() != CodeCreateInstanceCommandConflict ||
		descriptor.Kind() != fault.Conflict ||
		descriptor.Message() != "create-instance command conflicts with an existing request" ||
		len(descriptor.Params()) != 0 ||
		len(descriptor.Violations()) != 0 {
		t.Fatalf("descriptor/error = %#v/%v", descriptor, err)
	}
	for _, sensitive := range []string{"command-sensitive-id", "sha256:request-secret", "payload-secret"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("public error leaked %q: %q", sensitive, err.Error())
		}
	}
}

func TestCreateInstanceSnapshotConflictErrorExposesSafeStableContract(t *testing.T) {
	err := createInstanceSnapshotConflictError()
	descriptor, ok := fault.Describe(err)
	if !ok ||
		descriptor.Code() != CodeCreateInstanceSnapshotConflict ||
		descriptor.Kind() != fault.Conflict ||
		descriptor.Message() != "create-instance snapshot conflicts with the authoritative instance" ||
		len(descriptor.Params()) != 0 ||
		len(descriptor.Violations()) != 0 {
		t.Fatalf("descriptor/error = %#v/%v", descriptor, err)
	}
	for _, sensitive := range []string{"run-sensitive-id", "sha256:snapshot-secret", "payload-secret"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("public error leaked %q: %q", sensitive, err.Error())
		}
	}
}

func TestCreateInstanceAdapterContractViolationExposesSafeStableContract(t *testing.T) {
	cause := errors.New("operation=insert command=command-secret digest=sha256:secret payload=value-secret")
	err := createInstanceAdapterContractViolationError(cause)
	descriptor, ok := fault.Describe(err)
	if !ok ||
		descriptor.Code() != CodeCreateInstanceAdapterContractViolation ||
		descriptor.Kind() != fault.Internal ||
		descriptor.Message() != "create-instance adapter returned an invalid authoritative result" ||
		len(descriptor.Params()) != 0 ||
		len(descriptor.Violations()) != 0 ||
		!errors.Is(err, cause) {
		t.Fatalf("descriptor/error = %#v/%v", descriptor, err)
	}
	for _, sensitive := range []string{"command-secret", "sha256:secret", "value-secret", cause.Error()} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("public error leaked %q: %q", sensitive, err.Error())
		}
	}
}

func TestCreateInstanceCatalogGraphUnresolvableErrorExposesSafeStableContract(t *testing.T) {
	cause := errors.New("operation=resolve binding catalog=workflow-secret value=credential-secret")
	err := createInstanceCatalogGraphUnresolvableError(cause)
	descriptor, ok := fault.Describe(err)
	if !ok ||
		descriptor.Code() != CodeCreateInstanceCatalogGraphUnresolvable ||
		descriptor.Kind() != fault.FailedPrecondition ||
		descriptor.Message() != "create-instance catalog graph is unavailable or invalid" ||
		len(descriptor.Params()) != 0 ||
		len(descriptor.Violations()) != 0 ||
		!errors.Is(err, cause) {
		t.Fatalf("descriptor/error = %#v/%v", descriptor, err)
	}
	for _, sensitive := range []string{"resolve binding", "workflow-secret", "credential-secret", cause.Error()} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("public error leaked %q: %q", sensitive, err.Error())
		}
	}
}

func TestCreateInstanceRetryableErrorExposesSafeStableContract(t *testing.T) {
	cause := errors.New("transaction=transaction-secret command=command-secret")
	err := createInstanceRetryableError(cause)
	descriptor, ok := fault.Describe(err)
	if !ok ||
		descriptor.Code() != CodeCreateInstanceRetryable ||
		descriptor.Kind() != fault.Unavailable ||
		descriptor.Message() != "create-instance outcome is temporarily unavailable" ||
		len(descriptor.Params()) != 0 ||
		len(descriptor.Violations()) != 0 ||
		!errors.Is(err, cause) {
		t.Fatalf("descriptor/error = %#v/%v", descriptor, err)
	}
	for _, sensitive := range []string{"transaction-secret", "command-secret", cause.Error()} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("public error leaked %q: %q", sensitive, err.Error())
		}
	}
}

func TestDecideAdvanceRejectsUnsealedSnapshot(t *testing.T) {
	decision, err := DecideAdvance(execution.InstanceSnapshot{}, nil)
	if !fault.IsCode(err, CodeEntryStatesInvalid) || strings.Contains(err.Error(), "unsealed run snapshot") || decision.FinalStatus != nil || decision.NextEntryID != (execution.EntryID{}) || len(decision.Transitions) != 0 {
		t.Fatalf("DecideAdvance() = (%#v, %v)", decision, err)
	}
}
