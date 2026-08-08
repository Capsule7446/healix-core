package conformancetest

import (
	"context"
	"reflect"
	"sync"
	"testing"

	execution "github.com/Capsule7446/healix-core/application/execution"
	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/evidence"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// FaultPoint 标识步骤转换事务中用于验证原子回滚的故障注入位置。
type FaultPoint string

const (
	// FaultAfterTerminalFacts 在终态事实写入后注入故障。
	FaultAfterTerminalFacts FaultPoint = "AFTER_TERMINAL_FACTS"
	// FaultAfterAcceptedFacts 在已接受自愈事实写入后注入故障。
	FaultAfterAcceptedFacts FaultPoint = "AFTER_ACCEPTED_FACTS"
	// FaultAfterGovernance 在治理状态写入后注入故障。
	FaultAfterGovernance FaultPoint = "AFTER_GOVERNANCE"
	// FaultAfterEffect 在终态效果写入后注入故障。
	FaultAfterEffect FaultPoint = "AFTER_EFFECT"
	// FaultAfterAsset 在资产发布或审核写入后注入故障。
	FaultAfterAsset FaultPoint = "AFTER_ASSET"
	// FaultAfterOutbox 在 outbox 记录写入后注入故障。
	FaultAfterOutbox FaultPoint = "AFTER_OUTBOX"
	// FaultBeforeReplayResult 在返回重放结果前注入故障。
	FaultBeforeReplayResult FaultPoint = "BEFORE_REPLAY_RESULT"
)

// Snapshot 保存步骤转换、治理、效果、资产和重放结果的宿主状态快照。
type Snapshot struct {
	StepRevision       evidence.StepRevision
	TerminalFacts      int
	AcceptedFacts      int
	LastSequence       uint64
	GovernanceRevision domainautomation.Revision
	Effects            int
	Publications       int
	Reviews            int
	OutboxRecords      int
	ReplayResults      int
}

// Fixture 定义步骤转换 conformance 测试所需的事务、状态读取和故障控制端口。
type Fixture interface {
	execution.StepTransitionTransaction
	// Fence 返回此夹具接受的工作线程权威。
	Fence() domainexecution.WorkerFence
	// Snapshot 返回当前宿主状态快照。
	Snapshot() Snapshot
	// SetFault 在指定事务阶段注入故障。
	SetFault(FaultPoint)
	// ClearFault 清除当前故障注入。
	ClearFault()
}

// Factory 创建给定决策区间和已有合格运行次数的步骤转换测试夹具。
type Factory func(t *testing.T, band evidence.DecisionBand, priorQualifyingRuns int) Fixture

// Run 运行步骤转换的 stale fence/修订、权威重放、阈值效果、回滚和并发 CAS conformance 场景。
func Run(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("stale-fence-and-revision-write-nothing", func(t *testing.T) {
		fixture := factory(t, evidence.DecisionApplied, 0)
		before := fixture.Snapshot()
		stale := fixture.Fence()
		stale.ClaimToken += "-stale"
		if _, err := fixture.CommitStepTransition(context.Background(), stale, commit("commit-stale", 1, "run-stale", evidence.DecisionApplied), execution.NewDefaultHealGovernancePlanner()); !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) {
			t.Fatalf("stale fence error = %v", err)
		}
		if got := fixture.Snapshot(); !reflect.DeepEqual(got, before) {
			t.Fatalf("stale fence changed state: before=%#v after=%#v", before, got)
		}
		badRevision := commit("commit-revision", before.StepRevision+1, "run-revision", evidence.DecisionApplied)
		if _, err := fixture.CommitStepTransition(context.Background(), fixture.Fence(), badRevision, execution.NewDefaultHealGovernancePlanner()); !fault.IsCode(err, execution.CodeStepRevisionConflict) {
			t.Fatalf("stale revision error = %v", err)
		}
		if got := fixture.Snapshot(); !reflect.DeepEqual(got, before) {
			t.Fatalf("stale revision changed state: before=%#v after=%#v", before, got)
		}
	})

	t.Run("authoritative-replay-and-identity-conflict", func(t *testing.T) {
		fixture := factory(t, evidence.DecisionApplied, 0)
		command := commit("commit-replay", fixture.Snapshot().StepRevision, "run-replay", evidence.DecisionApplied)
		first, err := fixture.CommitStepTransition(context.Background(), fixture.Fence(), command, execution.NewDefaultHealGovernancePlanner())
		if err != nil || !first.WasApplied {
			t.Fatalf("first commit = %#v, %v", first, err)
		}
		afterFirst := fixture.Snapshot()
		replay, err := fixture.CommitStepTransition(context.Background(), fixture.Fence(), command, execution.NewDefaultHealGovernancePlanner())
		if err != nil || replay.WasApplied || replay.Revision != first.Revision || !reflect.DeepEqual(replay.Promotions, first.Promotions) {
			t.Fatalf("replay = %#v, %v; first = %#v", replay, err, first)
		}
		if got := fixture.Snapshot(); !reflect.DeepEqual(got, afterFirst) {
			t.Fatalf("replay changed state: before=%#v after=%#v", afterFirst, got)
		}
		changed := command
		changed.Event.Timestamp++
		beforeConflict := fixture.Snapshot()
		if _, err := fixture.CommitStepTransition(context.Background(), fixture.Fence(), changed, execution.NewDefaultHealGovernancePlanner()); !fault.IsCode(err, execution.CodeCommitIdentityConflict) {
			t.Fatalf("identity conflict error = %v", err)
		}
		if got := fixture.Snapshot(); !reflect.DeepEqual(got, beforeConflict) {
			t.Fatalf("identity conflict changed state: before=%#v after=%#v", beforeConflict, got)
		}
	})

	for _, test := range []struct {
		name         string
		band         evidence.DecisionBand
		publications int
		reviews      int
	}{
		{"applied-third-run-publishes-once", evidence.DecisionApplied, 1, 0},
		{"below-cap-third-run-creates-review", evidence.DecisionBelowCap, 0, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := factory(t, test.band, 2)
			before := fixture.Snapshot()
			command := commit("commit-threshold", before.StepRevision, "run-third", test.band)
			result, err := fixture.CommitStepTransition(context.Background(), fixture.Fence(), command, execution.NewDefaultHealGovernancePlanner())
			if err != nil || !result.WasApplied {
				t.Fatalf("threshold commit = %#v, %v", result, err)
			}
			after := fixture.Snapshot()
			if after.AcceptedFacts != before.AcceptedFacts+1 || after.LastSequence != before.LastSequence+1 || after.Effects != before.Effects+1 || after.Publications != before.Publications+test.publications || after.Reviews != before.Reviews+test.reviews {
				t.Fatalf("threshold state: before=%#v after=%#v", before, after)
			}
		})
	}

	t.Run("faults-roll-back-all-observable-state", func(t *testing.T) {
		for _, point := range []FaultPoint{FaultAfterTerminalFacts, FaultAfterAcceptedFacts, FaultAfterGovernance, FaultAfterEffect, FaultAfterAsset, FaultAfterOutbox, FaultBeforeReplayResult} {
			t.Run(string(point), func(t *testing.T) {
				fixture := factory(t, evidence.DecisionApplied, 2)
				before := fixture.Snapshot()
				fixture.SetFault(point)
				_, err := fixture.CommitStepTransition(context.Background(), fixture.Fence(), commit("commit-fault", before.StepRevision, "run-fault", evidence.DecisionApplied), execution.NewDefaultHealGovernancePlanner())
				if err == nil {
					t.Fatal("faulted commit succeeded")
				}
				if got := fixture.Snapshot(); !reflect.DeepEqual(got, before) {
					t.Fatalf("fault %s changed state: before=%#v after=%#v", point, before, got)
				}
			})
		}
	})

	t.Run("concurrent-threshold-crossing-has-one-effect", func(t *testing.T) {
		fixture := factory(t, evidence.DecisionApplied, 2)
		before := fixture.Snapshot()
		commands := []evidence.StepTransitionCommit{
			commit("commit-race-a", before.StepRevision, "run-race-a", evidence.DecisionApplied),
			commit("commit-race-b", before.StepRevision, "run-race-b", evidence.DecisionApplied),
		}
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(len(commands))
		errorsFound := make(chan error, len(commands))
		for _, command := range commands {
			command := command
			go func() {
				defer wait.Done()
				<-start
				_, err := fixture.CommitStepTransition(context.Background(), fixture.Fence(), command, execution.NewDefaultHealGovernancePlanner())
				errorsFound <- err
			}()
		}
		close(start)
		wait.Wait()
		close(errorsFound)
		succeeded := 0
		for err := range errorsFound {
			if err == nil {
				succeeded++
			} else if !fault.IsCode(err, execution.CodeStepRevisionConflict) {
				t.Fatalf("race error = %v", err)
			}
		}
		after := fixture.Snapshot()
		if succeeded != 1 || after.Effects != before.Effects+1 || after.Publications != before.Publications+1 {
			t.Fatalf("race successes/state = %d/%#v", succeeded, after)
		}
	})
}

// mustInstanceID 构造测试用 InstanceID；输入来自本文件中的字面量，失败表示测试夹具缺陷。
func mustInstanceID(value string) domainexecution.InstanceID {
	id, err := domainexecution.NewInstanceID(value)
	if err != nil {
		panic(err)
	}
	return id
}

// mustEntryID 构造测试用 EntryID；失败表示测试夹具缺陷。
func mustEntryID(value string) domainexecution.EntryID {
	id, err := domainexecution.NewEntryID(value)
	if err != nil {
		panic(err)
	}
	return id
}

// mustStepExecutionID 构造测试用 StepExecutionID；失败表示测试夹具缺陷。
func mustStepExecutionID(value string) domainexecution.StepExecutionID {
	id, err := domainexecution.NewStepExecutionID(value)
	if err != nil {
		panic(err)
	}
	return id
}

// commit 构造包含步骤事件和自愈观测的测试提交。
func commit(id string, revision evidence.StepRevision, instanceID string, band evidence.DecisionBand) evidence.StepTransitionCommit {
	return evidence.StepTransitionCommit{
		CommitID:         id,
		ExpectedRevision: revision,
		Event: evidence.StepPhaseEvent{
			ID: mustStepExecutionID("step"), EntryID: mustEntryID("execution"), FlowFragmentStepID: "workflow-step", DisplayName: "step",
			InvocationPath: domainexecution.RootInvocationPath(mustEntryID("execution")),
			Kind:           "ACTION", Phase: "SUCCEEDED", Occurrence: 1, Timestamp: 1,
		},
		HealObservations: []evidence.HealObservation{{
			ID: "fact-" + instanceID, InstanceID: mustInstanceID(instanceID), EntryID: mustEntryID("execution"), StepExecutionID: mustStepExecutionID("step"), Occurrence: 1,
			ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Confidence: 0.9,
			DecisionBand: band, Succeeded: true, ObservedAt: 1,
		}},
	}
}
