package conformancetest_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	execution "github.com/Capsule7446/healix-core/application/execution"
	"github.com/Capsule7446/healix-core/application/execution/conformancetest"
	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/evidence"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
)

type replayRecord struct {
	payload string
	result  evidence.StepTransitionCommitResult
}

type referenceState struct {
	stepRevision       evidence.StepRevision
	terminalFacts      int
	acceptedFacts      map[string]struct{}
	lastSequence       uint64
	governanceRevision domainautomation.Revision
	streak             domainautomation.HealStreak
	effects            map[execution.HealTerminalEffectKind]struct{}
	publications       int
	reviews            int
	outboxRecords      int
	replays            map[string]replayRecord
}

type referenceFixture struct {
	mu    sync.Mutex
	fence domainexecution.WorkerFence
	state referenceState
	fault conformancetest.FaultPoint
	band  evidence.DecisionBand
}

func newReferenceFixture(_ *testing.T, band evidence.DecisionBand, priorQualifyingRuns int) conformancetest.Fixture {
	fixture := &referenceFixture{
		fence: domainexecution.WorkerFence{RunID: mustInstanceID("run"), ClaimToken: "claim"},
		band:  band,
		state: referenceState{
			stepRevision:       1,
			acceptedFacts:      map[string]struct{}{},
			governanceRevision: 1,
			effects:            map[execution.HealTerminalEffectKind]struct{}{},
			replays:            map[string]replayRecord{},
		},
	}
	planner := execution.NewDefaultHealGovernancePlanner()
	for index := 0; index < priorQualifyingRuns; index++ {
		sequence := uint64(index + 1)
		runID := fmt.Sprintf("run-prior-%d", index+1)
		observation := evidence.HealObservation{
			ID: "fact-" + runID, RunID: mustInstanceID(runID), ExecutionID: mustEntryID("execution"), StepExecutionID: mustStepExecutionID("step"),
			ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Confidence: 0.9,
			DecisionBand: band, Succeeded: true, ObservedAt: int64(sequence),
		}
		decision, err := planner.PlanHealGovernance(execution.HealGovernancePlan{
			Snapshot: execution.HealGovernanceSnapshot{
				Key:                  execution.HealGovernanceKey{ElementTargetID: "node", BaseNodeVersionID: "base"},
				CurrentNodeVersionID: "base", Revision: fixture.state.governanceRevision, Streak: fixture.state.streak,
			},
			Fact: execution.HealAcceptedFact{
				Kind: execution.HealAcceptedObservation, FactID: observation.ID, CommitID: "commit-" + runID,
				RunID: mustInstanceID(runID), Sequence: sequence, Observation: &observation,
			},
		})
		if err != nil {
			panic(err)
		}
		fixture.state.acceptedFacts[observation.ID] = struct{}{}
		fixture.state.lastSequence = sequence
		fixture.state.streak = decision.NextStreak
		fixture.state.governanceRevision++
	}
	return fixture
}

func cloneState(source referenceState) referenceState {
	clone := source
	clone.acceptedFacts = make(map[string]struct{}, len(source.acceptedFacts))
	for id := range source.acceptedFacts {
		clone.acceptedFacts[id] = struct{}{}
	}
	clone.streak.Contributions = append([]domainautomation.ContributingHealFact(nil), source.streak.Contributions...)
	clone.effects = make(map[execution.HealTerminalEffectKind]struct{}, len(source.effects))
	for kind := range source.effects {
		clone.effects[kind] = struct{}{}
	}
	clone.replays = make(map[string]replayRecord, len(source.replays))
	for id, replay := range source.replays {
		replay.result.Promotions = append([]evidence.NodeVersionPromotion(nil), replay.result.Promotions...)
		clone.replays[id] = replay
	}
	return clone
}

func (f *referenceFixture) Fence() domainexecution.WorkerFence { return f.fence }

func (f *referenceFixture) Snapshot() conformancetest.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return snapshot(f.state)
}

func snapshot(state referenceState) conformancetest.Snapshot {
	return conformancetest.Snapshot{
		StepRevision: state.stepRevision, TerminalFacts: state.terminalFacts,
		AcceptedFacts: len(state.acceptedFacts), LastSequence: state.lastSequence,
		GovernanceRevision: state.governanceRevision, Effects: len(state.effects),
		Publications: state.publications, Reviews: state.reviews,
		OutboxRecords: state.outboxRecords, ReplayResults: len(state.replays),
	}
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

func (f *referenceFixture) CommitStepTransition(_ context.Context, fence domainexecution.WorkerFence, commit evidence.StepTransitionCommit, planner execution.HealGovernancePlanner) (evidence.StepTransitionCommitResult, error) {
	if err := execution.ValidateStepTransitionPayloadSize(commit); err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if fence != f.fence {
		return evidence.StepTransitionCommitResult{}, domainexecution.NewStaleWorkerFenceError()
	}
	payload := fmt.Sprintf("%#v", commit)
	if replay, exists := f.state.replays[commit.CommitID]; exists {
		if replay.payload != payload {
			return evidence.StepTransitionCommitResult{}, execution.CommitIdentityConflictError()
		}
		result := replay.result
		result.WasApplied = false
		result.Promotions = append([]evidence.NodeVersionPromotion(nil), result.Promotions...)
		return result, nil
	}
	if commit.ExpectedRevision != f.state.stepRevision {
		return evidence.StepTransitionCommitResult{}, execution.StepRevisionConflictError()
	}
	next := cloneState(f.state)
	next.terminalFacts++
	if err := f.fail(conformancetest.FaultAfterTerminalFacts); err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	for index := range commit.HealObservations {
		observation := commit.HealObservations[index]
		if _, exists := next.acceptedFacts[observation.ID]; exists {
			continue
		}
		next.acceptedFacts[observation.ID] = struct{}{}
		if err := f.fail(conformancetest.FaultAfterAcceptedFacts); err != nil {
			return evidence.StepTransitionCommitResult{}, err
		}
		next.lastSequence++
		decision, err := planner.PlanHealGovernance(execution.HealGovernancePlan{
			Snapshot: execution.HealGovernanceSnapshot{
				Key:                  execution.HealGovernanceKey{ElementTargetID: observation.ElementTargetID, BaseNodeVersionID: observation.BaseNodeVersionID},
				CurrentNodeVersionID: observation.BaseNodeVersionID, Revision: next.governanceRevision, Streak: next.streak,
			},
			Fact: execution.HealAcceptedFact{
				Kind: execution.HealAcceptedObservation, FactID: observation.ID, CommitID: commit.CommitID,
				RunID: observation.RunID, Sequence: next.lastSequence, Observation: &observation,
			},
		})
		if err != nil {
			return evidence.StepTransitionCommitResult{}, err
		}
		if decision.Key.ElementTargetID != observation.ElementTargetID || decision.Key.BaseNodeVersionID != observation.BaseNodeVersionID || decision.FactID != observation.ID || decision.Sequence != next.lastSequence || decision.ExpectedRevision != next.governanceRevision {
			return evidence.StepTransitionCommitResult{}, errors.New("planner decision authority mismatch")
		}
		next.streak = decision.NextStreak
		next.governanceRevision++
		if err := f.fail(conformancetest.FaultAfterGovernance); err != nil {
			return evidence.StepTransitionCommitResult{}, err
		}
		if decision.Effect != nil {
			if _, exists := next.effects[decision.Effect.Kind]; exists {
				return evidence.StepTransitionCommitResult{}, errors.New("duplicate terminal effect")
			}
			next.effects[decision.Effect.Kind] = struct{}{}
			if err := f.fail(conformancetest.FaultAfterEffect); err != nil {
				return evidence.StepTransitionCommitResult{}, err
			}
			switch decision.Effect.Kind {
			case execution.HealEffectAutoPublish:
				next.publications++
			case execution.HealEffectAwaitApproval:
				next.reviews++
			}
			if err := f.fail(conformancetest.FaultAfterAsset); err != nil {
				return evidence.StepTransitionCommitResult{}, err
			}
			next.outboxRecords++
			if err := f.fail(conformancetest.FaultAfterOutbox); err != nil {
				return evidence.StepTransitionCommitResult{}, err
			}
		}
	}
	next.stepRevision++
	promotions := []evidence.NodeVersionPromotion(nil)
	if next.publications > f.state.publications {
		promotions = []evidence.NodeVersionPromotion{{ElementTargetID: "node", VersionID: "version-1"}}
	}
	result := evidence.StepTransitionCommitResult{Revision: next.stepRevision, WasApplied: true, Promotions: promotions}
	next.replays[commit.CommitID] = replayRecord{payload: payload, result: result}
	if err := f.fail(conformancetest.FaultBeforeReplayResult); err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	f.state = next
	return result, nil
}

func (f *referenceFixture) fail(point conformancetest.FaultPoint) error {
	if f.fault == point {
		return fmt.Errorf("modeled failure at %s", point)
	}
	return nil
}

func TestReferenceStepTransitionTransactionConformance(t *testing.T) {
	conformancetest.Run(t, newReferenceFixture)
}

func TestReferenceStateCloneIsIndependent(t *testing.T) {
	fixture := newReferenceFixture(t, evidence.DecisionApplied, 2).(*referenceFixture)
	clone := cloneState(fixture.state)
	clone.acceptedFacts["other"] = struct{}{}
	clone.streak.Contributions[0].RunID = "other"
	if reflect.DeepEqual(clone, fixture.state) {
		t.Fatal("reference state clone aliases source")
	}
}

var _ conformancetest.Fixture = (*referenceFixture)(nil)
