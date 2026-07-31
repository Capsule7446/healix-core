package conformancetest_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	application "github.com/Capsule7446/healix-core/application/automation"
	"github.com/Capsule7446/healix-core/application/automation/conformancetest"
	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type storedNode struct {
	revision domain.Revision
	current  string
}

type referenceState struct {
	nodes     map[string]storedNode
	versions  map[string]struct{}
	workflows map[string]struct{}
	mappings  []domain.SamplingNodeMapping
	audits    []domain.SamplingNodeMapping
	outbox    []string
	replays   map[string]application.PublishSamplingOutcome
	digests   map[string]string
}

type referenceFixture struct {
	mu     sync.Mutex
	state  referenceState
	intent application.PublishSamplingIntent
	fault  conformancetest.FaultPoint
}

func newReferenceFixture(t *testing.T) conformancetest.Fixture {
	t.Helper()
	base, err := domain.NewElementTarget(
		domain.ElementTarget{ID: "existing", DisplayName: "existing", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1},
		domain.ElementTargetVersion{ID: "existing-v1", PageURL: "/old", Origin: "old", Selectors: selectors("#old"), Fingerprint: fp("old"), Source: domain.SourceManual, CreatedAt: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	reused, err := domain.NewElementTarget(
		domain.ElementTarget{ID: "reused", DisplayName: "reused", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1},
		domain.ElementTargetVersion{ID: "reused-v1", PageURL: "/reuse", Origin: "old", Selectors: selectors("#reuse"), Fingerprint: fp("reuse"), Source: domain.SourceManual, CreatedAt: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := base.PublishVersion("existing-v2", "/new", "new", selectors("#new"), fp("new"), domain.SourceSampling, 2)
	if err != nil {
		t.Fatal(err)
	}
	created, err := domain.NewElementTarget(
		domain.ElementTarget{ID: "created", DisplayName: "created", Properties: domain.Properties{}, CreatedAt: 2, UpdatedAt: 2},
		domain.ElementTargetVersion{ID: "created-v1", PageURL: "/created", Origin: "new", Selectors: selectors("#created"), Fingerprint: fp("created"), Source: domain.SourceSampling, CreatedAt: 2},
	)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := domain.NewFlowFragment(
		domain.FlowFragment{ID: "workflow", DisplayName: "workflow", Properties: domain.Properties{}, CreatedAt: 2, UpdatedAt: 2},
		domain.FlowFragmentVersion{ID: "workflow-v1", Definition: domain.FlowFragmentContent{Steps: []domain.FlowFragmentStep{{ID: "merge", DisplayName: "merge", Kind: domain.StepAction, Action: "click", ElementTargetID: "existing", ElementTargetVersionID: "existing-v2"}, {ID: "create", DisplayName: "create", Kind: domain.StepAction, Action: "click", ElementTargetID: "created", ElementTargetVersionID: "created-v1"}, {ID: "reuse", DisplayName: "reuse", Kind: domain.StepAction, Action: "click", ElementTargetID: "reused", ElementTargetVersionID: "reused-v1"}}}, CreatedAt: 2},
	)
	if err != nil {
		t.Fatal(err)
	}
	publication := domain.SamplingPublication{Nodes: []domain.SamplingElementTargetPublication{
		{TemporaryElementTargetID: "temporary-existing", ResolutionMode: "MERGE", Aggregate: merged, ExpectedRevision: base.ElementTarget.Revision, ExpectedCurrentVersionID: base.Current.ID, PublishVersion: true},
		{TemporaryElementTargetID: "temporary-created", ResolutionMode: "CREATE", Aggregate: created, PublishVersion: true},
		{TemporaryElementTargetID: "temporary-reused", ResolutionMode: "REUSE", Aggregate: reused, ExpectedRevision: reused.ElementTarget.Revision, ExpectedCurrentVersionID: reused.Current.ID},
	}, FlowFragment: workflow}
	command := application.SamplingPublicationCommand{PublicationID: "publication", Publication: publication}
	digest, err := application.SamplingPublicationRequestDigest(command)
	if err != nil {
		t.Fatal(err)
	}
	return &referenceFixture{
		state: referenceState{
			nodes:    map[string]storedNode{"existing": {revision: base.ElementTarget.Revision, current: base.Current.ID}, "reused": {revision: reused.ElementTarget.Revision, current: reused.Current.ID}},
			versions: map[string]struct{}{base.Current.ID: {}, reused.Current.ID: {}}, workflows: map[string]struct{}{},
			replays: map[string]application.PublishSamplingOutcome{}, digests: map[string]string{},
		},
		intent: application.PublishSamplingIntent{PublicationID: command.PublicationID, RequestDigest: digest, Publication: publication},
	}
}

func cloneSamplingOutcome(outcome application.PublishSamplingOutcome) application.PublishSamplingOutcome {
	outcome.Result.Nodes = append([]domain.SamplingNodeMapping(nil), outcome.Result.Nodes...)
	return outcome
}

func cloneSamplingIntent(intent application.PublishSamplingIntent) application.PublishSamplingIntent {
	intent.Publication = intent.Publication.Clone()
	return intent
}

func (f *referenceFixture) Intent() application.PublishSamplingIntent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneSamplingIntent(f.intent)
}
func (f *referenceFixture) SetFault(point conformancetest.FaultPoint) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fault = point
}
func (f *referenceFixture) ClearFault() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fault = ""
}
func (f *referenceFixture) MakeRevisionStale(nodeID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	current := f.state.nodes[nodeID]
	current.revision++
	f.state.nodes[nodeID] = current
}
func (f *referenceFixture) MakeCurrentVersionStale(nodeID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	current := f.state.nodes[nodeID]
	current.current += "-new"
	f.state.nodes[nodeID] = current
}
func (f *referenceFixture) Snapshot() conformancetest.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneState(f.state)
}

func (f *referenceFixture) CompetingIntents() (application.PublishSamplingIntent, application.PublishSamplingIntent) {
	return competingIntent(f.intent, "left"), competingIntent(f.intent, "right")
}

func competingIntent(base application.PublishSamplingIntent, suffix string) application.PublishSamplingIntent {
	publication := base.Publication.Clone()
	publication.FlowFragment.FlowFragment.ID += "-" + suffix
	publication.FlowFragment.Current.ID += "-" + suffix
	publication.FlowFragment.FlowFragment.CurrentVersionID = publication.FlowFragment.Current.ID
	publication.FlowFragment.Current.FlowFragmentID = publication.FlowFragment.FlowFragment.ID
	for index := range publication.Nodes {
		node := &publication.Nodes[index]
		switch node.ResolutionMode {
		case "MERGE":
			node.Aggregate.Current.ID += "-" + suffix
			node.Aggregate.ElementTarget.CurrentVersionID = node.Aggregate.Current.ID
		case "CREATE":
			node.Aggregate.ElementTarget.ID += "-" + suffix
			node.Aggregate.Current.ID += "-" + suffix
			node.Aggregate.ElementTarget.CurrentVersionID = node.Aggregate.Current.ID
			node.Aggregate.Current.ElementTargetID = node.Aggregate.ElementTarget.ID
		}
	}
	for index := range publication.FlowFragment.Current.Definition.Steps {
		step := &publication.FlowFragment.Current.Definition.Steps[index]
		for _, node := range publication.Nodes {
			if step.ID == "merge" && node.ResolutionMode == "MERGE" || step.ID == "create" && node.ResolutionMode == "CREATE" {
				step.ElementTargetID = node.Aggregate.ElementTarget.ID
				step.ElementTargetVersionID = node.Aggregate.Current.ID
			}
		}
	}
	command := application.SamplingPublicationCommand{PublicationID: "publication-" + suffix, Publication: publication}
	digest, err := application.SamplingPublicationRequestDigest(command)
	if err != nil {
		panic(err)
	}
	return application.PublishSamplingIntent{PublicationID: command.PublicationID, RequestDigest: digest, Publication: publication}
}

func expectedOutbox(publication domain.SamplingPublication) []string {
	mappings := conformancetest.Result(publication).Nodes
	events := make([]string, 0, len(mappings)+1)
	for _, mapping := range mappings {
		events = append(events, "NODE:"+mapping.ElementTargetID+":"+mapping.ElementTargetVersionID)
	}
	return append(events, "WORKFLOW:"+publication.FlowFragment.FlowFragment.ID)
}

func (f *referenceFixture) AssertOnlyApplied(winner, loser application.PublishSamplingIntent) error {
	if err := f.AssertApplied(winner); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.state.replays[loser.PublicationID]; exists {
		return errors.New("loser replay receipt exists")
	}
	if _, exists := f.state.digests[loser.PublicationID]; exists {
		return errors.New("loser digest exists")
	}
	if _, exists := f.state.workflows[loser.Publication.FlowFragment.FlowFragment.ID]; exists {
		return errors.New("loser workflow exists")
	}
	loserResult := conformancetest.Result(loser.Publication)
	for index, mapping := range loserResult.Nodes {
		decision := loser.Publication.Nodes[index]
		if decision.ResolutionMode != "REUSE" {
			if _, exists := f.state.versions[mapping.ElementTargetVersionID]; exists {
				return errors.New("loser target version exists")
			}
		}
		if decision.ResolutionMode == "CREATE" {
			if _, exists := f.state.nodes[mapping.ElementTargetID]; exists {
				return errors.New("loser target exists")
			}
		}
		if decision.ResolutionMode == "REUSE" {
			continue
		}
		for _, stored := range append(append([]domain.SamplingNodeMapping(nil), f.state.mappings...), f.state.audits...) {
			if reflect.DeepEqual(mapping, stored) {
				return errors.New("loser mapping or audit exists")
			}
		}
	}
	if !reflect.DeepEqual(f.state.outbox, expectedOutbox(winner.Publication)) {
		return errors.New("outbox does not exactly match winner publication")
	}
	return nil
}

func (f *referenceFixture) AssertApplied(intent application.PublishSamplingIntent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	expectedMappings := conformancetest.Result(intent.Publication).Nodes
	if !reflect.DeepEqual(f.state.mappings, expectedMappings) || !reflect.DeepEqual(f.state.audits, expectedMappings) {
		return errors.New("mappings or mode audit do not match publication")
	}
	if !reflect.DeepEqual(f.state.outbox, expectedOutbox(intent.Publication)) || len(f.state.replays) != 1 {
		return errors.New("outbox or replay result does not exactly match publication")
	}
	for _, decision := range intent.Publication.Nodes {
		stored, exists := f.state.nodes[decision.Aggregate.ElementTarget.ID]
		if !exists || stored.current != decision.Aggregate.Current.ID || stored.revision != decision.Aggregate.ElementTarget.Revision {
			return errors.New("node materialization does not match publication")
		}
		if _, exists := f.state.versions[decision.Aggregate.Current.ID]; !exists {
			return errors.New("node version was not materialized")
		}
	}
	if _, exists := f.state.workflows[intent.Publication.FlowFragment.FlowFragment.ID]; !exists {
		return errors.New("workflow was not materialized")
	}
	return nil
}

func (f *referenceFixture) LookupSamplingPublication(_ context.Context, publicationID, digest string) (application.PublishSamplingOutcome, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.state.replays[publicationID]
	if !ok {
		return application.PublishSamplingOutcome{}, false, nil
	}
	if f.state.digests[publicationID] != digest {
		return application.PublishSamplingOutcome{}, false, application.SamplingPublicationIdentityConflictError()
	}
	existing.Status = application.PublishSamplingReplayed
	return cloneSamplingOutcome(existing), true, nil
}

func (f *referenceFixture) PublishSampling(_ context.Context, intent application.PublishSamplingIntent) (application.PublishSamplingOutcome, error) {
	if err := application.ValidatePublishSamplingIntentDigest(intent); err != nil {
		return application.PublishSamplingOutcome{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.state.replays[intent.PublicationID]; ok {
		if f.state.digests[intent.PublicationID] != intent.RequestDigest {
			return application.PublishSamplingOutcome{}, application.SamplingPublicationIdentityConflictError()
		}
		existing.Status = application.PublishSamplingReplayed
		return cloneSamplingOutcome(existing), nil
	}
	next := cloneState(f.state)
	for _, decision := range intent.Publication.Nodes {
		nodeID, versionID := decision.Aggregate.ElementTarget.ID, decision.Aggregate.Current.ID
		switch decision.ResolutionMode {
		case "CREATE":
			if _, exists := next.nodes[nodeID]; exists {
				return application.PublishSamplingOutcome{}, errors.New("node identity conflict")
			}
		case "MERGE", "REUSE":
			current, exists := next.nodes[nodeID]
			if !exists || current.revision != decision.ExpectedRevision || current.current != decision.ExpectedCurrentVersionID {
				return application.PublishSamplingOutcome{}, application.ErrSamplingPublicationAuthorityConflict
			}
		}
		if decision.PublishVersion {
			if _, exists := next.versions[versionID]; exists {
				return application.PublishSamplingOutcome{}, errors.New("node version identity conflict")
			}
			next.nodes[nodeID] = storedNode{revision: decision.Aggregate.ElementTarget.Revision, current: versionID}
			next.versions[versionID] = struct{}{}
		}
	}
	if err := f.fail(conformancetest.FaultAfterNodes); err != nil {
		return application.PublishSamplingOutcome{}, err
	}
	workflowID := intent.Publication.FlowFragment.FlowFragment.ID
	if _, exists := next.workflows[workflowID]; exists {
		return application.PublishSamplingOutcome{}, errors.New("workflow identity conflict")
	}
	next.workflows[workflowID] = struct{}{}
	if err := f.fail(conformancetest.FaultAfterWorkflow); err != nil {
		return application.PublishSamplingOutcome{}, err
	}
	mappings := conformancetest.Result(intent.Publication).Nodes
	next.mappings = append(next.mappings, mappings...)
	if err := f.fail(conformancetest.FaultAfterMappings); err != nil {
		return application.PublishSamplingOutcome{}, err
	}
	next.audits = append(next.audits, mappings...)
	if err := f.fail(conformancetest.FaultAfterAudit); err != nil {
		return application.PublishSamplingOutcome{}, err
	}
	for _, mapping := range mappings {
		next.outbox = append(next.outbox, "NODE:"+mapping.ElementTargetID+":"+mapping.ElementTargetVersionID)
	}
	next.outbox = append(next.outbox, "WORKFLOW:"+workflowID)
	if err := f.fail(conformancetest.FaultAfterOutbox); err != nil {
		return application.PublishSamplingOutcome{}, err
	}
	if err := f.fail(conformancetest.FaultBeforeReplay); err != nil {
		return application.PublishSamplingOutcome{}, err
	}
	outcome := application.PublishSamplingOutcome{Status: application.PublishSamplingApplied, PublicationID: intent.PublicationID, RequestDigest: intent.RequestDigest, Result: conformancetest.Result(intent.Publication)}
	next.replays[intent.PublicationID], next.digests[intent.PublicationID] = cloneSamplingOutcome(outcome), intent.RequestDigest
	f.state = next
	return cloneSamplingOutcome(outcome), nil
}

func (f *referenceFixture) fail(point conformancetest.FaultPoint) error {
	if f.fault == point {
		return errors.New("injected fault")
	}
	return nil
}

func cloneState(state referenceState) referenceState {
	cloned := referenceState{nodes: map[string]storedNode{}, versions: map[string]struct{}{}, workflows: map[string]struct{}{}, mappings: append([]domain.SamplingNodeMapping(nil), state.mappings...), audits: append([]domain.SamplingNodeMapping(nil), state.audits...), outbox: append([]string(nil), state.outbox...), replays: map[string]application.PublishSamplingOutcome{}, digests: map[string]string{}}
	for key, value := range state.nodes {
		cloned.nodes[key] = value
	}
	for key := range state.versions {
		cloned.versions[key] = struct{}{}
	}
	for key := range state.workflows {
		cloned.workflows[key] = struct{}{}
	}
	for key, value := range state.replays {
		value.Result.Nodes = append([]domain.SamplingNodeMapping(nil), value.Result.Nodes...)
		cloned.replays[key] = value
	}
	for key, value := range state.digests {
		cloned.digests[key] = value
	}
	return cloned
}

func selectors(value string) []fingerprint.Selector {
	return []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: value}}
}
func fp(tag string) fingerprint.Fingerprint {
	return fingerprint.Fingerprint{Tag: tag, Attributes: map[string]string{}}
}

func TestReferenceSamplingPublicationTransactionConformance(t *testing.T) {
	conformancetest.Run(t, newReferenceFixture)
}
