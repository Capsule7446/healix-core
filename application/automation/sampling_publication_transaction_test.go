package automation

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

func samplingOutcomeFor(t testing.TB, command SamplingPublicationCommand, status PublishSamplingStatus) PublishSamplingOutcome {
	t.Helper()
	digest, err := SamplingPublicationRequestDigest(command)
	if err != nil {
		t.Fatalf("SamplingPublicationRequestDigest: %v", err)
	}
	publication := command.Publication
	mappings := make([]domain.SamplingNodeMapping, len(publication.Nodes))
	for index, node := range publication.Nodes {
		mappings[index] = domain.SamplingNodeMapping{
			TemporaryNodeID: node.TemporaryNodeID,
			NodeID:          node.Aggregate.Node.ID,
			NodeVersionID:   node.Aggregate.Current.ID,
			ResolutionMode:  node.ResolutionMode,
		}
	}
	return PublishSamplingOutcome{
		Status: status, PublicationID: command.PublicationID, RequestDigest: digest,
		Result: domain.SamplingPublicationResult{
			WorkflowID: publication.Workflow.Workflow.ID, WorkflowVersionID: publication.Workflow.Current.ID,
			VersionNumber: publication.Workflow.Current.VersionNumber, Nodes: mappings,
		},
	}
}

func TestSamplingPublicationRequestDigestIsDeterministicAndBoundarySensitive(t *testing.T) {
	command := SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)}
	first, err := SamplingPublicationRequestDigest(command)
	second, secondErr := SamplingPublicationRequestDigest(command)
	if err != nil || secondErr != nil || first != second || len(first) != 71 || !strings.HasPrefix(first, "sha256:") {
		t.Fatalf("digests = %q %q, %v %v", first, second, err, secondErr)
	}
	changed := command
	changed.Publication.Workflow.Workflow.DisplayName = "changed"
	other, err := SamplingPublicationRequestDigest(changed)
	if err != nil || other == first {
		t.Fatalf("changed digest = %q, %v", other, err)
	}
}

func TestSamplingPublicationServiceAcceptsAppliedAndReplay(t *testing.T) {
	for _, status := range []PublishSamplingStatus{PublishSamplingApplied, PublishSamplingReplayed} {
		t.Run(string(status), func(t *testing.T) {
			command := SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)}
			transaction := &samplingRepositoryFake{outcome: samplingOutcomeFor(t, command, status)}
			result, err := NewSamplingPublicationService(transaction, nil).Publish(context.Background(), command)
			if err != nil || result.WorkflowVersionID != "workflow-v1" {
				t.Fatalf("publish = %#v, %v", result, err)
			}
		})
	}
}

func TestSamplingPublicationServiceKeepsPrivateValidationSnapshot(t *testing.T) {
	command := SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)}
	outcome := samplingOutcomeFor(t, command, PublishSamplingApplied)
	transaction := &samplingRepositoryFake{outcome: outcome, mutate: func(intent *PublishSamplingIntent) {
		intent.Publication.Workflow.Workflow.ID = "mutated"
		intent.Publication.Workflow.Workflow.Properties["mutated"] = "yes"
	}}
	result, err := NewSamplingPublicationService(transaction, nil).Publish(context.Background(), command)
	if err != nil || result.WorkflowID != "workflow" || command.Publication.Workflow.Workflow.ID != "workflow" {
		t.Fatalf("publish = %#v, %v; command=%#v", result, err, command.Publication.Workflow.Workflow)
	}
}

func TestSamplingPublicationServiceRejectsMalformedOutcome(t *testing.T) {
	command := SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)}
	tests := []struct {
		name   string
		mutate func(*PublishSamplingOutcome)
	}{
		{name: "status", mutate: func(outcome *PublishSamplingOutcome) { outcome.Status = "UNKNOWN" }},
		{name: "publication", mutate: func(outcome *PublishSamplingOutcome) { outcome.PublicationID = "other" }},
		{name: "digest", mutate: func(outcome *PublishSamplingOutcome) { outcome.RequestDigest = "sha256:" + strings.Repeat("0", 64) }},
		{name: "workflow", mutate: func(outcome *PublishSamplingOutcome) { outcome.Result.WorkflowID = "other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome := samplingOutcomeFor(t, command, PublishSamplingApplied)
			test.mutate(&outcome)
			_, err := NewSamplingPublicationService(&samplingRepositoryFake{outcome: outcome}, nil).Publish(context.Background(), command)
			if !errors.Is(err, ErrSamplingPublicationContract) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestSamplingPublicationServiceReturnsConfigurationErrorWithoutTransaction(t *testing.T) {
	command := SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)}
	_, err := NewSamplingPublicationService(nil, nil).Publish(context.Background(), command)
	if !errors.Is(err, ErrSamplingPublicationConfiguration) {
		t.Fatalf("error = %v", err)
	}
}

func TestSamplingPublicationServiceReturnsOwnedMappings(t *testing.T) {
	command := SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)}
	outcome := samplingOutcomeFor(t, command, PublishSamplingApplied)
	transaction := &samplingRepositoryFake{outcome: outcome}
	result, err := NewSamplingPublicationService(transaction, nil).Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	transaction.outcome.Result.Nodes = append(transaction.outcome.Result.Nodes, domain.SamplingNodeMapping{TemporaryNodeID: "adapter-owned"})
	if len(result.Nodes) != 0 {
		t.Fatalf("returned mappings changed with adapter state: %#v", result.Nodes)
	}
}

func TestSamplingPublicationServiceReplaysForceCreateBeforeAuthorization(t *testing.T) {
	publication, err := MapSamplingPublication(SamplingPublicationRequest{WorkflowID: "workflow", WorkflowVersionID: "workflow-v1", PublishedAt: 2, Workspace: sampledWorkflow("FORCE_CREATE"), Nodes: []SamplingNodeAuthority{{TemporaryNodeID: "temporary-node", NodeID: "forced", NodeVersionID: "forced-v1", ForceCreateAuthorized: true}}})
	if err != nil {
		t.Fatal(err)
	}
	command := SamplingPublicationCommand{PublicationID: "publication", ForceCreateAuthorization: "consumed", Publication: publication}
	transaction := &samplingRepositoryFake{outcome: samplingOutcomeFor(t, command, PublishSamplingReplayed)}
	authorizer := &forceCreateAuthorizerFake{err: errors.New("authorization already consumed")}

	result, err := NewSamplingPublicationService(transaction, authorizer).Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.WorkflowVersionID != "workflow-v1" || authorizer.called || transaction.called {
		t.Fatalf("replay = %#v, authorized=%v published=%v", result, authorizer.called, transaction.called)
	}
}

func TestSamplingPublicationServiceRequiresVerifiedForceCreateAuthorization(t *testing.T) {
	publication, err := MapSamplingPublication(SamplingPublicationRequest{WorkflowID: "workflow", WorkflowVersionID: "workflow-v1", PublishedAt: 2, Workspace: sampledWorkflow("FORCE_CREATE"), Nodes: []SamplingNodeAuthority{{TemporaryNodeID: "temporary-node", NodeID: "forced", NodeVersionID: "forced-v1", ForceCreateAuthorized: true}}})
	if err != nil {
		t.Fatal(err)
	}
	command := SamplingPublicationCommand{PublicationID: "publication", ForceCreateAuthorization: "authorization", Publication: publication}
	transaction := &samplingRepositoryFake{outcome: samplingOutcomeFor(t, command, PublishSamplingApplied)}
	if _, err := NewSamplingPublicationService(transaction, nil).Publish(context.Background(), command); !errors.Is(err, ErrSamplingPublicationAuthorization) || transaction.called {
		t.Fatalf("missing authorizer = %v, called=%v", err, transaction.called)
	}
	authorizerCause := errors.New("authorization backend unavailable")
	authorizer := &forceCreateAuthorizerFake{err: authorizerCause}
	if _, err := NewSamplingPublicationService(transaction, authorizer).Publish(context.Background(), command); !errors.Is(err, ErrSamplingPublicationAuthorization) || !errors.Is(err, authorizerCause) || transaction.called {
		t.Fatalf("rejected authorization = %v, called=%v", err, transaction.called)
	}
	authorizer.err = nil
	expectedDigest, err := SamplingPublicationRequestDigest(command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSamplingPublicationService(transaction, authorizer).Publish(context.Background(), command); err != nil || !authorizer.called || !transaction.called {
		t.Fatalf("authorized publish = %v, authorized=%v called=%v", err, authorizer.called, transaction.called)
	}
	if authorizer.intent.PublicationID != command.PublicationID || authorizer.intent.RequestDigest != expectedDigest || authorizer.intent.AuthorizationReference != command.ForceCreateAuthorization {
		t.Fatalf("authorization intent = %#v", authorizer.intent)
	}
}
