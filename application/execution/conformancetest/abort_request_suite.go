package conformancetest

import (
	"context"
	"testing"

	"github.com/Capsule7446/healix-core/application/execution"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// AbortFaultPoint 标识宿主适配器记录待处理中止意图时可注入故障的位置。
//
// 这些位置对应 AbortRequestTransaction 声明为原子的写入阶段。宿主若在事务外提交其中任一阶段，
// 回滚检查会捕获该问题，而不依赖审阅者阅读适配器实现。空值表示“无故障”，由 ClearFault 恢复。
type AbortFaultPoint string

const (
	// AbortFaultBeforeReplay 在 LookupAbortRequest 内、读取任何内容前失败。宿主若在此之前已让请求
	// 可见，调用方将无法区分新请求与半应用请求。
	AbortFaultBeforeReplay AbortFaultPoint = "BEFORE_REPLAY"
	// AbortFaultAfterDecision 在意图校验完成且首个写入尚未发生时失败。
	AbortFaultAfterDecision AbortFaultPoint = "AFTER_DECISION"
	// AbortFaultAfterIntent 在待处理终态意图已通过 CAS 写入后失败。
	AbortFaultAfterIntent AbortFaultPoint = "AFTER_INTENT"
	// AbortFaultAfterReceipt 在中止命令收据写入后失败；该写入是幂等收据之前的最后一次写入，
	// 也是非原子宿主看起来最像已完成的阶段。
	AbortFaultAfterReceipt AbortFaultPoint = "AFTER_RECEIPT"
)

// AbortFaultPoints 按查询优先、随后提交写入顺序列出套件注入的全部故障点。宿主可调用它驱动自身
// 测试，避免重新声明同一集合。
func AbortFaultPoints() []AbortFaultPoint {
	return []AbortFaultPoint{
		AbortFaultBeforeReplay,
		AbortFaultAfterDecision,
		AbortFaultAfterIntent,
		AbortFaultAfterReceipt,
	}
}

// AbortSnapshot 保存一次中止请求允许改变的全部状态，并作为单个可比较值读回。
//
// EntryStatus 之所以存在，正因为请求绝不能改变它。计数器报告各类写入产生的行数；套件比较这些
// 计数而不固定其数值，因为一次请求产生多少行属于宿主业务，但回滚尝试是否留下任何痕迹不是。
type AbortSnapshot struct {
	EntryStatus            domainexecution.EntryStatus
	TerminalIntent         execution.TerminalIntent
	TerminalIntentRevision int64
	CancellationGeneration int64
	PendingIntents         int
	CommandReceipts        int
	// IdempotencyReceipts 是端口声明为原子的第三类写入，必须读回，因为其他两个计数器无法覆盖两种
	// 情况：事务外提交收据会使崩溃尝试在重试时看似已应用；每次重放都追加收据会令其无限增长，
	// 即使记录的结果保持正确。
	IdempotencyReceipts int
}

// AbortFixture 表示一个待测试的宿主适配器，并补充端口本身未暴露的两项能力：读回已提交状态，
// 以及让适配器在指定位置失败。
//
// SetFault 和 ClearFault 必须可由驱动测试的 goroutine 在没有请求运行时安全调用；套件不会在预期
// 观察故障的调用并发执行期间修改故障设置。
type AbortFixture interface {
	execution.AbortRequestTransaction
	// Fence 返回此夹具接受的工作线程权威。
	Fence() domainexecution.WorkerFence
	// EntryID 返回此夹具持有的入口。
	EntryID() domainexecution.EntryID
	// Snapshot 读回中止请求可能改变的全部状态。
	Snapshot() AbortSnapshot
	// SetFault 在下一次请求的指定位置启用故障。
	SetFault(AbortFaultPoint)
	// ClearFault 清除 SetFault 启用的故障。
	ClearFault()
}

// AbortFactory 在给定状态下创建一个持有运行中入口的新夹具。
//
// 其身份必须是确定的：返回的每个夹具报告相同 Fence 和 EntryID；从相同状态创建的两个夹具必须
// 以相同快照开始。套件从两个夹具构建干净运行和崩溃后重试运行并进行比较，这只有在两项保证成立
// 时才有意义。
type AbortFactory func(t *testing.T, state execution.EntryCompletionState) AbortFixture

// RunAbortRequest 针对一个宿主适配器运行中止请求 conformance 套件。
//
// 它通过 AbortRequestService 调用 AbortRequestTransaction，因为这是访问端口的唯一支持路径；
// 仅在直接驱动端口时通过的宿主，尚未在实际运行路径上得到测试。每个子测试创建自己的夹具，
// 因此失败不会给下一个测试留下残留状态。
func RunAbortRequest(t *testing.T, factory AbortFactory) {
	t.Helper()

	t.Run("applies-once-then-replays", func(t *testing.T) {
		state := abortRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		service := execution.NewAbortRequestService(fixture)
		command := abortCommand(fixture, state, "command-1")
		decision := mustAbortDecision(t, command)

		applied, err := service.Request(context.Background(), command)
		if err != nil {
			t.Fatalf("Request() = %v, want nil", err)
		}
		if applied.Status != execution.RequestAbortApplied {
			t.Fatalf("Request() status = %s, want %s", applied.Status, execution.RequestAbortApplied)
		}
		if applied.Decision != decision {
			t.Fatalf("Request() decision = %+v, want %+v", applied.Decision, decision)
		}
		after := fixture.Snapshot()
		assertAbortRecorded(t, after, decision)

		replayed, err := service.Request(context.Background(), command)
		if err != nil {
			t.Fatalf("replayed Request() = %v, want nil", err)
		}
		if replayed.Status != execution.RequestAbortReplayed {
			t.Fatalf("replayed status = %s, want %s", replayed.Status, execution.RequestAbortReplayed)
		}
		if replayed.Decision != decision {
			t.Fatalf("replayed decision = %+v, want the recorded %+v", replayed.Decision, decision)
		}
		if again := fixture.Snapshot(); again != after {
			t.Fatalf("replay changed committed state: %+v -> %+v", after, again)
		}
	})

	t.Run("request-leaves-the-entry-running", func(t *testing.T) {
		// 这正是 D-17 与 D-12 分开成为独立契约的全部原因。适配器若在此处终止入口，会给实例两条
		// 可能不一致的终态写入路径，并使完成操作的权威 CAS 停留在已不匹配的行上。
		state := abortRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		before := fixture.Snapshot()
		command := abortCommand(fixture, state, "command-1")

		if _, err := execution.NewAbortRequestService(fixture).Request(context.Background(), command); err != nil {
			t.Fatalf("Request() = %v, want nil", err)
		}
		after := fixture.Snapshot()
		if after.EntryStatus != before.EntryStatus {
			t.Fatalf("entry status moved from %q to %q; an abort request records intent and ends nothing", before.EntryStatus, after.EntryStatus)
		}
		if after.EntryStatus != domainexecution.EntryRunning {
			t.Fatalf("entry status = %q, want it still %q", after.EntryStatus, domainexecution.EntryRunning)
		}
		if after.CancellationGeneration != before.CancellationGeneration {
			t.Fatalf("cancellation generation moved from %d to %d; a request does not spend one", before.CancellationGeneration, after.CancellationGeneration)
		}
	})

	t.Run("escalates-a-cancel-and-refuses-a-second-abort", func(t *testing.T) {
		escalating := abortRunningState(execution.TerminalIntentCancel)
		fixture := factory(t, escalating)
		command := abortCommand(fixture, escalating, "command-1")
		applied, err := execution.NewAbortRequestService(fixture).Request(context.Background(), command)
		if err != nil {
			t.Fatalf("escalating Request() = %v, want nil", err)
		}
		if applied.Decision.NextIntent != execution.TerminalIntentAbort {
			t.Fatalf("next intent = %q, want %q", applied.Decision.NextIntent, execution.TerminalIntentAbort)
		}

		aborting := abortRunningState(execution.TerminalIntentAbort)
		repeat := factory(t, aborting)
		before := repeat.Snapshot()
		_, err = execution.NewAbortRequestService(repeat).Request(context.Background(), abortCommand(repeat, aborting, "command-2"))
		if !fault.IsCode(err, execution.CodeAbortRequestAlreadyAborting) {
			t.Fatalf("error = %v, want %s", err, execution.CodeAbortRequestAlreadyAborting)
		}
		if after := repeat.Snapshot(); after != before {
			t.Fatalf("refused request changed committed state: %+v -> %+v", before, after)
		}
	})

	t.Run("faults-roll-back-all-observable-state", func(t *testing.T) {
		for _, point := range AbortFaultPoints() {
			t.Run(string(point), func(t *testing.T) {
				state := abortRunningState(execution.TerminalIntentNone)
				fixture := factory(t, state)
				service := execution.NewAbortRequestService(fixture)
				command := abortCommand(fixture, state, "command-1")
				decision := mustAbortDecision(t, command)
				before := fixture.Snapshot()

				fixture.SetFault(point)
				if _, err := service.Request(context.Background(), command); err == nil {
					t.Fatalf("Request() succeeded with a fault armed at %s", point)
				}
				if crashed := fixture.Snapshot(); crashed != before {
					t.Fatalf("fault at %s left state behind: %+v -> %+v", point, before, crashed)
				}

				// 崩溃后的重试必须与首次干净尝试不可区分：决策相同，已提交状态相同。
				fixture.ClearFault()
				retried, err := service.Request(context.Background(), command)
				if err != nil {
					t.Fatalf("retry after %s = %v, want nil", point, err)
				}
				if retried.Status != execution.RequestAbortApplied && retried.Status != execution.RequestAbortReplayed {
					t.Fatalf("retry after %s status = %s", point, retried.Status)
				}
				if retried.Decision != decision {
					t.Fatalf("retry after %s decision = %+v, want %+v", point, retried.Decision, decision)
				}
				assertAbortRecorded(t, fixture.Snapshot(), decision)
			})
		}
	})

	// 接下来两个子测试有意分开。过期 fence 和过期观测状态都表示“其他人先移动了状态”，但调用方
	// 的处理相反：领取失效意味着停止，状态变化意味着重新读取并重建。适配器若以同一个错误码或
	// 未分类存储错误回答二者，宿主将无法区分，因此每个测试都断言各自错误码，而不只是“有错误”。
	t.Run("stale-fence-writes-nothing", func(t *testing.T) {
		state := abortRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		before := fixture.Snapshot()
		stale := abortCommand(fixture, state, "command-1")
		stale.Fence.ClaimToken += "-stale"

		_, err := execution.NewAbortRequestService(fixture).Request(context.Background(), stale)
		if !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) {
			t.Fatalf("stale fence error = %v, want code %s", err, domainexecution.CodeWorkerFenceStale)
		}
		if after := fixture.Snapshot(); after != before {
			t.Fatalf("stale fence changed committed state: %+v -> %+v", before, after)
		}
	})

	t.Run("stale-observed-state-conflicts-and-writes-nothing", func(t *testing.T) {
		state := abortRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		before := fixture.Snapshot()
		// 命令形状有效且 fence 仍然当前；它只是观测到了入口已经越过的修订。
		stale := abortCommand(fixture, state, "command-1")
		stale.State.TerminalIntentRevision = state.TerminalIntentRevision + 1

		_, err := execution.NewAbortRequestService(fixture).Request(context.Background(), stale)
		if !fault.IsCode(err, execution.CodeRequestAbortIdentityConflict) {
			t.Fatalf("stale state error = %v, want code %s", err, execution.CodeRequestAbortIdentityConflict)
		}
		if after := fixture.Snapshot(); after != before {
			t.Fatalf("stale state changed committed state: %+v -> %+v", before, after)
		}
	})
}

// assertAbortRecorded 断言已提交请求忠实记录了决策 Core 产生的值：两个计数器必须原样等于 Next*
// 对，且无论尝试多少次，每一类写入都必须恰好存在一行。
func assertAbortRecorded(t *testing.T, snapshot AbortSnapshot, decision execution.AbortRequestDecision) {
	t.Helper()
	if snapshot.TerminalIntent != decision.NextIntent {
		t.Fatalf("terminal intent = %q, want %q", snapshot.TerminalIntent, decision.NextIntent)
	}
	if snapshot.TerminalIntentRevision != decision.NextIntentRevision {
		t.Fatalf("intent revision = %d, want %d", snapshot.TerminalIntentRevision, decision.NextIntentRevision)
	}
	if snapshot.CancellationGeneration != decision.NextCancellationGeneration {
		t.Fatalf("cancellation generation = %d, want %d", snapshot.CancellationGeneration, decision.NextCancellationGeneration)
	}
	if snapshot.PendingIntents != 1 {
		t.Fatalf("pending intents = %d, want exactly 1", snapshot.PendingIntents)
	}
	if snapshot.CommandReceipts != 1 {
		t.Fatalf("command receipts = %d, want exactly 1", snapshot.CommandReceipts)
	}
	if snapshot.IdempotencyReceipts != 1 {
		t.Fatalf("idempotency receipts = %d, want exactly 1 however many attempts it took", snapshot.IdempotencyReceipts)
	}
}

// abortRunningState 构造给定终态意图、初始修订为 1 且 generation 为 0 的运行中状态。
func abortRunningState(intent execution.TerminalIntent) execution.EntryCompletionState {
	return execution.EntryCompletionState{
		EntryStatus:            domainexecution.EntryRunning,
		TerminalIntent:         intent,
		TerminalIntentRevision: 1,
		CancellationGeneration: 0,
	}
}

// abortCommand 使用夹具身份和给定观测状态构造中止请求命令。
func abortCommand(fixture AbortFixture, state execution.EntryCompletionState, commandID string) execution.RequestAbortCommand {
	return execution.RequestAbortCommand{
		EntryID: fixture.EntryID(),
		Fence:   fixture.Fence(),
		State:   state,
		Request: execution.AbortRequest{AbortPendingCommandID: commandID},
	}
}

// mustAbortDecision 计算中止决策；失败时立即终止测试。
func mustAbortDecision(t *testing.T, command execution.RequestAbortCommand) execution.AbortRequestDecision {
	t.Helper()
	decision, err := execution.DecideAbortRequest(command.State, command.Request)
	if err != nil {
		t.Fatalf("DecideAbortRequest() = %v", err)
	}
	return decision
}
