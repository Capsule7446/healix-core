package automation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

const samplingPublicationDigestV1 = "sampling-publication-v1"

var (
	ErrSamplingPublicationIdentityConflict  = errors.New("sampling publication identity conflict")
	ErrSamplingPublicationAuthorityConflict = errors.New("sampling publication authority conflict")
	ErrSamplingPublicationDigestMismatch    = errors.New("sampling publication digest does not match payload")
	ErrSamplingPublicationAuthorization     = errors.New("sampling publication authorization rejected")
	ErrSamplingPublicationConfiguration     = errors.New("sampling publication service is not configured")
	ErrSamplingPublicationContract          = errors.New("sampling publication adapter contract violation")
)

type SamplingPublicationIdentityConflictError struct{ PublicationID string }

func (e *SamplingPublicationIdentityConflictError) Error() string {
	return "sampling publication identity conflict: " + e.PublicationID
}

func (e *SamplingPublicationIdentityConflictError) Is(target error) bool {
	return target == ErrSamplingPublicationIdentityConflict
}

type SamplingPublicationContractError struct{ Cause error }

func (e *SamplingPublicationContractError) Error() string {
	return "sampling publication adapter contract violation: " + e.Cause.Error()
}

func (e *SamplingPublicationContractError) Unwrap() error { return e.Cause }
func (e *SamplingPublicationContractError) Is(target error) bool {
	return target == ErrSamplingPublicationContract
}

type SamplingPublicationCommand struct {
	PublicationID            string
	ForceCreateAuthorization string
	Publication              domain.SamplingPublication
}

type PublishSamplingIntent struct {
	PublicationID            string
	RequestDigest            string
	ForceCreateAuthorization string
	Publication              domain.SamplingPublication
}

type PublishSamplingStatus string

const (
	PublishSamplingApplied  PublishSamplingStatus = "APPLIED"
	PublishSamplingReplayed PublishSamplingStatus = "REPLAYED"
)

type PublishSamplingOutcome struct {
	Status        PublishSamplingStatus
	PublicationID string
	RequestDigest string
	Result        domain.SamplingPublicationResult
}

type SamplingPublicationTransaction interface {
	LookupSamplingPublication(context.Context, string, string) (PublishSamplingOutcome, bool, error)
	PublishSampling(context.Context, PublishSamplingIntent) (PublishSamplingOutcome, error)
}

type ForceCreateAuthorizationIntent struct {
	PublicationID          string
	RequestDigest          string
	AuthorizationReference string
}

type ForceCreateAuthorizer interface {
	AuthorizeForceCreate(context.Context, ForceCreateAuthorizationIntent) error
}

type SamplingPublicationService struct {
	transaction SamplingPublicationTransaction
	authorizer  ForceCreateAuthorizer
}

func NewSamplingPublicationService(transaction SamplingPublicationTransaction, authorizer ForceCreateAuthorizer) SamplingPublicationService {
	return SamplingPublicationService{transaction: transaction, authorizer: authorizer}
}

func ValidatePublishSamplingIntentDigest(intent PublishSamplingIntent) error {
	digest, err := SamplingPublicationRequestDigest(SamplingPublicationCommand{
		PublicationID:            intent.PublicationID,
		ForceCreateAuthorization: intent.ForceCreateAuthorization,
		Publication:              intent.Publication,
	})
	if err != nil {
		return fmt.Errorf("validate sampling publication intent: %w", err)
	}
	if intent.RequestDigest != digest {
		return ErrSamplingPublicationDigestMismatch
	}
	return nil
}

func SamplingPublicationRequestDigest(command SamplingPublicationCommand) (string, error) {
	owned := command
	owned.Publication = command.Publication.Clone()
	if err := validateSamplingPublicationCommand(owned); err != nil {
		return "", err
	}
	payload, err := json.Marshal(struct {
		Schema                   string
		PublicationID            string
		ForceCreateAuthorization string
		Publication              domain.SamplingPublication
	}{samplingPublicationDigestV1, owned.PublicationID, owned.ForceCreateAuthorization, owned.Publication})
	if err != nil {
		return "", fmt.Errorf("encode sampling publication request: %w", err)
	}
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (s SamplingPublicationService) Publish(ctx context.Context, command SamplingPublicationCommand) (domain.SamplingPublicationResult, error) {
	if isNilDependency(s.transaction) {
		return domain.SamplingPublicationResult{}, ErrSamplingPublicationConfiguration
	}
	owned := command
	owned.Publication = command.Publication.Clone()
	digest, err := SamplingPublicationRequestDigest(owned)
	if err != nil {
		return domain.SamplingPublicationResult{}, err
	}
	if s.transaction == nil {
		return domain.SamplingPublicationResult{}, ErrSamplingPublicationConfiguration
	}
	replay, found, err := s.transaction.LookupSamplingPublication(ctx, owned.PublicationID, digest)
	if err != nil {
		return domain.SamplingPublicationResult{}, fmt.Errorf("lookup sampling publication: %w", err)
	}
	if found {
		if err := validatePublishSamplingOutcome(owned, digest, replay); err != nil {
			return domain.SamplingPublicationResult{}, &SamplingPublicationContractError{Cause: err}
		}
		return cloneSamplingPublicationResult(replay.Result), nil
	}
	if requiresForceCreateAuthorization(owned.Publication) {
		if s.authorizer == nil {
			return domain.SamplingPublicationResult{}, ErrSamplingPublicationAuthorization
		}
		if err := s.authorizer.AuthorizeForceCreate(ctx, ForceCreateAuthorizationIntent{
			PublicationID:          owned.PublicationID,
			RequestDigest:          digest,
			AuthorizationReference: owned.ForceCreateAuthorization,
		}); err != nil {
			return domain.SamplingPublicationResult{}, fmt.Errorf("%w: %w", ErrSamplingPublicationAuthorization, err)
		}
	}
	if s.transaction == nil {
		return domain.SamplingPublicationResult{}, ErrSamplingPublicationConfiguration
	}
	transactionPublication := owned.Publication.Clone()
	outcome, err := s.transaction.PublishSampling(ctx, PublishSamplingIntent{
		PublicationID:            owned.PublicationID,
		RequestDigest:            digest,
		ForceCreateAuthorization: owned.ForceCreateAuthorization,
		Publication:              transactionPublication,
	})
	if err != nil {
		return domain.SamplingPublicationResult{}, fmt.Errorf("publish sampling result: %w", err)
	}
	if err := validatePublishSamplingOutcome(owned, digest, outcome); err != nil {
		return domain.SamplingPublicationResult{}, &SamplingPublicationContractError{Cause: err}
	}
	return cloneSamplingPublicationResult(outcome.Result), nil
}

func cloneSamplingPublicationResult(result domain.SamplingPublicationResult) domain.SamplingPublicationResult {
	result.Nodes = append([]domain.SamplingNodeMapping(nil), result.Nodes...)
	return result
}

func requiresForceCreateAuthorization(publication domain.SamplingPublication) bool {
	for _, node := range publication.Nodes {
		if node.ResolutionMode == "FORCE_CREATE" {
			return true
		}
	}
	return false
}

func validateSamplingPublicationCommand(command SamplingPublicationCommand) error {
	if strings.TrimSpace(command.PublicationID) == "" {
		return errors.New("sampling publication id is required")
	}
	if err := command.Publication.Validate(); err != nil {
		return fmt.Errorf("validate sampling publication: %w", err)
	}
	hasForceCreate := false
	for _, node := range command.Publication.Nodes {
		hasForceCreate = hasForceCreate || node.ResolutionMode == "FORCE_CREATE"
	}
	if hasForceCreate && strings.TrimSpace(command.ForceCreateAuthorization) == "" {
		return errors.New("force-create authorization is required")
	}
	if !hasForceCreate && command.ForceCreateAuthorization != "" {
		return errors.New("force-create authorization is not applicable")
	}
	return nil
}

func validatePublishSamplingOutcome(command SamplingPublicationCommand, digest string, outcome PublishSamplingOutcome) error {
	if outcome.Status != PublishSamplingApplied && outcome.Status != PublishSamplingReplayed {
		return fmt.Errorf("unsupported status %q", outcome.Status)
	}
	if outcome.PublicationID != command.PublicationID || outcome.RequestDigest != digest {
		return errors.New("outcome identity does not match request")
	}
	workflow := command.Publication.Workflow
	if outcome.Result.WorkflowID != workflow.Workflow.ID || outcome.Result.WorkflowVersionID != workflow.Current.ID || outcome.Result.VersionNumber != workflow.Current.VersionNumber {
		return errors.New("outcome workflow does not match publication")
	}
	if len(outcome.Result.Nodes) != len(command.Publication.Nodes) {
		return errors.New("outcome mappings do not exactly match publication")
	}
	for index, node := range command.Publication.Nodes {
		mapping := outcome.Result.Nodes[index]
		if mapping.TemporaryNodeID != node.TemporaryNodeID || mapping.NodeID != node.Aggregate.Node.ID || mapping.NodeVersionID != node.Aggregate.Current.ID || mapping.ResolutionMode != node.ResolutionMode {
			return errors.New("outcome mapping does not match publication")
		}
	}
	return nil
}
