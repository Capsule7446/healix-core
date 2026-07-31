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
		{name: "zero candidate revision", mutate: func(request *HealReviewRequest) { request.ExpectedCandidateRevision = 0 }, want: "persisted revision"},
		{name: "zero node revision", mutate: func(request *HealReviewRequest) { request.ExpectedNodeRevision = 0 }, want: "persisted revision"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.mutate(&request)
			err := request.Validate()
			// A revision that is already classified passes through rather than being
			// relabelled: it names a different problem than the command's shape.
			if strings.Contains(test.want, "persisted revision") {
				if !fault.IsCode(err, domain.CodePersistedRevisionInvalid) {
					t.Fatalf("Validate() error = %v, want the persisted revision code", err)
				}
				return
			}
			requireHealReviewCommandRejection(t, err, test.want)
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
			// The two zero-revision cases now surface the registered code that
			// ValidatePersisted already produced, instead of an uncoded wrapper that
			// only differed by which revision it named. Which revision the caller
			// supplied is a field-level detail and belongs in a violation once this
			// validator gains an envelope, not in a second unclassified error.
			if strings.Contains(test.want, "revision") {
				if !fault.IsCode(err, domain.CodePersistedRevisionInvalid) {
					t.Fatalf("Validate() error = %v, want the persisted revision code", err)
				}
				return
			}
			requireHealReviewCommandRejection(t, err, test.want)
		})
	}

	invalid := cloneHealReviewIntent(approve)
	invalid.ReviewedBy = ""
	_, digestErr := HealReviewRequestDigest(invalid)
	requireHealReviewCommandRejection(t, digestErr, "trusted reviewer metadata")
	requireHealReviewCommandRejection(t, ValidateHealReviewIntentDigest(invalid), "trusted reviewer metadata")

	if got := (HealReviewIntent{}).NextNodeValue(); got.ElementTarget.ID != "" || got.Current.ID != "" {
		t.Fatalf("nil NextNode value = %#v", got)
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
		{name: "unmapped step reference", mutate: func(request *SamplingPublicationRequest) {
			request.Workspace.Steps[0].Children[0].ElementTargetID = "unknown-temporary-node"
		}, want: string(sampling.CodePublicationMappingInvalid)},
		{name: "invalid workflow", mutate: func(request *SamplingPublicationRequest) { request.Workspace.DisplayName = "" }, want: "build sampled workflow"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := newRequest()
			test.mutate(&request)
			if _, err := MapSamplingPublication(request); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("MapSamplingPublication() error = %v, want %q", err, test.want)
			}
		})
	}
}
