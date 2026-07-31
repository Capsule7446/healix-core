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
	"github.com/Capsule7446/healix-core/domain/fault"
)

const samplingPublicationDigestV1 = "sampling-publication-v1"

var ErrSamplingPublicationAuthorityConflict = errors.New("sampling publication authority conflict")

const (
	CodeSamplingPublicationIdentityConflict  fault.Code = "SAMPLING_PUBLICATION_IDENTITY_CONFLICT"
	CodeSamplingPublicationDigestMismatch    fault.Code = "AUTOMATION_SAMPLING_PUBLICATION_DIGEST_MISMATCH"
	CodeSamplingPublicationUnavailable       fault.Code = "AUTOMATION_SAMPLING_PUBLICATION_UNAVAILABLE"
	CodeSamplingPublicationContractViolation fault.Code = "AUTOMATION_SAMPLING_PUBLICATION_ADAPTER_CONTRACT_VIOLATION"
)

func SamplingPublicationIdentityConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeSamplingPublicationIdentityConflict,
		"sampling publication identity conflicts with an existing request",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func SamplingPublicationDigestMismatchError() error {
	err, constructionErr := fault.New(
		fault.InvalidArgument,
		CodeSamplingPublicationDigestMismatch,
		"sampling publication digest does not match the request payload",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func SamplingPublicationUnavailableError() error {
	err, constructionErr := fault.New(
		fault.Unavailable,
		CodeSamplingPublicationUnavailable,
		"sampling publication service is unavailable",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func samplingPublicationContractViolationError(cause error) error {
	err, constructionErr := fault.Wrap(
		cause,
		fault.Internal,
		CodeSamplingPublicationContractViolation,
		"sampling publication adapter returned an invalid outcome",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

type SamplingPublicationCommand struct {
	PublicationID string
	Publication   domain.SamplingPublication
}

type PublishSamplingIntent struct {
	PublicationID string
	RequestDigest string
	Publication   domain.SamplingPublication
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

type SamplingPublicationService struct {
	transaction SamplingPublicationTransaction
}

func NewSamplingPublicationService(transaction SamplingPublicationTransaction) SamplingPublicationService {
	return SamplingPublicationService{transaction: transaction}
}

func ValidatePublishSamplingIntentDigest(intent PublishSamplingIntent) error {
	digest, err := SamplingPublicationRequestDigest(SamplingPublicationCommand{
		PublicationID: intent.PublicationID,
		Publication:   intent.Publication,
	})
	if err != nil {
		return fmt.Errorf("validate sampling publication intent: %w", err)
	}
	if intent.RequestDigest != digest {
		return SamplingPublicationDigestMismatchError()
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
		Schema        string
		PublicationID string
		Publication   domain.SamplingPublication
	}{samplingPublicationDigestV1, owned.PublicationID, owned.Publication})
	if err != nil {
		return "", fmt.Errorf("encode sampling publication request: %w", err)
	}
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (s SamplingPublicationService) Publish(ctx context.Context, command SamplingPublicationCommand) (domain.SamplingPublicationResult, error) {
	if isNilDependency(s.transaction) {
		return domain.SamplingPublicationResult{}, SamplingPublicationUnavailableError()
	}
	owned := command
	owned.Publication = command.Publication.Clone()
	digest, err := SamplingPublicationRequestDigest(owned)
	if err != nil {
		return domain.SamplingPublicationResult{}, err
	}
	if s.transaction == nil {
		return domain.SamplingPublicationResult{}, SamplingPublicationUnavailableError()
	}
	replay, found, err := s.transaction.LookupSamplingPublication(ctx, owned.PublicationID, digest)
	if err != nil {
		return domain.SamplingPublicationResult{}, fmt.Errorf("lookup sampling publication: %w", err)
	}
	if found {
		if err := validatePublishSamplingOutcome(owned, digest, replay); err != nil {
			return domain.SamplingPublicationResult{}, samplingPublicationContractViolationError(err)
		}
		return cloneSamplingPublicationResult(replay.Result), nil
	}
	if s.transaction == nil {
		return domain.SamplingPublicationResult{}, SamplingPublicationUnavailableError()
	}
	transactionPublication := owned.Publication.Clone()
	outcome, err := s.transaction.PublishSampling(ctx, PublishSamplingIntent{
		PublicationID: owned.PublicationID,
		RequestDigest: digest,
		Publication:   transactionPublication,
	})
	if err != nil {
		return domain.SamplingPublicationResult{}, fmt.Errorf("publish sampling result: %w", err)
	}
	if err := validatePublishSamplingOutcome(owned, digest, outcome); err != nil {
		return domain.SamplingPublicationResult{}, samplingPublicationContractViolationError(err)
	}
	return cloneSamplingPublicationResult(outcome.Result), nil
}

func cloneSamplingPublicationResult(result domain.SamplingPublicationResult) domain.SamplingPublicationResult {
	result.Nodes = append([]domain.SamplingNodeMapping(nil), result.Nodes...)
	return result
}

func validateSamplingPublicationCommand(command SamplingPublicationCommand) error {
	if strings.TrimSpace(command.PublicationID) == "" {
		return errors.New("sampling publication id is required")
	}
	if err := command.Publication.Validate(); err != nil {
		return fmt.Errorf("validate sampling publication: %w", err)
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
	workflow := command.Publication.FlowFragment
	if outcome.Result.FlowFragmentID != workflow.FlowFragment.ID || outcome.Result.WorkflowVersionID != workflow.Current.ID || outcome.Result.VersionNumber != workflow.Current.VersionNumber {
		return errors.New("outcome workflow does not match publication")
	}
	if len(outcome.Result.Nodes) != len(command.Publication.Nodes) {
		return errors.New("outcome mappings do not exactly match publication")
	}
	for index, node := range command.Publication.Nodes {
		mapping := outcome.Result.Nodes[index]
		if mapping.TemporaryElementTargetID != node.TemporaryElementTargetID || mapping.ElementTargetID != node.Aggregate.ElementTarget.ID || mapping.ElementTargetVersionID != node.Aggregate.Current.ID || mapping.ResolutionMode != node.ResolutionMode {
			return errors.New("outcome mapping does not match publication")
		}
	}
	return nil
}
