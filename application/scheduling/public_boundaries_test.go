package scheduling

import (
	"errors"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestBuildRunSnapshotRejectsMissingAndExtraCommandGraphEdges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CreateRunCommand, *ResolvedCreateRun)
		want   string
	}{
		{name: "missing item values", mutate: func(command *CreateRunCommand, _ *ResolvedCreateRun) {
			delete(command.Entries, "item-1")
			command.Entries["unknown"] = map[string]parameter.Value{}
		}, want: "values are missing"},
		{name: "missing root invocation", mutate: func(_ *CreateRunCommand, resolved *ResolvedCreateRun) {
			for index := range resolved.Invocations {
				if resolved.Invocations[index].ParentPath == "" {
					resolved.Invocations[index].ParentPath = "unexpected-parent"
					break
				}
			}
		}, want: "root invocation is missing"},
		{name: "extra item values", mutate: func(command *CreateRunCommand, _ *ResolvedCreateRun) {
			command.Entries["unknown"] = map[string]parameter.Value{}
		}, want: "unknown test-task item values"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := validCreateRunCommand()
			resolved := validResolvedCreateRun(t, command)
			test.mutate(&command, &resolved)
			if _, err := BuildRunSnapshot(command, resolved); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("BuildRunSnapshot() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCreateRunCommandInvalidErrorExposesSafeStableContract(t *testing.T) {
	cause := errors.New("command-sensitive-id=cmd-secret value=credential-secret")
	err := createRunCommandInvalidError(cause)
	descriptor, ok := fault.Describe(err)
	if !ok ||
		descriptor.Code() != CodeCreateRunCommandInvalid ||
		descriptor.Kind() != fault.InvalidArgument ||
		descriptor.Message() != "create-run command is invalid" ||
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

func TestCreateRunCommandConflictErrorExposesSafeStableContract(t *testing.T) {
	err := createRunCommandConflictError()
	descriptor, ok := fault.Describe(err)
	if !ok ||
		descriptor.Code() != CodeCreateRunCommandConflict ||
		descriptor.Kind() != fault.Conflict ||
		descriptor.Message() != "create-run command conflicts with an existing request" ||
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

func TestCreateRunSnapshotConflictErrorExposesSafeStableContract(t *testing.T) {
	err := createRunSnapshotConflictError()
	descriptor, ok := fault.Describe(err)
	if !ok ||
		descriptor.Code() != CodeCreateRunSnapshotConflict ||
		descriptor.Kind() != fault.Conflict ||
		descriptor.Message() != "create-run snapshot conflicts with the authoritative run" ||
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

func TestCreateRunTypedErrorsCoverNilCauseAndUnwrap(t *testing.T) {
	catalog := &CreateRunCatalogGraphError{Operation: "resolve"}
	if got := catalog.Error(); !strings.Contains(got, "resolve") || strings.HasSuffix(got, ": ") {
		t.Fatalf("catalog error = %q", got)
	}
	if catalog.Unwrap() != nil || !errors.Is(catalog, ErrCreateRunCatalogGraph) {
		t.Fatalf("catalog classification/unwrap = %v/%v", errors.Is(catalog, ErrCreateRunCatalogGraph), catalog.Unwrap())
	}

	retryable := &CreateRunRetryableError{Operation: "transaction"}
	if got := retryable.Error(); !strings.Contains(got, "transaction") || strings.HasSuffix(got, ": ") {
		t.Fatalf("retryable error = %q", got)
	}
	if retryable.Unwrap() != nil || !errors.Is(retryable, ErrCreateRunRetryable) {
		t.Fatalf("retryable classification/unwrap = %v/%v", errors.Is(retryable, ErrCreateRunRetryable), retryable.Unwrap())
	}
}

func TestDecideAdvanceRejectsUnsealedSnapshot(t *testing.T) {
	decision, err := DecideAdvance(execution.RunSnapshot{}, nil)
	if !fault.IsCode(err, CodeEntryStatesInvalid) || strings.Contains(err.Error(), "unsealed run snapshot") || decision.FinalStatus != nil || decision.NextExecutionID != "" || len(decision.Transitions) != 0 {
		t.Fatalf("DecideAdvance() = (%#v, %v)", decision, err)
	}
}
