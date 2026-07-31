package automation

import (
	"context"
	"strings"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
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
			if err := request.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}

	invalid := valid
	invalid.Decision = "UNKNOWN"
	if _, err := HealReviewRequestIdentityDigest(invalid); err == nil || !strings.Contains(err.Error(), "unsupported heal review decision") {
		t.Fatalf("HealReviewRequestIdentityDigest() error = %v", err)
	}
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
			if err := intent.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}

	invalid := cloneHealReviewIntent(approve)
	invalid.ReviewedBy = ""
	if _, err := HealReviewRequestDigest(invalid); err == nil || !strings.Contains(err.Error(), "trusted reviewer metadata") {
		t.Fatalf("HealReviewRequestDigest() error = %v", err)
	}
	if err := ValidateHealReviewIntentDigest(invalid); err == nil || !strings.Contains(err.Error(), "trusted reviewer metadata") {
		t.Fatalf("ValidateHealReviewIntentDigest() error = %v", err)
	}

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
		}, want: "rewrite sampled workflow references"},
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
