package automation

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestHealCandidateReviewTransitions(t *testing.T) {
	candidate := HealCandidate{
		Hash:              "candidate",
		ElementTargetID:   "node",
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
			reviewed.Selectors[0].Value = "changed"
			reviewed.Fingerprint.Attributes["role"] = "changed"
			if candidate.Selectors[0].Value != "button" || candidate.Fingerprint.Attributes["role"] != "submit" {
				t.Fatalf("Review() shared nested state with receiver: %#v", candidate)
			}
		})
	}
}

func TestHealCandidateReviewRejectsRevisionExhaustionWithoutMutation(t *testing.T) {
	candidate := HealCandidate{
		Hash:              "candidate-secret",
		ElementTargetID:   "node-secret",
		BaseNodeVersionID: "version-secret",
		Status:            HealCandidateAwaitingApproval,
		Revision:          Revision(math.MaxUint64),
	}

	reviewed, err := candidate.Review(HealCandidatePromoted)
	if !reflect.DeepEqual(reviewed, HealCandidate{}) || !fault.IsCode(err, CodeRevisionExhausted) || candidate.Status != HealCandidateAwaitingApproval || candidate.Revision != Revision(math.MaxUint64) {
		t.Fatalf("reviewed/error/original = %#v/%v/%#v", reviewed, err, candidate)
	}
	for _, secret := range []string{"candidate-secret", "node-secret", "version-secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("public error leaked %q: %q", secret, err.Error())
		}
	}
}

func TestHealCandidateRejectsInvalidIdentityAndTransitions(t *testing.T) {
	valid := HealCandidate{Hash: "candidate", ElementTargetID: "node", BaseNodeVersionID: "node-v1", Status: HealCandidateAwaitingApproval, Revision: 1}
	tests := []struct {
		name        string
		run         func() error
		wantCode    fault.Code
		wantKind    fault.Kind
		wantMessage string
	}{
		{name: "missing identity", run: func() error {
			return (HealCandidate{
				Hash:              " \t\n",
				ElementTargetID:   "node-secret",
				BaseNodeVersionID: "version-secret",
				Status:            HealCandidateAwaitingApproval,
				Revision:          1,
			}).Validate()
		}, wantCode: CodeHealCandidateIdentityInvalid, wantKind: fault.InvalidArgument, wantMessage: "heal candidate identity is invalid"},
		{name: "not awaiting approval", run: func() error {
			candidate := valid
			candidate.Hash = "candidate-secret"
			candidate.Status = HealCandidatePromoted
			return candidate.Validate()
		}, wantCode: CodeHealCandidateStateInvalid, wantKind: fault.FailedPrecondition, wantMessage: "heal candidate state does not allow this operation"},
		{name: "not reviewed", run: func() error {
			candidate := valid
			candidate.Hash = "candidate-secret"
			return candidate.ValidateReviewed()
		}, wantCode: CodeHealCandidateStateInvalid, wantKind: fault.FailedPrecondition, wantMessage: "heal candidate state does not allow this operation"},
		{name: "unsupported review status", run: func() error {
			_, err := valid.Review(HealCandidateStatus("malicious\nstatus-secret"))
			return err
		}, wantCode: CodeHealCandidateReviewStatusInvalid, wantKind: fault.InvalidArgument, wantMessage: "heal candidate review status is invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			descriptor, ok := fault.Describe(err)
			if !fault.IsCode(err, tt.wantCode) || !ok || descriptor.Code() != tt.wantCode || descriptor.Kind() != tt.wantKind || descriptor.Message() != tt.wantMessage || len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 {
				t.Fatalf("error/descriptor = %v/%#v", err, descriptor)
			}
			for _, secret := range []string{"candidate-secret", "node-secret", "version-secret", "status-secret", "malicious"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("public error leaked %q: %q", secret, err.Error())
				}
			}
		})
	}

	t.Run("unpersisted revision retains revision contract", func(t *testing.T) {
		candidate := valid
		candidate.Hash = "candidate-secret"
		candidate.Revision = 0
		err := candidate.Validate()
		if !fault.IsCode(err, CodePersistedRevisionInvalid) || strings.Contains(err.Error(), candidate.Hash) {
			t.Fatalf("error = %v", err)
		}
	})
}

func samplingCreatePublication() SamplingPublication {
	node := versionedNodeAggregate()
	node.ElementTarget.CurrentVersionID = "node-v1"
	node.Current = versionedNodeVersion("node-v1", "node", 1, 0)
	node.Versions = []ElementTargetVersion{node.Current}
	return SamplingPublication{
		FlowFragment: versionedWorkflowAggregate(),
		Nodes: []SamplingElementTargetPublication{{
			TemporaryElementTargetID: "temporary",
			ResolutionMode:           "CREATE",
			Aggregate:                node,
			PublishVersion:           true,
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
		{name: "missing temporary id", mutate: func(p *SamplingPublication) { p.Nodes[0].TemporaryElementTargetID = " " }, want: "temporary id is required"},
		{name: "unsupported resolution", mutate: func(p *SamplingPublication) { p.Nodes[0].ResolutionMode = "COPY" }, want: "unsupported resolution mode"},
		{name: "duplicate temporary id", mutate: func(p *SamplingPublication) { p.Nodes = append(p.Nodes, p.Nodes[0]) }, want: "duplicate sampled node"},
		// The previous expectation matched the temporary element target id itself,
		// which is precisely the value that must not reach the message. Nodes are now
		// addressed by their position in the caller's own slice.
		{name: "invalid aggregate", mutate: func(p *SamplingPublication) { p.Nodes[0].Aggregate.ElementTarget.ID = "" }, want: "sampled node 0"},
		{name: "create has authority", mutate: func(p *SamplingPublication) { p.Nodes[0].ExpectedRevision = 1 }, want: "new ownership"},
		{name: "duplicate formal node", mutate: func(p *SamplingPublication) {
			second := p.Nodes[0]
			second.TemporaryElementTargetID = "other"
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
	node.ExpectedRevision = node.Aggregate.ElementTarget.Revision
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
	node.ExpectedRevision = node.Aggregate.ElementTarget.Revision
	node.ExpectedCurrentVersionID = node.Aggregate.Current.ID
	if err := publication.Validate(); err == nil || !strings.Contains(err.Error(), "merge requires current revision") {
		t.Fatalf("invalid merge error = %v", err)
	}
}

func TestNodeDependencyIdentitySeparatesAmbiguousPairs(t *testing.T) {
	first := ElementTargetDependencyIdentity("ab", "c")
	second := ElementTargetDependencyIdentity("a", "bc")
	if first == second || first != "ab\x00c" {
		t.Fatalf("dependency identities = %q/%q", first, second)
	}
}

func TestSamplingPublicationCloneOwnsNestedAggregates(t *testing.T) {
	publication := SamplingPublication{FlowFragment: versionedWorkflowAggregate(), Nodes: []SamplingElementTargetPublication{{TemporaryElementTargetID: "temporary", Aggregate: versionedNodeAggregate()}}}
	clone := publication.Clone()

	clone.FlowFragment.Current.Definition.Steps[0].DisplayName = "changed"
	clone.Nodes[0].Aggregate.Current.Selectors[0].Value = "changed"
	clone.Nodes[0].Aggregate.Current.Fingerprint.Attributes["role"] = "changed"

	if publication.FlowFragment.Current.Definition.Steps[0].DisplayName == "changed" || publication.Nodes[0].Aggregate.Current.Selectors[0].Value == "changed" || publication.Nodes[0].Aggregate.Current.Fingerprint.Attributes["role"] == "changed" {
		t.Fatal("Clone() shares nested publication state")
	}
}
