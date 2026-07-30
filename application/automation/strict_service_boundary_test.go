package automation

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

func TestAggregateServicesRejectMissingRepositoriesWithoutPanicking(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{"environment", func() error {
			_, err := NewEnvironmentService(nil).Delete(context.Background(), "environment", 1, 1)
			return err
		}},
		{"node", func() error { _, err := NewNodeService(nil).Delete(context.Background(), "node", 1, 1); return err }},
		{"workflow", func() error {
			_, err := NewFlowFragmentService(nil).Delete(context.Background(), "workflow", 1, 1)
			return err
		}},
		{"test task", func() error {
			_, err := NewExecutionFlowService(nil).PublishVersion(context.Background(), "task", 1, domain.ExecutionFlowVersionPublication{ID: "v2"})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("panicked: %v", recovered)
				}
			}()
			if err := test.call(); !errors.Is(err, ErrAutomationConfiguration) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

type typedNilSamplingTransaction struct{}

func (*typedNilSamplingTransaction) LookupSamplingPublication(context.Context, string, string) (PublishSamplingOutcome, bool, error) {
	return PublishSamplingOutcome{}, false, nil
}
func (*typedNilSamplingTransaction) PublishSampling(context.Context, PublishSamplingIntent) (PublishSamplingOutcome, error) {
	return PublishSamplingOutcome{}, nil
}

func TestConstructorsAndMethodsRejectTypedNilDependencies(t *testing.T) {
	var environment *environmentRepositoryFake
	var folder *folderRepositoryFake
	var sampling *typedNilSamplingTransaction

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"environment", func() error {
			_, err := NewEnvironmentService(environment).Delete(context.Background(), "id", 1, 1)
			return err
		}, ErrAutomationConfiguration},
		{"folder", func() error {
			_, err := NewFolderService(folder).Delete(context.Background(), domain.FolderNode, "id", 1)
			return err
		}, ErrAutomationConfiguration},
		{"sampling", func() error {
			_, err := NewSamplingPublicationService(sampling, nil).Publish(context.Background(), SamplingPublicationCommand{})
			return err
		}, ErrSamplingPublicationConfiguration},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("panicked: %v", recovered)
				}
			}()
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestFolderMethodsRejectBlankIdentityBeforeRepositoryAccess(t *testing.T) {
	repositoryFailure := errors.New("repository must not be called")
	repository := &folderRepositoryFake{loadErr: repositoryFailure}
	service := NewFolderService(repository)
	folder := domain.Folder{Kind: domain.FolderNode}
	calls := []func() error{
		func() error { _, err := service.Create(context.Background(), folder, 1); return err },
		func() error {
			_, err := service.Move(context.Background(), domain.FolderNode, " \t", "", 1, 1)
			return err
		},
		func() error { _, err := service.Delete(context.Background(), domain.FolderNode, "", 1); return err },
	}
	for index, call := range calls {
		if err := call(); err == nil || errors.Is(err, repositoryFailure) {
			t.Fatalf("call %d error = %v", index, err)
		}
	}
}

func TestHealReviewRequestValidateDirectBoundaries(t *testing.T) {
	valid := HealReviewRequest{CommandID: "command", Decision: HealReviewApprove, ElementTargetID: "node", BaseNodeVersionID: "version", CandidateHash: "hash", ExpectedCandidateRevision: 1, ExpectedNodeRevision: 1}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*HealReviewRequest)
	}{
		{"blank command", func(value *HealReviewRequest) { value.CommandID = " \t" }},
		{"blank node", func(value *HealReviewRequest) { value.ElementTargetID = "" }},
		{"blank base", func(value *HealReviewRequest) { value.BaseNodeVersionID = " " }},
		{"blank hash", func(value *HealReviewRequest) { value.CandidateHash = "" }},
		{"invalid decision", func(value *HealReviewRequest) { value.Decision = "UNKNOWN" }},
		{"candidate revision zero", func(value *HealReviewRequest) { value.ExpectedCandidateRevision = 0 }},
		{"node revision zero", func(value *HealReviewRequest) { value.ExpectedNodeRevision = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.mutate(&request)
			if err := request.Validate(); err == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
}

func TestHealReviewIntentNextNodeValueReturnsOwnedValue(t *testing.T) {
	if got := (HealReviewIntent{}).NextNodeValue(); got.ElementTarget.ID != "" {
		t.Fatalf("nil node value = %#v", got)
	}
	node := domain.ElementTargetAggregate{ElementTarget: domain.ElementTarget{ID: "node"}, Current: domain.ElementTargetVersion{ID: "version", Selectors: nil}}
	intent := HealReviewIntent{NextNode: &node}
	got := intent.NextNodeValue()
	got.ElementTarget.ID = "changed"
	if intent.NextNode.ElementTarget.ID != "node" {
		t.Fatal("returned node aliases intent")
	}
}

func TestAggregateTransitionsRejectBlankIDsBeforeRepositoryAccess(t *testing.T) {
	repositoryFailure := errors.New("repository must not be called")
	tests := []struct {
		name string
		call func() error
	}{
		{"environment empty", func() error {
			_, err := NewEnvironmentService(&environmentRepositoryFake{loadErr: repositoryFailure}).Delete(context.Background(), "", 1, 1)
			return err
		}},
		{"environment whitespace", func() error {
			_, err := NewEnvironmentService(&environmentRepositoryFake{loadErr: repositoryFailure}).Delete(context.Background(), "  ", 1, 1)
			return err
		}},
		{"node empty", func() error {
			_, err := NewNodeService(&nodeRepositoryFake{loadErr: repositoryFailure}).Delete(context.Background(), "", 1, 1)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if err == nil || errors.Is(err, repositoryFailure) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
