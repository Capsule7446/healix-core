package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/node"
)

// RunProgram's backstop is the last thing between a host and an unclassified
// error. It has two jobs that pull in opposite directions: give every bare
// failure a code, and never relabel one that already has a code. Testing it
// through the public entry rather than by calling the classifier directly is
// the point — a classifier that works in isolation but is wired after the
// wrong return would still leak.

type failingRootNode struct{ err error }

func (*failingRootNode) ID() string { return "failing-root" }

func (n *failingRootNode) Run(context.Context, *node.Runtime) error { return n.err }

func unclassifiedFailureEntry(t *testing.T, root node.Node) CompiledEntry {
	t.Helper()
	snapshot, err := instanceSnapshotForCompilerTest(minimalCompilerPlan(), map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := compiled.Entry(mustEntryID("execution-entry"))
	if !ok {
		t.Fatal("execution-entry is missing")
	}
	entry.program.Root = root
	return entry
}

func runUnclassifiedFailure(t *testing.T, cause error) (EntryResult, error) {
	t.Helper()
	entry := unclassifiedFailureEntry(t, &failingRootNode{err: cause})
	return RunProgram(context.Background(), entry, Config{
		InstanceID: entry.InstanceID, SnapshotDigest: entry.SnapshotDigest, EntryID: entry.EntryID,
		ClaimToken: "claim", AuthorityVerifier: originGuardAuthority{}, Driver: &engineTestDriver{},
	})
}

// TestRunProgramClassifiesBareFailures covers the wrapping arm: a node that
// returns an uncoded error must not reach the host uncoded, whatever shape the
// error takes.
func TestRunProgramClassifiesBareFailures(t *testing.T) {
	secret := "token=s3cr3t https://internal.test/admin"
	tests := []struct {
		name  string
		cause error
	}{
		{name: "errors.New", cause: errors.New("bare failure")},
		{name: "fmt.Errorf", cause: fmt.Errorf("wrapped: %w", errors.New("inner"))},
		{name: "errors.Join", cause: errors.Join(errors.New("first"), errors.New("second"))},
		{name: "carrying host detail", cause: errors.New(secret)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := runUnclassifiedFailure(t, test.cause)

			if !fault.IsCode(err, node.CodeOperationFailed) {
				t.Fatalf("RunProgram() error = %v, want %q", err, node.CodeOperationFailed)
			}
			if kind, ok := fault.KindOf(err); !ok || kind != fault.Internal {
				t.Fatalf("kind = %q (%v), want %q", kind, ok, fault.Internal)
			}
			if result.ExecutionOutcome != OutcomeFailed {
				t.Fatalf("outcome = %q, want %q", result.ExecutionOutcome, OutcomeFailed)
			}
			// The original error survives for a host that asks for it, and
			// only for a host that asks: Unwrap reaches it, the public text
			// does not.
			if !errors.Is(err, test.cause) {
				t.Fatalf("the private cause was dropped: %v", err)
			}
			descriptor, ok := fault.Describe(err)
			if !ok {
				t.Fatalf("no descriptor for %v", err)
			}
			if strings.Contains(descriptor.Message(), test.cause.Error()) {
				t.Fatalf("public message echoed the cause: %q", descriptor.Message())
			}
			if strings.Contains(descriptor.Message(), "s3cr3t") || strings.Contains(descriptor.Message(), "internal.test") {
				t.Fatalf("public message leaked host detail: %q", descriptor.Message())
			}
		})
	}
}

// TestRunProgramPassesClassifiedFailuresThroughUnchanged covers the other arm.
// Re-wrapping an already-coded fault would replace the code a host branches on
// with a generic one, which is worse than not classifying at all: the specific
// failure becomes indistinguishable from an unknown one.
func TestRunProgramPassesClassifiedFailuresThroughUnchanged(t *testing.T) {
	tests := []struct {
		name string
		code fault.Code
		kind fault.Kind
	}{
		{name: "element not found", code: node.CodeElementNotFound, kind: fault.NotFound},
		{name: "healing refused", code: node.CodeHealingRefused, kind: fault.FailedPrecondition},
		{name: "worker fence stale", code: domainexecution.CodeWorkerFenceStale, kind: fault.Conflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classified, err := fault.New(test.kind, test.code, "safe message")
			if err != nil {
				t.Fatal(err)
			}

			_, runErr := runUnclassifiedFailure(t, classified)

			if !fault.IsCode(runErr, test.code) {
				t.Fatalf("RunProgram() error = %v, want %q preserved", runErr, test.code)
			}
			if fault.IsCode(runErr, node.CodeOperationFailed) {
				t.Fatalf("an already-classified fault was relabelled as a generic operation failure: %v", runErr)
			}
			if kind, ok := fault.KindOf(runErr); !ok || kind != test.kind {
				t.Fatalf("kind = %q (%v), want %q preserved", kind, ok, test.kind)
			}
			// Passed through, not re-wrapped: the fault the node produced is
			// the one the host receives.
			if runErr != error(classified) {
				t.Fatalf("the classified fault was rebuilt rather than passed through: %#v", runErr)
			}
		})
	}
}

// TestRunProgramReturnsNoErrorOnSuccess pins the nil arm, so a classifier that
// manufactured a fault from a nil cause could not pass.
func TestRunProgramReturnsNoErrorOnSuccess(t *testing.T) {
	result, err := runUnclassifiedFailure(t, nil)

	if err != nil {
		t.Fatalf("RunProgram() on a succeeding root = %v", err)
	}
	if result.ExecutionOutcome != OutcomeSucceeded {
		t.Fatalf("outcome = %q, want %q", result.ExecutionOutcome, OutcomeSucceeded)
	}
}

// TestRunProgramKeepsCancellationDistinguishable guards the interaction the
// backstop is most likely to break: context.Canceled has no fault code, so a
// naive classifier turns a cancellation into an INTERNAL operation failure and
// the host retries something the operator deliberately stopped.
func TestRunProgramKeepsCancellationDistinguishable(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			result, err := runUnclassifiedFailure(t, cause)

			if !errors.Is(err, cause) {
				t.Fatalf("RunProgram() error = %v, want it to still satisfy errors.Is(%v)", err, cause)
			}
			if result.ExecutionOutcome != OutcomeCanceled {
				t.Fatalf("outcome = %q, want %q", result.ExecutionOutcome, OutcomeCanceled)
			}
		})
	}
}
