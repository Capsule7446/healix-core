package automation

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/sampling"
)

func validCapturedHealReviewIntents(t *testing.T) (HealReviewIntent, HealReviewIntent) {
	t.Helper()
	source, nodes, approveTransaction, command := healReviewFixture(t)
	if _, err := newReviewService(t, source, nodes, approveTransaction).Approve(context.Background(), command); err != nil {
		t.Fatalf("capture approval intent: %v", err)
	}

	source, nodes, rejectTransaction, command := healReviewFixture(t)
	if err := newReviewService(t, source, nodes, rejectTransaction).Reject(context.Background(), command); err != nil {
		t.Fatalf("capture rejection intent: %v", err)
	}
	return cloneHealReviewIntent(approveTransaction.intent), cloneHealReviewIntent(rejectTransaction.intent)
}

// requireHealReviewCommandRejection asserts the split the contract asks for: the
// host switches on the code, the detail survives only as a private cause, and
// nothing of that detail reaches public text. The detail used to be the public
// message, so asserting its absence there is the point.
func requireHealReviewCommandRejection(t *testing.T, err error, wantDetail string) {
	t.Helper()
	if err == nil {
		t.Fatal("a malformed heal review command was accepted")
	}
	if !fault.IsCode(err, domain.CodeHealCandidateReviewCommandInvalid) {
		t.Fatalf("error = %v, want code %s", err, domain.CodeHealCandidateReviewCommandInvalid)
	}
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Kind() != fault.InvalidArgument {
		t.Fatalf("descriptor = %#v (ok=%v); a caller-fixable command must not be INTERNAL", descriptor, ok)
	}
	if strings.Contains(descriptor.Message(), wantDetail) {
		t.Fatalf("public message %q carries the detail %q", descriptor.Message(), wantDetail)
	}
	cause := errors.Unwrap(err)
	if cause == nil || !strings.Contains(cause.Error(), wantDetail) {
		t.Fatalf("private cause = %v, want it to retain %q", cause, wantDetail)
	}
}

// requireHealReviewIntentRejection is the intent-side counterpart: an intent is
// built by the review service or replayed from persistence, so its invariant
// failures are INTERNAL contract violations, never caller-fixable arguments.
func requireHealReviewIntentRejection(t *testing.T, err error, wantDetail string) {
	t.Helper()
	if err == nil {
		t.Fatal("a malformed heal review intent was accepted")
	}
	if !fault.IsCode(err, CodeHealReviewContractViolation) {
		t.Fatalf("error = %v, want code %s", err, CodeHealReviewContractViolation)
	}
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Kind() != fault.Internal {
		t.Fatalf("descriptor = %#v (ok=%v); a Core-built artifact's invariant failure is not caller-fixable", descriptor, ok)
	}
	if strings.Contains(descriptor.Message(), wantDetail) {
		t.Fatalf("public message %q carries the detail %q", descriptor.Message(), wantDetail)
	}
	if cause := errors.Unwrap(err); cause == nil || !strings.Contains(cause.Error(), wantDetail) {
		t.Fatalf("private cause = %v, want it to retain %q", cause, wantDetail)
	}
}

func TestHealReviewRequestRejectsEachPublicIdentityBoundary(t *testing.T) {
	valid := HealReviewRequest{
		CommandID: "command", Decision: HealReviewApprove, ElementTargetID: "node", BaseNodeVersionID: "node-v1",
		CandidateHash: "candidate", ExpectedCandidateRevision: 1, ExpectedNodeRevision: 1,
	}
	tests := []struct {
		name   string
		mutate func(*HealReviewRequest)
		want   string
	}{
		{name: "missing identity", mutate: func(request *HealReviewRequest) { request.CommandID = " \t" }, want: "requires command"},
		{name: "unsupported decision", mutate: func(request *HealReviewRequest) { request.Decision = "UNKNOWN" }, want: "unsupported heal review decision"},
		// A zero expected revision in a request is caller-fixable — the caller reads
		// the authoritative revision and supplies it — so it is the command code
		// (INVALID_ARGUMENT), not the persisted-state code (FAILED_PRECONDITION).
		{name: "zero candidate revision", mutate: func(request *HealReviewRequest) { request.ExpectedCandidateRevision = 0 }, want: "expected candidate revision"},
		{name: "zero node revision", mutate: func(request *HealReviewRequest) { request.ExpectedNodeRevision = 0 }, want: "expected node revision"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.mutate(&request)
			requireHealReviewCommandRejection(t, request.Validate(), test.want)
		})
	}

	invalid := valid
	invalid.Decision = "UNKNOWN"
	_, digestErr := HealReviewRequestIdentityDigest(invalid)
	requireHealReviewCommandRejection(t, digestErr, "unsupported heal review decision")
}

func TestHealReviewIntentRejectsEachTransitionInvariant(t *testing.T) {
	approve, reject := validCapturedHealReviewIntents(t)
	tests := []struct {
		name   string
		base   HealReviewIntent
		mutate func(*HealReviewIntent)
		want   string
	}{
		{name: "missing identity", base: approve, mutate: func(intent *HealReviewIntent) { intent.ElementTargetID = " \n" }, want: "requires command"},
		{name: "missing reviewer metadata", base: approve, mutate: func(intent *HealReviewIntent) { intent.ReviewedBy = "" }, want: "trusted reviewer metadata"},
		{name: "zero candidate revision", base: approve, mutate: func(intent *HealReviewIntent) { intent.ExpectedCandidateRevision = 0 }, want: "expected candidate revision"},
		{name: "zero node revision", base: approve, mutate: func(intent *HealReviewIntent) { intent.ExpectedNodeRevision = 0 }, want: "expected node revision"},
		{name: "candidate authority drift", base: approve, mutate: func(intent *HealReviewIntent) { intent.NextCandidate.Hash = "other" }, want: "candidate transition"},
		{name: "approval shape drift", base: approve, mutate: func(intent *HealReviewIntent) { intent.NextCandidate.Status = domain.HealCandidateAwaitingApproval }, want: "approval requires"},
		{name: "approval node drift", base: approve, mutate: func(intent *HealReviewIntent) { intent.NextNode.ElementTarget.ID = "other" }, want: "approval node transition"},
		{name: "rejection shape drift", base: reject, mutate: func(intent *HealReviewIntent) { intent.NextStreak = nil }, want: "rejection requires"},
		{name: "rejection authority drift", base: reject, mutate: func(intent *HealReviewIntent) { intent.ExpectedStreakDigest = "" }, want: "rejection streak transition"},
		{name: "unsupported decision", base: approve, mutate: func(intent *HealReviewIntent) { intent.Decision = "UNKNOWN" }, want: "unsupported heal review decision"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := cloneHealReviewIntent(test.base)
			test.mutate(&intent)
			err := intent.Validate()
			if err == nil {
				t.Fatal("Validate() accepted an invalid intent")
			}
			// Intent zero-revision failures still surface the persisted-state code:
			// by the time an intent exists, its revisions describe a captured
			// transaction artifact, not an argument the command caller supplied.
			if strings.Contains(test.want, "revision") {
				if !fault.IsCode(err, domain.CodePersistedRevisionInvalid) {
					t.Fatalf("Validate() error = %v, want the persisted revision code", err)
				}
				return
			}
			requireHealReviewIntentRejection(t, err, test.want)
		})
	}

	invalid := cloneHealReviewIntent(approve)
	invalid.ReviewedBy = ""
	_, digestErr := HealReviewRequestDigest(invalid)
	requireHealReviewIntentRejection(t, digestErr, "trusted reviewer metadata")
	requireHealReviewIntentRejection(t, ValidateHealReviewIntentDigest(invalid), "trusted reviewer metadata")

	if got := (HealReviewIntent{}).NextNodeValue(); got.ElementTarget.ID != "" || got.Current.ID != "" {
		t.Fatalf("nil NextNode value = %#v", got)
	}
}

// requireSamplingPublicationAuthorityRejection asserts the boundary split for
// MapSamplingPublication's own uncoded checks: the host switches on
// CodeSamplingPublicationAuthorityInvalid, and the identity-bearing detail
// (temporary/formal element target ids) survives only on the private cause.
func requireSamplingPublicationAuthorityRejection(t *testing.T, err error, wantDetail string) {
	t.Helper()
	if err == nil {
		t.Fatal("an invalid sampling publication authority was accepted")
	}
	if !fault.IsCode(err, CodeSamplingPublicationAuthorityInvalid) {
		t.Fatalf("error = %v, want code %s", err, CodeSamplingPublicationAuthorityInvalid)
	}
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Kind() != fault.InvalidArgument {
		t.Fatalf("descriptor = %#v (ok=%v)", descriptor, ok)
	}
	if strings.Contains(descriptor.Message(), wantDetail) {
		t.Fatalf("public message %q carries the detail %q", descriptor.Message(), wantDetail)
	}
	if cause := errors.Unwrap(err); cause == nil || !strings.Contains(cause.Error(), wantDetail) {
		t.Fatalf("private cause = %v, want it to retain %q", cause, wantDetail)
	}
}

func TestMapSamplingPublicationRejectsEachRequestAndCompositionBoundary(t *testing.T) {
	newRequest := func() SamplingPublicationRequest {
		return SamplingPublicationRequest{
			FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1", PublishedAt: 2,
			Workspace: sampledWorkflow(sampling.ResolutionModeCreate),
			Nodes:     []SamplingNodeAuthority{{TemporaryElementTargetID: "temporary-node", ElementTargetID: "node", ElementTargetVersionID: "node-v1"}},
		}
	}
	tests := []struct {
		name   string
		mutate func(*SamplingPublicationRequest)
		want   string
	}{
		{name: "missing request identity", mutate: func(request *SamplingPublicationRequest) { request.FlowFragmentID = "" }, want: "requires workflow identity"},
		{name: "incomplete authority", mutate: func(request *SamplingPublicationRequest) { request.Nodes[0].ElementTargetID = "" }, want: "requires temporary and formal identity"},
		{name: "duplicate temporary authority", mutate: func(request *SamplingPublicationRequest) {
			request.Nodes = append(request.Nodes, SamplingNodeAuthority{TemporaryElementTargetID: "temporary-node", ElementTargetID: "other", ElementTargetVersionID: "other-v1"})
		}, want: "duplicate sampling node authority"},
		{name: "duplicate formal version", mutate: func(request *SamplingPublicationRequest) {
			request.Nodes = append(request.Nodes, SamplingNodeAuthority{TemporaryElementTargetID: "other", ElementTargetID: "other", ElementTargetVersionID: "node-v1"})
		}, want: "duplicate formal sampling node version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := newRequest()
			test.mutate(&request)
			_, err := MapSamplingPublication(request)
			requireSamplingPublicationAuthorityRejection(t, err, test.want)
		})
	}

	t.Run("unmapped step reference", func(t *testing.T) {
		request := newRequest()
		request.Workspace.Steps[0].Children[0].ElementTargetID = "unknown-temporary-node"
		// RewriteUnpublishedElementTargetReferences already returns its own coded
		// fault; MapSamplingPublication's boundary classifier must pass it through
		// unchanged rather than burying it under CodeSamplingPublicationAuthorityInvalid.
		_, err := MapSamplingPublication(request)
		if err == nil || !fault.IsCode(err, sampling.CodePublicationMappingInvalid) {
			t.Fatalf("MapSamplingPublication() error = %v, want code %s", err, sampling.CodePublicationMappingInvalid)
		}
	})

	// domainautomation.NewFlowFragment's own errors are classified at its own
	// package boundary by a parallel migration, so this only asserts that SOME
	// fault code crosses MapSamplingPublication — never a specific one, since
	// which code that is depends on work outside this package.
	t.Run("invalid workflow", func(t *testing.T) {
		request := newRequest()
		request.Workspace.DisplayName = ""
		_, err := MapSamplingPublication(request)
		if err == nil {
			t.Fatal("an invalid sampled workflow was accepted")
		}
		if _, ok := fault.CodeOf(err); !ok {
			t.Fatalf("MapSamplingPublication() error = %v, want it to carry some fault code", err)
		}
	})
}
