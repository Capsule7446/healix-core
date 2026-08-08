package conformancetest

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Capsule7446/healix-core/application/engine"
	execution "github.com/Capsule7446/healix-core/application/execution"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// CompletionFaultPoint 标识一次入口完成内部套件要求夹具失败的位置。
//
// 这些位置对应 EntryCompletionTransaction 声明为原子的写入阶段。宿主若在事务外提交其中任一阶段，
// 回滚检查会捕获该问题，而不依赖审阅者阅读适配器实现。空值表示“无故障”，由 ClearFault 恢复。
type CompletionFaultPoint string

const (
	// CompletionFaultBeforeReplay 在 LookupEntryCompletion 内、读取任何内容前失败。宿主若在此之前
	// 已让完成可见，调用方将无法区分新请求与半应用请求。
	CompletionFaultBeforeReplay CompletionFaultPoint = "BEFORE_REPLAY"
	// CompletionFaultAfterDecision 在意图校验完成且首个写入尚未发生时失败。
	CompletionFaultAfterDecision CompletionFaultPoint = "AFTER_DECISION"
	// CompletionFaultAfterEntry 在入口终态和两个计数器写入后失败。
	CompletionFaultAfterEntry CompletionFaultPoint = "AFTER_ENTRY"
	// CompletionFaultAfterFacts 在入口终态事实写入后失败。
	CompletionFaultAfterFacts CompletionFaultPoint = "AFTER_FACTS"
	// CompletionFaultAfterEvidence 在运行的证据引用写入后失败。
	CompletionFaultAfterEvidence CompletionFaultPoint = "AFTER_EVIDENCE"
	// CompletionFaultAfterGate 在 execution action gate 终态化后失败。
	CompletionFaultAfterGate CompletionFaultPoint = "AFTER_GATE"
	// CompletionFaultAfterOutbox 在 outbox 记录写入后失败；这是幂等收据之前的最后一次写入，
	// 也是非原子宿主看起来最像已完成的阶段。
	CompletionFaultAfterOutbox CompletionFaultPoint = "AFTER_OUTBOX"
)

// CompletionFaultPoints 按查询优先、随后提交写入顺序列出套件注入的全部故障点。宿主可调用它驱动
// 自身测试，避免重新声明同一集合。
func CompletionFaultPoints() []CompletionFaultPoint {
	return []CompletionFaultPoint{
		CompletionFaultBeforeReplay,
		CompletionFaultAfterDecision,
		CompletionFaultAfterEntry,
		CompletionFaultAfterFacts,
		CompletionFaultAfterEvidence,
		CompletionFaultAfterGate,
		CompletionFaultAfterOutbox,
	}
}

// CompletionSnapshot 保存一次入口完成允许改变的全部状态，并作为单个可比较值读回。
//
// 前四个字段是入口自身状态，最终必须精确等于决策 Core 产生的值。计数器报告夹具执行各类写入的
// 次数；套件比较计数而不固定其数值，因为一次完成产生多少行属于宿主业务，但回滚尝试是否留下
// 任何痕迹不是。
type CompletionSnapshot struct {
	EntryStatus domainexecution.EntryStatus
	// TerminalCause 与 EntryStatus 一样被读回：D-18 使崩溃中断入口与运行后失败的入口保持可区分；
	// 宿主若决定了原因却未持久化，二者都会是 FAILED，而套件无法察觉。
	TerminalCause          execution.TerminalCause
	TerminalIntent         execution.TerminalIntent
	TerminalIntentRevision int64
	CancellationGeneration int64
	Completions            int
	TerminalFacts          int
	EvidenceRefs           int
	GateTerminalizations   int
	OutboxRecords          int
}

// CompletionFixture 表示一个待测试的宿主适配器，并补充端口本身未暴露的两项能力：读回已提交状态，
// 以及让适配器在指定位置失败。
//
// SetFault 和 ClearFault 必须可由驱动测试的 goroutine 在没有完成运行时安全调用；套件不会在预期
// 观察故障的调用并发执行期间修改故障设置。
type CompletionFixture interface {
	execution.EntryCompletionTransaction
	// Fence 返回此夹具接受的工作线程权威；携带其他 fence 的完成必须以 CodeWorkerFenceStale 拒绝。
	Fence() domainexecution.WorkerFence
	// EntryID 返回此夹具持有的入口。
	EntryID() domainexecution.EntryID
	// Snapshot 读回完成可能改变的全部状态。
	Snapshot() CompletionSnapshot
	// SetFault 在下一次完成的指定位置启用故障。
	SetFault(CompletionFaultPoint)
	// ClearFault 清除 SetFault 启用的故障。
	ClearFault()
}

// CompletionFactory 在给定状态下创建一个持有入口的新夹具。
//
// 其身份必须是确定的：返回的每个夹具报告相同 Fence 和 EntryID；从相同状态创建的两个夹具必须
// 以相同快照开始。套件从两个夹具构建干净运行和崩溃后重试运行并进行比较，这只有在两项保证成立
// 时才有意义。
type CompletionFactory func(t *testing.T, state execution.EntryCompletionState) CompletionFixture

// RunEntryCompletion 针对一个宿主适配器运行入口完成 conformance 套件。
//
// 它通过 EntryCompletionService 调用 EntryCompletionTransaction，因为这是访问端口的唯一支持路径；
// 仅在直接驱动端口时通过的宿主，尚未在实际运行路径上得到测试。每个子测试创建自己的夹具，
// 因此失败不会给下一个测试留下残留状态。
func RunEntryCompletion(t *testing.T, factory CompletionFactory) {
	t.Helper()

	t.Run("applies-once-then-replays", func(t *testing.T) {
		state := completionRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		service := execution.NewEntryCompletionService(fixture)
		before := fixture.Snapshot()
		command := completionCommand(fixture, state, completionFailedOutcome(), "")
		decision := mustCompletionDecision(t, command)

		applied, err := service.Complete(context.Background(), command)
		if err != nil {
			t.Fatalf("Complete() = %v, want nil", err)
		}
		if applied.Status != execution.CompleteEntryApplied {
			t.Fatalf("Complete() status = %s, want %s", applied.Status, execution.CompleteEntryApplied)
		}
		if applied.EntryID != fixture.EntryID() {
			t.Fatalf("Complete() entry = %s, want %s", applied.EntryID, fixture.EntryID())
		}
		if applied.Decision != decision {
			t.Fatalf("Complete() decision = %#v, want %#v", applied.Decision, decision)
		}
		after := fixture.Snapshot()
		assertCompletionCommitted(t, before, after, decision)

		replayed, err := service.Complete(context.Background(), command)
		if err != nil {
			t.Fatalf("replayed Complete() = %v, want nil", err)
		}
		if replayed.Status != execution.CompleteEntryReplayed {
			t.Fatalf("replayed status = %s, want %s", replayed.Status, execution.CompleteEntryReplayed)
		}
		if replayed.Decision != decision || replayed.RequestDigest != applied.RequestDigest {
			t.Fatalf("replay = %#v, want the recorded outcome %#v", replayed, applied)
		}
		if got := fixture.Snapshot(); got != after {
			t.Fatalf("replay changed state: before=%#v after=%#v", after, got)
		}
	})

	t.Run("lookup-of-an-unknown-request-finds-nothing-and-writes-nothing", func(t *testing.T) {
		state := completionRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		before := fixture.Snapshot()
		command := completionCommand(fixture, state, completionFailedOutcome(), "")
		digest, err := execution.CompleteEntryRequestDigest(command)
		if err != nil {
			t.Fatalf("CompleteEntryRequestDigest() = %v, want nil", err)
		}

		recorded, found, err := fixture.LookupEntryCompletion(context.Background(), fixture.EntryID(), digest)
		if err != nil {
			t.Fatalf("LookupEntryCompletion() = %v, want nil", err)
		}
		if found {
			t.Fatalf("LookupEntryCompletion() found %#v for a request never applied", recorded)
		}
		if recorded != (execution.CompleteEntryOutcome{}) {
			t.Fatalf("LookupEntryCompletion() miss returned %#v, want the zero outcome", recorded)
		}
		if got := fixture.Snapshot(); got != before {
			t.Fatalf("lookup changed state: before=%#v after=%#v", before, got)
		}
	})

	t.Run("stale-fence-writes-nothing", func(t *testing.T) {
		state := completionRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		service := execution.NewEntryCompletionService(fixture)
		before := fixture.Snapshot()
		command := completionCommand(fixture, state, completionFailedOutcome(), "")
		command.Fence.ClaimToken += "-stale"

		if _, err := service.Complete(context.Background(), command); !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) {
			t.Fatalf("stale fence error = %v, want code %s", err, domainexecution.CodeWorkerFenceStale)
		}
		if got := fixture.Snapshot(); got != before {
			t.Fatalf("stale fence changed state: before=%#v after=%#v", before, got)
		}
	})

	t.Run("stale-observed-state-conflicts-and-writes-nothing", func(t *testing.T) {
		state := completionRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		service := execution.NewEntryCompletionService(fixture)
		before := fixture.Snapshot()
		// 状态仍为 RUNNING 且仍可决策，因此请求会到达适配器，并由 CAS 而非 Core 拒绝。
		drifted := state
		drifted.TerminalIntentRevision++
		command := completionCommand(fixture, drifted, completionFailedOutcome(), "")

		if _, err := service.Complete(context.Background(), command); !fault.IsCode(err, execution.CodeCompleteEntryIdentityConflict) {
			t.Fatalf("stale state error = %v, want code %s", err, execution.CodeCompleteEntryIdentityConflict)
		}
		if got := fixture.Snapshot(); got != before {
			t.Fatalf("stale state changed state: before=%#v after=%#v", before, got)
		}
	})

	t.Run("undecidable-request-leaves-no-trace", func(t *testing.T) {
		state := completionRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		service := execution.NewEntryCompletionService(fixture)
		before := fixture.Snapshot()
		pending := state
		pending.EntryStatus = domainexecution.EntryPending
		command := completionCommand(fixture, pending, completionFailedOutcome(), "")

		if _, err := service.Complete(context.Background(), command); !fault.IsCode(err, execution.CodeEntryCompletionNotRunning) {
			t.Fatalf("undecidable request error = %v, want code %s", err, execution.CodeEntryCompletionNotRunning)
		}
		if got := fixture.Snapshot(); got != before {
			t.Fatalf("undecidable request changed state: before=%#v after=%#v", before, got)
		}
	})

	t.Run("forged-intent-is-refused-and-writes-nothing", func(t *testing.T) {
		for _, test := range []struct {
			name  string
			forge func(execution.CompleteEntryIntent) execution.CompleteEntryIntent
		}{
			{"digest-does-not-match-the-command", func(intent execution.CompleteEntryIntent) execution.CompleteEntryIntent {
				intent.RequestDigest = "sha256:" + strings.Repeat("0", 64)
				return intent
			}},
			{"host-computed-the-next-intent-revision", func(intent execution.CompleteEntryIntent) execution.CompleteEntryIntent {
				intent.Decision.NextIntentRevision++
				return intent
			}},
			{"host-computed-the-next-cancellation-generation", func(intent execution.CompleteEntryIntent) execution.CompleteEntryIntent {
				intent.Decision.NextCancellationGeneration++
				return intent
			}},
			{"host-chose-the-terminal-status", func(intent execution.CompleteEntryIntent) execution.CompleteEntryIntent {
				intent.Decision.EntryStatus = domainexecution.EntryCanceled
				return intent
			}},
		} {
			t.Run(test.name, func(t *testing.T) {
				state := completionRunningState(execution.TerminalIntentNone)
				fixture := factory(t, state)
				before := fixture.Snapshot()
				command := completionCommand(fixture, state, completionFailedOutcome(), "abort-command-1")
				digest, err := execution.CompleteEntryRequestDigest(command)
				if err != nil {
					t.Fatalf("CompleteEntryRequestDigest() = %v, want nil", err)
				}
				intent := test.forge(execution.CompleteEntryIntent{
					EntryID:       fixture.EntryID(),
					RequestDigest: digest,
					Command:       command,
					Decision:      mustCompletionDecision(t, command),
				})

				if _, err := fixture.CompleteEntry(context.Background(), intent); !fault.IsCode(err, execution.CodeCompleteEntryDigestMismatch) {
					t.Fatalf("forged intent error = %v, want code %s", err, execution.CodeCompleteEntryDigestMismatch)
				}
				if got := fixture.Snapshot(); got != before {
					t.Fatalf("forged intent changed state: before=%#v after=%#v", before, got)
				}
			})
		}
	})

	t.Run("faults-roll-back-and-the-retry-equals-one-clean-run", func(t *testing.T) {
		for _, point := range CompletionFaultPoints() {
			t.Run(string(point), func(t *testing.T) {
				state := completionRunningState(execution.TerminalIntentCancel)
				outcome := completionFailedOutcome()

				clean := factory(t, state)
				cleanCommand := completionCommand(clean, state, outcome, "abort-command-1")
				if _, err := execution.NewEntryCompletionService(clean).Complete(context.Background(), cleanCommand); err != nil {
					t.Fatalf("clean completion = %v, want nil", err)
				}
				want := clean.Snapshot()

				faulted := factory(t, state)
				service := execution.NewEntryCompletionService(faulted)
				command := completionCommand(faulted, state, outcome, "abort-command-1")
				before := faulted.Snapshot()

				faulted.SetFault(point)
				if _, err := service.Complete(context.Background(), command); err == nil {
					t.Fatalf("completion faulted at %s returned nil error", point)
				}
				if got := faulted.Snapshot(); got != before {
					t.Fatalf("fault %s changed state: before=%#v after=%#v", point, before, got)
				}

				faulted.ClearFault()
				retried, err := service.Complete(context.Background(), command)
				if err != nil {
					t.Fatalf("retry after %s = %v, want nil", point, err)
				}
				if retried.Status != execution.CompleteEntryApplied {
					t.Fatalf("retry after %s status = %s, want %s", point, retried.Status, execution.CompleteEntryApplied)
				}
				if got := faulted.Snapshot(); got != want {
					t.Fatalf("retry after %s is not equivalent to one clean run: got=%#v want=%#v", point, got, want)
				}

				replayed, err := service.Complete(context.Background(), command)
				if err != nil {
					t.Fatalf("replay after %s = %v, want nil", point, err)
				}
				if replayed.Status != execution.CompleteEntryReplayed {
					t.Fatalf("replay after %s status = %s, want %s", point, replayed.Status, execution.CompleteEntryReplayed)
				}
				if replayed.Decision != retried.Decision {
					t.Fatalf("replay after %s decision = %#v, want %#v", point, replayed.Decision, retried.Decision)
				}
				if got := faulted.Snapshot(); got != want {
					t.Fatalf("replay after %s changed state: before=%#v after=%#v", point, want, got)
				}
			})
		}
	})

	t.Run("concurrent-identical-requests-apply-once", func(t *testing.T) {
		state := completionRunningState(execution.TerminalIntentAbort)
		fixture := factory(t, state)
		service := execution.NewEntryCompletionService(fixture)
		before := fixture.Snapshot()
		command := completionCommand(fixture, state, completionFailedOutcome(), "abort-command-1")
		decision := mustCompletionDecision(t, command)

		const workers = 4
		start := make(chan struct{})
		results := make(chan execution.CompleteEntryOutcome, workers)
		failures := make(chan error, workers)
		var wait sync.WaitGroup
		wait.Add(workers)
		for i := 0; i < workers; i++ {
			go func() {
				defer wait.Done()
				<-start
				result, err := service.Complete(context.Background(), command)
				results <- result
				failures <- err
			}()
		}
		close(start)
		wait.Wait()
		close(results)
		close(failures)

		// 每个调用方都必须成功：相同请求按定义属于重放；宿主若报告冲突，会让工作线程重新读取一个
		// 已经持有答案的入口。
		for err := range failures {
			if err != nil {
				t.Fatalf("concurrent completion = %v, want nil", err)
			}
		}
		applied := 0
		for result := range results {
			switch result.Status {
			case execution.CompleteEntryApplied:
				applied++
			case execution.CompleteEntryReplayed:
			default:
				t.Fatalf("concurrent completion status = %s", result.Status)
			}
			if result.Decision != decision {
				t.Fatalf("concurrent completion decision = %#v, want %#v", result.Decision, decision)
			}
		}
		if applied != 1 {
			t.Fatalf("concurrent completions applied %d times, want 1", applied)
		}
		assertCompletionCommitted(t, before, fixture.Snapshot(), decision)
	})

	t.Run("engine-success-under-a-cancel-intent-commits-succeeded", func(t *testing.T) {
		// 持久化边界的裁决一：引擎已完成，外部副作用已发生，记录必须如实反映。取消意图没有丢失，
		// 而是随下一次修订传递，在那里由 DecideAdvance 真正停止实例。
		state := completionRunningState(execution.TerminalIntentCancel)
		fixture := factory(t, state)
		service := execution.NewEntryCompletionService(fixture)
		before := fixture.Snapshot()
		command := completionCommand(fixture, state, completionSucceededOutcome(), "abort-command-1")

		if _, err := service.Complete(context.Background(), command); err != nil {
			t.Fatalf("Complete() = %v, want nil", err)
		}
		after := fixture.Snapshot()
		if after.EntryStatus != domainexecution.EntrySucceeded {
			t.Fatalf("entry status = %s, want %s", after.EntryStatus, domainexecution.EntrySucceeded)
		}
		if after.TerminalIntent != execution.TerminalIntentCancel {
			t.Fatalf("terminal intent = %s, want the observed %s", after.TerminalIntent, execution.TerminalIntentCancel)
		}
		if after.TerminalIntentRevision != before.TerminalIntentRevision+1 {
			t.Fatalf("terminal intent revision = %d, want %d", after.TerminalIntentRevision, before.TerminalIntentRevision+1)
		}
		if after.CancellationGeneration != before.CancellationGeneration {
			t.Fatalf("cancellation generation = %d, want the unchanged %d", after.CancellationGeneration, before.CancellationGeneration)
		}
	})

	t.Run("abort-command-identity-does-not-change-what-is-committed", func(t *testing.T) {
		// 持久化边界的裁决二：待处理中止命令是幂等和审计身份，因此必须改变请求摘要，不能改变决策。
		state := completionRunningState(execution.TerminalIntentCancel)
		withoutAbort := factory(t, state)
		withAbort := factory(t, state)
		if withoutAbort.Fence() != withAbort.Fence() || withoutAbort.EntryID() != withAbort.EntryID() {
			t.Fatalf("factory returned differing identities: %#v/%s and %#v/%s",
				withoutAbort.Fence(), withoutAbort.EntryID(), withAbort.Fence(), withAbort.EntryID())
		}
		commandWithout := completionCommand(withoutAbort, state, completionFailedOutcome(), "")
		commandWith := completionCommand(withAbort, state, completionFailedOutcome(), "abort-command-1")

		digestWithout, err := execution.CompleteEntryRequestDigest(commandWithout)
		if err != nil {
			t.Fatalf("CompleteEntryRequestDigest() = %v, want nil", err)
		}
		digestWith, err := execution.CompleteEntryRequestDigest(commandWith)
		if err != nil {
			t.Fatalf("CompleteEntryRequestDigest() = %v, want nil", err)
		}
		if digestWithout == digestWith {
			t.Fatal("commands naming different abort commands share a request digest")
		}

		resultWithout, err := execution.NewEntryCompletionService(withoutAbort).Complete(context.Background(), commandWithout)
		if err != nil {
			t.Fatalf("Complete() without a pending abort = %v, want nil", err)
		}
		resultWith, err := execution.NewEntryCompletionService(withAbort).Complete(context.Background(), commandWith)
		if err != nil {
			t.Fatalf("Complete() with a pending abort = %v, want nil", err)
		}
		if resultWithout.Decision != resultWith.Decision {
			t.Fatalf("abort identity changed the decision: %#v and %#v", resultWithout.Decision, resultWith.Decision)
		}
		if withoutAbort.Snapshot() != withAbort.Snapshot() {
			t.Fatalf("abort identity changed what was committed: %#v and %#v", withoutAbort.Snapshot(), withAbort.Snapshot())
		}
	})
}

// assertCompletionCommitted 约束夹具必须提交传入的决策。
//
// 四个状态字段必须精确等于决策，因为它们是 Core 产生且宿主承诺原样写入的值。完成次数必须递增一，
// 这是幂等收据的意义所在。其余计数器只要求不倒退：一次完成写入多少行属于宿主业务，但不能擦除
// 已记录的证据。
func assertCompletionCommitted(t *testing.T, before, after CompletionSnapshot, decision execution.EntryCompletionDecision) {
	t.Helper()
	if after.EntryStatus != decision.EntryStatus {
		t.Fatalf("entry status = %s, want %s", after.EntryStatus, decision.EntryStatus)
	}
	if after.TerminalCause != decision.TerminalCause {
		t.Fatalf("terminal cause = %s, want %s", after.TerminalCause, decision.TerminalCause)
	}
	if after.TerminalIntent != decision.NextIntent {
		t.Fatalf("terminal intent = %s, want %s", after.TerminalIntent, decision.NextIntent)
	}
	if after.TerminalIntentRevision != decision.NextIntentRevision {
		t.Fatalf("terminal intent revision = %d, want %d", after.TerminalIntentRevision, decision.NextIntentRevision)
	}
	if after.CancellationGeneration != decision.NextCancellationGeneration {
		t.Fatalf("cancellation generation = %d, want %d", after.CancellationGeneration, decision.NextCancellationGeneration)
	}
	if after.Completions != before.Completions+1 {
		t.Fatalf("completions = %d, want %d", after.Completions, before.Completions+1)
	}
	if after.TerminalFacts < before.TerminalFacts ||
		after.EvidenceRefs < before.EvidenceRefs ||
		after.GateTerminalizations < before.GateTerminalizations ||
		after.OutboxRecords < before.OutboxRecords {
		t.Fatalf("a completion discarded writes it had already recorded: before=%#v after=%#v", before, after)
	}
}

// completionRunningState 是完成可以开始的唯一状态。修订和 generation 使用不同的非零值，使宿主
// 若错把一个字段返回到另一个字段，能在失败消息中显现。
func completionRunningState(intent execution.TerminalIntent) execution.EntryCompletionState {
	return execution.EntryCompletionState{
		EntryStatus:            domainexecution.EntryRunning,
		TerminalIntent:         intent,
		TerminalIntentRevision: 7,
		CancellationGeneration: 3,
	}
}

// completionCommand 使用夹具身份和给定状态、引擎结果构造入口完成命令。
func completionCommand(fixture CompletionFixture, state execution.EntryCompletionState, outcome execution.EngineOutcome, abortPendingCommandID string) execution.CompleteEntryCommand {
	return execution.CompleteEntryCommand{
		EntryID:               fixture.EntryID(),
		Fence:                 fixture.Fence(),
		State:                 state,
		Outcome:               outcome,
		AbortPendingCommandID: abortPendingCommandID,
	}
}

// completionSucceededOutcome 构造已完成且证据、时间线均成功的运行结果。
func completionSucceededOutcome() execution.EngineOutcome {
	return execution.EngineOutcome{Result: engine.EntryResult{
		ExecutionOutcome: engine.OutcomeSucceeded,
		RecordingOutcome: engine.RecordingSucceeded,
		TimelineOutcome:  engine.TimelineComplete,
	}}
}

// completionFailedOutcome 构造运行失败但证据降级的结果；这种形状最容易暴露宿主将录制质量泄漏到
// 终态的错误。
func completionFailedOutcome() execution.EngineOutcome {
	return execution.EngineOutcome{
		Result: engine.EntryResult{
			ExecutionOutcome: engine.OutcomeFailed,
			RecordingOutcome: engine.RecordingStopFailed,
			TimelineOutcome:  engine.TimelineComplete,
		},
		FailureCode: execution.CodeSchedulingAdapterUnavailable,
	}
}

// mustCompletionDecision 计算套件用于对比宿主的答案。套件自行构造的命令按设计可决策，因此此处
// 失败表示本文件缺陷，而非被测适配器缺陷。
func mustCompletionDecision(t *testing.T, command execution.CompleteEntryCommand) execution.EntryCompletionDecision {
	t.Helper()
	decision, err := execution.DecideEntryCompletion(command.State, command.Outcome)
	if err != nil {
		t.Fatalf("DecideEntryCompletion() = %v, want nil", err)
	}
	return decision
}
