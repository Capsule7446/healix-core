package execution

import (
	"context"
	"reflect"
	"testing"
	"time"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

type recordingAuthorizer struct {
	authorizeCount int
	events         *[]string
	authorizeErr   error
}

func (a *recordingAuthorizer) AuthorizeEntry(_ context.Context, _ domainexecution.WorkerFence, _ domainexecution.Entry) error {
	a.authorizeCount++
	*a.events = append(*a.events, "authorize")
	return a.authorizeErr
}

// TestEntryAuthorizationFailurePreventsSessionCreation verifies that when the
// authorizer rejects the fence, no browser session is created.
func TestEntryAuthorizationFailurePreventsSessionCreation(t *testing.T) {
	events := []string{}
	factory := &sessionFactoryFixture{events: &events}
	authorizer := &recordingAuthorizer{events: &events, authorizeErr: domainexecution.NewStaleWorkerFenceError()}
	runner := &entryRunnerFixture{events: &events}

	err := mustEntryExecutorWithAuthorizer(t, authorizer, factory, runner).Execute(
		context.Background(),
		domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "claim"},
		domainexecution.Entry{ID: mustEntryID("entry")},
	)

	if err == nil {
		t.Fatal("Execute() returned nil, want error")
	}
	if !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) {
		t.Fatalf("Execute() error = %v, want code %s", err, domainexecution.CodeWorkerFenceStale)
	}
	if len(factory.sessions) != 0 {
		t.Fatalf("factory.Create created %d sessions, want 0", len(factory.sessions))
	}
	if authorizer.authorizeCount != 1 {
		t.Fatalf("authorizer.AuthorizeEntry called %d times, want 1", authorizer.authorizeCount)
	}
}

// TestEntryAuthorizationOrder verifies that the authorizer runs before the
// factory when authorization succeeds.
func TestEntryAuthorizationOrder(t *testing.T) {
	events := []string{}
	authorizer := &recordingAuthorizer{events: &events}
	factory := &sessionFactoryFixture{events: &events}
	runner := &entryRunnerFixture{events: &events}

	err := mustEntryExecutorWithAuthorizer(t, authorizer, factory, runner).Execute(
		context.Background(),
		domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "claim"},
		domainexecution.Entry{ID: mustEntryID("entry")},
	)

	if err != nil {
		t.Fatalf("Execute() = %v, want nil", err)
	}
	want := []string{"authorize", "create:entry", "run:entry", "close:entry"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

// TestEntryExecutorRejectsNilAuthorizer verifies that a nil authorizer is
// rejected at construction.
func TestEntryExecutorRejectsNilAuthorizer(t *testing.T) {
	factory := &sessionFactoryFixture{events: &[]string{}}
	runner := &entryRunnerFixture{events: &[]string{}}
	_, err := NewEntryExecutor(nil, factory, runner, time.Second)
	if err == nil {
		t.Fatal("NewEntryExecutor with nil authorizer returned nil error")
	}
	if !fault.IsCode(err, CodeEntryExecutorConfigurationInvalid) {
		t.Fatalf("error = %v, want code %s", err, CodeEntryExecutorConfigurationInvalid)
	}
}

// TestEntryExecutorMalformedFenceBeforeAuthorizer verifies that a malformed
// fence returns CodeWorkerFenceInvalid before the authorizer or factory are
// reached.
func TestEntryExecutorMalformedFenceBeforeAuthorizer(t *testing.T) {
	events := []string{}
	authorizer := &recordingAuthorizer{events: &events}
	factory := &sessionFactoryFixture{events: &events}
	runner := &entryRunnerFixture{events: &events}

	err := mustEntryExecutorWithAuthorizer(t, authorizer, factory, runner).Execute(
		context.Background(),
		domainexecution.WorkerFence{InstanceID: mustInstanceID("run")},
		domainexecution.Entry{ID: mustEntryID("entry")},
	)

	if err == nil {
		t.Fatal("Execute() returned nil, want error")
	}
	if !fault.IsCode(err, domainexecution.CodeWorkerFenceInvalid) {
		t.Fatalf("Execute() error = %v, want code %s", err, domainexecution.CodeWorkerFenceInvalid)
	}
	if len(factory.sessions) != 0 {
		t.Fatalf("factory.Create created %d sessions, want 0", len(factory.sessions))
	}
	if authorizer.authorizeCount != 0 {
		t.Fatalf("authorizer.AuthorizeEntry called %d times, want 0", authorizer.authorizeCount)
	}
}
