package automation

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestHealCandidateReviewTransitions(t *testing.T) {
	candidate := HealCandidate{
		Hash:              "candidate",
		NodeID:            "node",
		BaseNodeVersionID: "node-v1",
		Status:            HealCandidateAwaitingApproval,
		Selectors:         []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "button"}},
		Fingerprint:       fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"role": "submit"}},
		Revision:          1,
	}

	for _, status := range []HealCandidateStatus{HealCandidatePromoted, HealCandidateRejected} {
		t.Run(string(status), func(t *testing.T) {
			reviewed, err := candidate.Review(status)
			if err != nil {
				t.Fatalf("Review() error = %v", err)
			}
			if reviewed.Status != status || reviewed.Revision != 2 {
				t.Fatalf("Review() = %#v", reviewed)
			}
			if err := reviewed.ValidateReviewed(); err != nil {
				t.Fatalf("ValidateReviewed() error = %v", err)
			}
			if candidate.Status != HealCandidateAwaitingApproval || candidate.Revision != 1 {
				t.Fatalf("Review() mutated receiver: %#v", candidate)
			}
		})
	}
}

func TestHealCandidateRejectsInvalidIdentityAndTransitions(t *testing.T) {
	valid := HealCandidate{Hash: "candidate", NodeID: "node", BaseNodeVersionID: "node-v1", Status: HealCandidateAwaitingApproval, Revision: 1}
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{name: "missing identity", run: func() error { return (HealCandidate{}).Validate() }, want: "requires identity"},
		{name: "unpersisted revision", run: func() error { candidate := valid; candidate.Revision = 0; return candidate.Validate() }, want: "persisted revision must be non-zero"},
		{name: "not awaiting approval", run: func() error {
			candidate := valid
			candidate.Status = HealCandidatePromoted
			return candidate.Validate()
		}, want: "not awaiting approval"},
		{name: "not reviewed", run: func() error { return valid.ValidateReviewed() }, want: "not reviewed"},
		{name: "unsupported review status", run: func() error { _, err := valid.Review(HealCandidateStale); return err }, want: "unsupported reviewed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func samplingCreatePublication() SamplingPublication {
	node := versionedNodeAggregate()
	node.Node.CurrentVersionID = "node-v1"
	node.Current = versionedNodeVersion("node-v1", "node", 1, 0)
	node.Versions = []NodeVersion{node.Current}
	return SamplingPublication{
		Workflow: versionedWorkflowAggregate(),
		Nodes: []SamplingNodePublication{{
			TemporaryNodeID: "temporary",
			ResolutionMode:  "CREATE",
			Aggregate:       node,
			PublishVersion:  true,
		}},
	}
}

func TestSamplingPublicationValidation(t *testing.T) {
	valid := samplingCreatePublication()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid publication rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*SamplingPublication)
		want   string
	}{
		{name: "missing temporary id", mutate: func(p *SamplingPublication) { p.Nodes[0].TemporaryNodeID = " " }, want: "temporary id is required"},
		{name: "unsupported resolution", mutate: func(p *SamplingPublication) { p.Nodes[0].ResolutionMode = "COPY" }, want: "unsupported resolution mode"},
		{name: "duplicate temporary id", mutate: func(p *SamplingPublication) { p.Nodes = append(p.Nodes, p.Nodes[0]) }, want: "duplicate sampled node"},
		{name: "invalid aggregate", mutate: func(p *SamplingPublication) { p.Nodes[0].Aggregate.Node.ID = "" }, want: "sampled node temporary"},
		{name: "create has authority", mutate: func(p *SamplingPublication) { p.Nodes[0].ExpectedRevision = 1 }, want: "new ownership"},
		{name: "duplicate formal node", mutate: func(p *SamplingPublication) {
			second := p.Nodes[0]
			second.TemporaryNodeID = "other"
			p.Nodes = append(p.Nodes, second)
		}, want: "duplicate formal sampled node"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publication := valid.Clone()
			tt.mutate(&publication)
			if err := publication.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestSamplingPublicationValidatesReuseAuthority(t *testing.T) {
	publication := samplingCreatePublication()
	node := &publication.Nodes[0]
	node.ResolutionMode = "REUSE"
	node.ExpectedRevision = node.Aggregate.Node.Revision
	node.ExpectedCurrentVersionID = node.Aggregate.Current.ID
	node.PublishVersion = false
	if err := publication.Validate(); err != nil {
		t.Fatalf("valid reuse rejected: %v", err)
	}

	node.ExpectedCurrentVersionID = "stale-version"
	if err := publication.Validate(); err == nil || !strings.Contains(err.Error(), "reuse must keep") {
		t.Fatalf("invalid reuse error = %v", err)
	}
}

func TestSamplingPublicationRejectsInvalidMergeAuthority(t *testing.T) {
	publication := samplingCreatePublication()
	node := &publication.Nodes[0]
	node.ResolutionMode = "MERGE"
	node.ExpectedRevision = node.Aggregate.Node.Revision
	node.ExpectedCurrentVersionID = node.Aggregate.Current.ID
	if err := publication.Validate(); err == nil || !strings.Contains(err.Error(), "merge requires current revision") {
		t.Fatalf("invalid merge error = %v", err)
	}
}

func TestValidationIssuesErrorReportsSafeContext(t *testing.T) {
	if got := (ValidationIssues{}).Error(); got != "" {
		t.Fatalf("empty Error() = %q", got)
	}
	issues := ValidationIssues{
		{Code: IssueEnvironment, Location: "workflow.step", Recommendation: "provide env.region"},
		{Code: IssueWorkflowMissing},
	}
	want := "ENVIRONMENT_KEY_MISSING at workflow.step: provide env.region; WORKFLOW_UNAVAILABLE"
	if got := issues.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestNodeDependencyIdentitySeparatesAmbiguousPairs(t *testing.T) {
	first := NodeDependencyIdentity("ab", "c")
	second := NodeDependencyIdentity("a", "bc")
	if first == second || first != "ab\x00c" {
		t.Fatalf("dependency identities = %q/%q", first, second)
	}
}

func TestSamplingPublicationCloneOwnsNestedAggregates(t *testing.T) {
	publication := SamplingPublication{Workflow: versionedWorkflowAggregate(), Nodes: []SamplingNodePublication{{TemporaryNodeID: "temporary", Aggregate: versionedNodeAggregate()}}}
	clone := publication.Clone()

	clone.Workflow.Current.Definition.Steps[0].DisplayName = "changed"
	clone.Nodes[0].Aggregate.Current.Selectors[0].Value = "changed"
	clone.Nodes[0].Aggregate.Current.Fingerprint.Attributes["role"] = "changed"

	if publication.Workflow.Current.Definition.Steps[0].DisplayName == "changed" || publication.Nodes[0].Aggregate.Current.Selectors[0].Value == "changed" || publication.Nodes[0].Aggregate.Current.Fingerprint.Attributes["role"] == "changed" {
		t.Fatal("Clone() shares nested publication state")
	}
}
