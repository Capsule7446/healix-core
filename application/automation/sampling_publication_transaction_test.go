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
			TemporaryElementTargetID: node.TemporaryElementTargetID,
			ElementTargetID:          node.Aggregate.ElementTarget.ID,
			ElementTargetVersionID:   node.Aggregate.Current.ID,
			ResolutionMode:           node.ResolutionMode,
		}
	}
	return PublishSamplingOutcome{
		Status: status, PublicationID: command.PublicationID, RequestDigest: digest,
		Result: domain.SamplingPublicationResult{
			FlowFragmentID: publication.FlowFragment.FlowFragment.ID, WorkflowVersionID: publication.FlowFragment.Current.ID,
			VersionNumber: publication.FlowFragment.Current.VersionNumber, Nodes: mappings,
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
	changed.Publication.FlowFragment.FlowFragment.DisplayName = "changed"
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
			result, err := NewSamplingPublicationService(transaction).Publish(context.Background(), command)
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
		intent.Publication.FlowFragment.FlowFragment.ID = "mutated"
		intent.Publication.FlowFragment.FlowFragment.Properties["mutated"] = "yes"
	}}
	result, err := NewSamplingPublicationService(transaction).Publish(context.Background(), command)
	if err != nil || result.FlowFragmentID != "workflow" || command.Publication.FlowFragment.FlowFragment.ID != "workflow" {
		t.Fatalf("publish = %#v, %v; command=%#v", result, err, command.Publication.FlowFragment.FlowFragment)
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
		{name: "workflow", mutate: func(outcome *PublishSamplingOutcome) { outcome.Result.FlowFragmentID = "other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome := samplingOutcomeFor(t, command, PublishSamplingApplied)
			test.mutate(&outcome)
			_, err := NewSamplingPublicationService(&samplingRepositoryFake{outcome: outcome}).Publish(context.Background(), command)
			if !errors.Is(err, ErrSamplingPublicationContract) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestSamplingPublicationServiceReturnsConfigurationErrorWithoutTransaction(t *testing.T) {
	command := SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)}
	_, err := NewSamplingPublicationService(nil).Publish(context.Background(), command)
	if !errors.Is(err, ErrSamplingPublicationConfiguration) {
		t.Fatalf("error = %v", err)
	}
}

func TestSamplingPublicationServiceReturnsOwnedMappings(t *testing.T) {
	command := createSamplingCommand(t)
	for _, status := range []PublishSamplingStatus{PublishSamplingApplied, PublishSamplingReplayed} {
		t.Run(string(status), func(t *testing.T) {
			outcome := samplingOutcomeFor(t, command, status)
			transaction := &samplingTransactionProbe{publishOutcome: outcome}
			if status == PublishSamplingReplayed {
				transaction.lookupOutcome, transaction.lookupFound = outcome, true
			}
			result, err := NewSamplingPublicationService(transaction).Publish(context.Background(), command)
			if err != nil || len(result.Nodes) != 1 {
				t.Fatalf("publish = %#v, %v", result, err)
			}
			if status == PublishSamplingApplied {
				transaction.publishOutcome.Result.Nodes[0].ElementTargetVersionID = "adapter-mutated"
			} else {
				transaction.lookupOutcome.Result.Nodes[0].ElementTargetVersionID = "adapter-mutated"
			}
			if result.Nodes[0].ElementTargetVersionID != "forced-v1" {
				t.Fatalf("returned mapping aliases adapter state: %#v", result.Nodes)
			}
		})
	}
}

func TestSamplingPublicationServiceRejectsMalformedMappings(t *testing.T) {
	command := createSamplingCommand(t)
	tests := []struct {
		name   string
		mutate func(*PublishSamplingOutcome)
	}{
		{name: "missing", mutate: func(outcome *PublishSamplingOutcome) { outcome.Result.Nodes = nil }},
		{name: "extra", mutate: func(outcome *PublishSamplingOutcome) {
			outcome.Result.Nodes = append(outcome.Result.Nodes, outcome.Result.Nodes[0])
		}},
		{name: "temporary identity", mutate: func(outcome *PublishSamplingOutcome) { outcome.Result.Nodes[0].TemporaryElementTargetID = "other" }},
		{name: "target identity", mutate: func(outcome *PublishSamplingOutcome) { outcome.Result.Nodes[0].ElementTargetID = "other" }},
		{name: "version identity", mutate: func(outcome *PublishSamplingOutcome) { outcome.Result.Nodes[0].ElementTargetVersionID = "other" }},
		{name: "resolution mode", mutate: func(outcome *PublishSamplingOutcome) { outcome.Result.Nodes[0].ResolutionMode = "REUSE" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome := samplingOutcomeFor(t, command, PublishSamplingApplied)
			test.mutate(&outcome)
			_, err := NewSamplingPublicationService(&samplingTransactionProbe{publishOutcome: outcome}).Publish(context.Background(), command)
			if !errors.Is(err, ErrSamplingPublicationContract) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
