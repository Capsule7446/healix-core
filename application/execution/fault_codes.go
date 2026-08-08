package execution

import "github.com/Capsule7446/healix-core/domain/fault"

// 本包发布的每个 fault.Code 都在此声明，这是 AGENTS.md 对每个包的要求，也是七个领域包当前采用的做法。
//
// 单一入口正是目的所在。错误码属于公共兼容契约：只能新增或墓碑化，不得重命名或复用；审计者检查
// 这一规则时必须能一次看到完整集合。错误码若分散在各功能旁，“全部错误码”就意味着搜索七个文件，
// 并信任搜索结果。
//
// 各区块按产生错误码的功能分组，并保留其 Kind 依据，因为错误码承诺的处理方式决定了调用方如何选择；
// 详见 docs/contracts/error-code-registry.md，architecture/fault_contract_guard_test.go 会逐行将其
// 与这些声明比对。

// --- ports.go ---

const (
	// CodeFactCommitterRequired 表示提交执行事实所需的提交器缺失。
	CodeFactCommitterRequired fault.Code = "EXECUTION_FACT_COMMITTER_REQUIRED"
	// CodeStepRevisionConflict 表示步骤转换修订与当前状态冲突。
	CodeStepRevisionConflict fault.Code = "EXECUTION_STEP_REVISION_CONFLICT"
	// CodeCommitIdentityConflict 表示步骤转换提交身份与已接受提交冲突。
	CodeCommitIdentityConflict fault.Code = "EXECUTION_STEP_TRANSITION_COMMIT_IDENTITY_CONFLICT"
	// CodeStepTransitionCommitPayloadTooLarge 同时表示总负载预算超限或单个字符串超过自身字节上限；
	// 处理方式始终是缩小提交，而不是纠正某个字段的值。
	CodeStepTransitionCommitPayloadTooLarge fault.Code = "EXECUTION_STEP_TRANSITION_COMMIT_PAYLOAD_TOO_LARGE"
	// CodeStepTransitionCommitRunMismatch 表示提交事实自身的 InstanceID 与工作线程 fence 的 InstanceID
	// 不一致。该错误属于 FAILED_PRECONDITION：调用方必须重新读取权威领取，而不是提供另一个值。
	CodeStepTransitionCommitRunMismatch fault.Code = "EXECUTION_STEP_TRANSITION_COMMIT_RUN_MISMATCH"
)

// --- entry_completion_transaction.go ---

const (
	// CodeCompleteEntryCommandInvalid 表示 Core 无法生成摘要的完成请求：身份、fence、观测状态或引擎
	// 结果形状错误。调用方必须纠正请求，因此分类为 INVALID_ARGUMENT。
	CodeCompleteEntryCommandInvalid fault.Code = "EXECUTION_COMPLETE_ENTRY_COMMAND_INVALID"
	// CodeCompleteEntryDigestMismatch 表示意图摘要或决策并非由其命令推导而来；它阻止宿主替换 Core
	// 从未产生的决策。
	CodeCompleteEntryDigestMismatch fault.Code = "EXECUTION_COMPLETE_ENTRY_DIGEST_MISMATCH"
	// CodeCompleteEntryUnavailable 表示服务没有接入事务。请求本身没有错误，因此分类为 UNAVAILABLE
	// 而非 INVALID_ARGUMENT。
	CodeCompleteEntryUnavailable fault.Code = "EXECUTION_COMPLETE_ENTRY_UNAVAILABLE"
	// CodeCompleteEntryAdapterContractViolation 表示适配器返回端口禁止的结果：未知状态、不同身份或
	// 适配器自行重新计算的决策。这是适配器实现缺陷而非业务失败，因此分类为 INTERNAL。
	CodeCompleteEntryAdapterContractViolation fault.Code = "EXECUTION_COMPLETE_ENTRY_ADAPTER_CONTRACT_VIOLATION"
	// CodeCompleteEntryIdentityConflict 是适配器在入口不再持有命令声称观测的状态时返回的错误码，
	// 表示 CAS 发现其他写入者已先移动状态。
	CodeCompleteEntryIdentityConflict fault.Code = "EXECUTION_COMPLETE_ENTRY_IDENTITY_CONFLICT"
)

// --- abort_request_transaction.go ---

const (
	// CodeRequestAbortCommandInvalid 表示 Core 无法生成摘要的中止请求：身份、fence、观测状态或请求
	// 形状错误。调用方必须纠正请求，因此分类为 INVALID_ARGUMENT。
	CodeRequestAbortCommandInvalid fault.Code = "EXECUTION_REQUEST_ABORT_COMMAND_INVALID"
	// CodeRequestAbortDigestMismatch 表示意图摘要或决策并非由其命令推导而来；它阻止宿主替换 Core
	// 从未产生的计数器。
	CodeRequestAbortDigestMismatch fault.Code = "EXECUTION_REQUEST_ABORT_DIGEST_MISMATCH"
	// CodeRequestAbortUnavailable 表示服务没有接入事务。请求本身没有错误，因此分类为 UNAVAILABLE
	// 而非 INVALID_ARGUMENT。
	CodeRequestAbortUnavailable fault.Code = "EXECUTION_REQUEST_ABORT_UNAVAILABLE"
	// CodeRequestAbortAdapterContractViolation 表示适配器返回端口禁止的结果：未知状态、不同身份或
	// 适配器自行重新计算的决策。这是适配器实现缺陷而非业务失败，因此分类为 INTERNAL。
	CodeRequestAbortAdapterContractViolation fault.Code = "EXECUTION_REQUEST_ABORT_ADAPTER_CONTRACT_VIOLATION"
	// CodeRequestAbortIdentityConflict 是适配器在入口不再持有命令声称观测的状态时返回的错误码，
	// 表示 CAS 发现其他写入者已先移动状态。
	CodeRequestAbortIdentityConflict fault.Code = "EXECUTION_REQUEST_ABORT_IDENTITY_CONFLICT"
)

// --- entry_completion.go ---

const (
	// CodeEntryCompletionStateInvalid 表示观测状态形状错误：未知入口状态或终态意图，或负修订。调用
	// 方读取了不属于 Core 词汇的值，应纠正读取结果，因此分类为 INVALID_ARGUMENT。
	CodeEntryCompletionStateInvalid fault.Code = "EXECUTION_ENTRY_COMPLETION_STATE_INVALID"
	// CodeEntryCompletionRevisionExhausted 表示 Core 拒绝写入后继修订的状态。参数形状没有错误，只是
	// 计数器没有剩余空间，因此分类为 OUT_OF_RANGE。
	CodeEntryCompletionRevisionExhausted fault.Code = "EXECUTION_ENTRY_COMPLETION_REVISION_EXHAUSTED"
	// CodeEntryCompletionNotRunning 表示形状有效但不是 RUNNING 的状态。只有运行中入口可完成，调用方
	// 必须重新读取后重试，因此分类为 FAILED_PRECONDITION。
	CodeEntryCompletionNotRunning fault.Code = "EXECUTION_ENTRY_COMPLETION_NOT_RUNNING"
	// CodeEngineOutcomeInvalid 表示引擎结果超出引擎词汇，或错误码存在但为空白。
	CodeEngineOutcomeInvalid fault.Code = "EXECUTION_ENGINE_OUTCOME_INVALID"
)

// --- heal_governance.go ---

const (
	// CodeHealGovernanceSnapshotInvalid 表示自愈治理快照无效。
	CodeHealGovernanceSnapshotInvalid fault.Code = "EXECUTION_HEAL_GOVERNANCE_SNAPSHOT_INVALID"
	// CodeHealAcceptedFactInvalid 表示已接受的自愈事实无效。
	CodeHealAcceptedFactInvalid fault.Code = "EXECUTION_HEAL_ACCEPTED_FACT_INVALID"
	// CodeHealTerminalEffectConflict 表示自愈终态效果发生冲突。
	CodeHealTerminalEffectConflict fault.Code = "EXECUTION_HEAL_TERMINAL_EFFECT_CONFLICT"
)

// --- entry_executor.go ---

const (
	// CodeEntryExecutorConfigurationInvalid 表示 NewEntryExecutor 的构造检查失败：这些值属于执行器
	// 自身配置而非独立调用参数，必须在构造前纠正配置，因此分类为 FAILED_PRECONDITION。
	CodeEntryExecutorConfigurationInvalid fault.Code = "EXECUTION_ENTRY_EXECUTOR_CONFIGURATION_INVALID"
	// CodeSchedulingAdapterUnavailable 表示浏览器会话工厂失败：需要恢复可达的是宿主适配器，而非调用方。
	CodeSchedulingAdapterUnavailable fault.Code = "EXECUTION_SCHEDULING_ADAPTER_UNAVAILABLE"
	// CodeEntryBrowserSessionAdapterContractViolation 表示宿主工厂返回 nil 或无效会话：工厂自身违反
	// 契约，调用方无法通过修改请求解决。
	CodeEntryBrowserSessionAdapterContractViolation fault.Code = "EXECUTION_ENTRY_BROWSER_SESSION_ADAPTER_CONTRACT_VIOLATION"
)

// --- abort_request.go ---

const (
	// CodeAbortRequestInvalid 表示请求形状错误：待处理中止命令 ID 缺失、为空白或带首尾空格。该身份
	// 原样进入宿主幂等行，无法追溯到命令的值应由调用方纠正，因此分类为 INVALID_ARGUMENT。
	CodeAbortRequestInvalid fault.Code = "EXECUTION_ABORT_REQUEST_INVALID"
	// CodeAbortRequestNotRunning 表示形状有效但不是 RUNNING 的状态。只有运行中入口可请求停止；其他
	// 状态已达到终态或尚未开始，调用方必须重新读取后重试，因此分类为 FAILED_PRECONDITION。
	//
	// 它与 CodeEntryCompletionNotRunning 是不同错误码，因为处理方式不同：迟到的完成通常是待查询的
	// 重放，而针对已完成入口的中止请求没有可执行的动作。
	CodeAbortRequestNotRunning fault.Code = "EXECUTION_ABORT_REQUEST_NOT_RUNNING"
	// CodeAbortRequestAlreadyAborting 表示入口意图已经是 ABORT 的请求。中止正在进行且没有可推进的
	// 内容，因此分类为 FAILED_PRECONDITION，而不是静默成功。
	CodeAbortRequestAlreadyAborting fault.Code = "EXECUTION_ABORT_REQUEST_ALREADY_ABORTING"
)
