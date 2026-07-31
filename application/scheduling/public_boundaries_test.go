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
