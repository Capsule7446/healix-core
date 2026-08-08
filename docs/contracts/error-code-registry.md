# 业务错误码注册表

本表定义当前稳定的公开业务错误码契约。错误码发布后不可修改、不可复用：所属上下文、`Kind`、含义、允许的安全详情结构和重试语义必须保持不变。`Message` 列按现行协议保留安全英文兜底文案；它不是 i18n key，也不是稳定的文本协议。

## 注册规则

- 格式为带限界上下文前缀的 `UPPER_SNAKE_CASE`。
- `EXECUTION_*` 负责 Node 运行时、执行引擎、调度和执行应用层的失败。
- 每个错误码的允许参数固定为小驼峰、与区域无关且有界的字段；不得携带 secret、selector、环境/参数值、URL、命令载荷、堆栈或 cause。
- 只有聚合输入校验错误允许携带 violation，且顺序必须确定。
- 违规原因码属于共享内核，列在“字段违规码”中；它们是唯一没有上下文前缀的 code，只能作为 `Violation` code，不能作为顶层 `Error` code。
- 生产代码中的未注册 code、重复 code、跨上下文前缀和公开 `errors.New` 哨兵均违反契约。
- [`architecture/fault_contract_guard_test.go`](../../architecture/fault_contract_guard_test.go) 报告 Kind 或 message 不一致时，**先修代码，不修改已发布行**。若确需改变语义，新增错误码而不是改写原行。

## 字段违规码

字段违规码由 `domain/fault` 所有，供所有上下文的聚合校验封套复用。`field` 说明哪个输入失败，code 说明失败原因。词表保持封闭且精简，不为每个字段单独创建 code。

| Code | 失败含义 | 说明 |
|---|---|---|
| `VALIDATION_FIELD_REQUIRED` | 必填输入缺失或为空。 | 补充 `field` 指定的字段。 |
| `VALIDATION_FIELD_INVALID` | 输入值不符合规则。 | 覆盖范围、格式、枚举与顺序规则；拒绝值保持私有。 |
| `VALIDATION_FIELD_DUPLICATE` | 要求唯一的值发生重复。 | `field` 指向后出现项；重复值保持私有。 |
| `VALIDATION_FIELD_MISMATCH` | 输入与所属聚合矛盾。 | 覆盖所有者、父级以及策略和值的不一致。 |

- `field` 是符合 `^[a-z][A-Za-z0-9.]{0,126}$` 的逻辑路径，表示公开契约词汇，不表示内部 struct 字段。
- `field` 中的集合索引从 **0** 开始，对应调用方传入的集合。
- violation 的 `message` 和 `params` 也遵守顶层 code 的安全规则，不携带身份、键、枚举值、cause 或用户输入。
- 一个封套最多携带 `fault.MaxViolations` 条 violation。超过上限时保留确定性前缀、丢弃其余项；消费方不得把数量视为完整清单。

## 执行上下文错误码

| 错误码 | 分类 Kind | 安全文案 Safe message | 允许参数 | 重试与说明 |
|---|---|---|---|---|
| `EXECUTION_ELEMENT_NOT_FOUND` | `NOT_FOUND` | `element was not found` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_OPERATION_CANCELED` | `CANCELED` | `node operation was canceled` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_OPERATION_TIMEOUT` | `DEADLINE_EXCEEDED` | `node operation timed out` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_TRANSIENT_DRIVER` | `UNAVAILABLE` | `node driver is temporarily unavailable` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_OPERATION_FAILED` | `INTERNAL` | `node operation failed` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_STEP_CONFIGURATION_INVALID` | `INVALID_ARGUMENT` | `step configuration is invalid` | 仅允许有序类型化 violations | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_STEP_PHASE_TRANSITION_INVALID` | `FAILED_PRECONDITION` | `step phase transition is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_HEALING_REFUSED` | `FAILED_PRECONDITION` | `healing was refused` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_EVIDENCE_RECORD_FAILED` | `UNAVAILABLE` | `execution evidence could not be recorded` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_STEP_TIMELINE_START_FAILED` | `INTERNAL` | `step timeline start could not be recorded` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_STEP_TIMELINE_FINISH_FAILED` | `INTERNAL` | `step timeline finish could not be recorded` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_NODE_COMPLETION_OBSERVATION_FAILED` | `INTERNAL` | `node completion observation could not be recorded` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_LEAF_COMPLETION_FAILED` | `INTERNAL` | `leaf execution completion failed` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_PLAN_UNSEALED` | `FAILED_PRECONDITION` | `execution plan must be sealed` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_STATUS_TRANSITION_INVALID` | `FAILED_PRECONDITION` | `execution status transition is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_INSTANCE_STATUS_TRANSITION_INVALID` | `FAILED_PRECONDITION` | `instance status transition is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_WORKER_FENCE_STALE` | `CONFLICT` | `worker execution authority is stale` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_WORKER_FENCE_INVALID` | `INVALID_ARGUMENT` | `worker execution authority is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_ENTRY_STATES_INVALID` | `FAILED_PRECONDITION` | `execution entry states are invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_FACT_COMMITTER_REQUIRED` | `FAILED_PRECONDITION` | `execution fact committer is required` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_AUTHORITY_VERIFIER_REQUIRED` | `FAILED_PRECONDITION` | `execution authority verifier is required` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_IDENTITY_MISMATCH` | `FAILED_PRECONDITION` | `execution identity does not match the sealed entry` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_TIMELINE_CONFIGURATION_INVALID` | `FAILED_PRECONDITION` | `execution timeline configuration is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_COMPLETION_CONFIGURATION_INVALID` | `FAILED_PRECONDITION` | `execution completion configuration is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_SCHEDULING_DEPENDENCY_REQUIRED` | `FAILED_PRECONDITION` | `execution scheduling dependency is required` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_RUNTIME_CONFIGURATION_INVALID` | `FAILED_PRECONDITION` | `execution runtime configuration is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_SCHEDULING_ADAPTER_UNAVAILABLE` | `UNAVAILABLE` | `scheduling adapter is unavailable` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_ENTRY_EXECUTOR_CONFIGURATION_INVALID` | `FAILED_PRECONDITION` | `entry executor configuration is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_ENTRY_BROWSER_SESSION_ADAPTER_CONTRACT_VIOLATION` | `INTERNAL` | `browser session adapter returned an invalid outcome` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_STEP_TRANSITION_COMMIT_PAYLOAD_TOO_LARGE` | `OUT_OF_RANGE` | `step transition commit payload is too large` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_STEP_TRANSITION_COMMIT_RUN_MISMATCH` | `FAILED_PRECONDITION` | `step transition commit does not match the claimed run` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_SCHEDULING_CLAIM_INVALID` | `FAILED_PRECONDITION` | `scheduling claim is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_CREATE_INSTANCE_COMMAND_INVALID` | `INVALID_ARGUMENT` | `create-instance command is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_CREATE_INSTANCE_COMMAND_CONFLICT` | `CONFLICT` | `create-instance command conflicts with an existing request` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_CREATE_INSTANCE_SNAPSHOT_CONFLICT` | `CONFLICT` | `create-instance snapshot conflicts with the authoritative instance` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_CREATE_INSTANCE_PLAN_INVALID` | `INVALID_ARGUMENT` | `create-instance plan is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_CREATE_INSTANCE_STEP_SHAPE_INVALID` | `INVALID_ARGUMENT` | `create-instance step shape is invalid` | 仅允许有序类型化 violations | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_CREATE_INSTANCE_SNAPSHOT_INVALID` | `INVALID_ARGUMENT` | `create-instance snapshot is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_ENVIRONMENT_SNAPSHOT_INVALID` | `INVALID_ARGUMENT` | `environment snapshot is invalid` | 仅允许有序类型化 violations | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_CREATE_INSTANCE_ADAPTER_CONTRACT_VIOLATION` | `INTERNAL` | `create-instance adapter returned an invalid authoritative result` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_CREATE_INSTANCE_RETRYABLE` | `UNAVAILABLE` | `create-instance outcome is temporarily unavailable` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_CREATE_INSTANCE_CATALOG_GRAPH_UNRESOLVABLE` | `FAILED_PRECONDITION` | `create-instance catalog graph is unavailable or invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_CANCEL_INSTANCE_COMMAND_INVALID` | `INVALID_ARGUMENT` | `cancel instance command is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_ABORT_INSTANCE_COMMAND_INVALID` | `INVALID_ARGUMENT` | `abort instance command is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_REORDER_QUEUE_COMMAND_INVALID` | `INVALID_ARGUMENT` | `reorder queue command is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_INSTANCE_SIGNAL_RETRYABLE` | `UNAVAILABLE` | `execution cancellation signal must be retried` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_STEP_REVISION_CONFLICT` | `CONFLICT` | `step transition revision conflicts with current state` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_STEP_TRANSITION_COMMIT_IDENTITY_CONFLICT` | `CONFLICT` | `step transition commit identity conflicts with the previously accepted commit` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_INSTANCE_COMMAND_IDENTITY_CONFLICT` | `CONFLICT` | `instance command identity conflicts with an existing request` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_INSTANCE_IDENTITY_CONFLICT` | `CONFLICT` | `instance identity conflicts with the authoritative state` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_INSTANCE_REVISION_CONFLICT` | `CONFLICT` | `instance revision conflicts with current state` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_INSTANCE_STATUS_CONFLICT` | `CONFLICT` | `instance status conflicts with current state` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_QUEUE_REVISION_CONFLICT` | `CONFLICT` | `queue revision conflicts with current state` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_QUEUE_MEMBERSHIP_CONFLICT` | `CONFLICT` | `queue membership conflicts with the authoritative state` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_INSTANCE_COMMAND_ADAPTER_CONTRACT_VIOLATION` | `INTERNAL` | `instance command adapter returned an invalid authoritative result` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_HEAL_GOVERNANCE_SNAPSHOT_INVALID` | `FAILED_PRECONDITION` | `heal governance snapshot is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_HEAL_ACCEPTED_FACT_INVALID` | `INVALID_ARGUMENT` | `accepted heal fact is invalid` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_HEAL_TERMINAL_EFFECT_CONFLICT` | `CONFLICT` | `heal terminal effect conflicts with persisted state` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_ENTRY_COMPLETION_STATE_INVALID` | `INVALID_ARGUMENT` | `entry completion state is invalid` | 仅允许有序类型化 violations | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_ENTRY_COMPLETION_REVISION_EXHAUSTED` | `OUT_OF_RANGE` | `entry completion revision has no representable successor` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_ENTRY_COMPLETION_NOT_RUNNING` | `FAILED_PRECONDITION` | `entry is not running and cannot be completed` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_ENGINE_OUTCOME_INVALID` | `INVALID_ARGUMENT` | `engine outcome is invalid` | 仅允许有序类型化 violations | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_COMPLETE_ENTRY_COMMAND_INVALID` | `INVALID_ARGUMENT` | `complete entry command is invalid` | 仅允许有序类型化 violations | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_COMPLETE_ENTRY_DIGEST_MISMATCH` | `INVALID_ARGUMENT` | `complete entry intent does not match its command` | 仅允许有序类型化 violations | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_COMPLETE_ENTRY_UNAVAILABLE` | `UNAVAILABLE` | `entry completion transaction is unavailable` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_COMPLETE_ENTRY_IDENTITY_CONFLICT` | `CONFLICT` | `entry completion observed a stale state` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_COMPLETE_ENTRY_ADAPTER_CONTRACT_VIOLATION` | `INTERNAL` | `entry completion adapter violated its contract` | 仅允许有序类型化 violations | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_ABORT_REQUEST_INVALID` | `INVALID_ARGUMENT` | `abort request is invalid` | 仅允许有序类型化 violations | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_ABORT_REQUEST_NOT_RUNNING` | `FAILED_PRECONDITION` | `entry is not running and cannot be asked to abort` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_ABORT_REQUEST_ALREADY_ABORTING` | `FAILED_PRECONDITION` | `entry is already aborting` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_REQUEST_ABORT_COMMAND_INVALID` | `INVALID_ARGUMENT` | `abort request command is invalid` | 仅允许有序类型化 violations | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_REQUEST_ABORT_DIGEST_MISMATCH` | `INVALID_ARGUMENT` | `abort request intent does not match its command` | 仅允许有序类型化 violations | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_REQUEST_ABORT_UNAVAILABLE` | `UNAVAILABLE` | `abort request transaction is unavailable` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_REQUEST_ABORT_IDENTITY_CONFLICT` | `CONFLICT` | `entry state changed before the abort request was recorded` | 无 | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |
| `EXECUTION_REQUEST_ABORT_ADAPTER_CONTRACT_VIOLATION` | `INTERNAL` | `abort request adapter violated the port contract` | 仅允许有序类型化 violations | 执行上下文当前分类；身份、输入细节与 cause 仅保留在私有错误链，按该 code 的补救语义处理。 |

## 自动化上下文

| 错误码 | 分类 Kind | 安全文案 Safe message | 允许参数 / violations | 说明 |
|---|---|---|---|---|
| `AUTOMATION_EXECUTION_FLOW_INVALID` | `INVALID_ARGUMENT` | `execution flow input is invalid` | 仅允许有序类型化 violations | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_EXECUTION_FLOW_HISTORY_INVALID` | `FAILED_PRECONDITION` | `execution flow version history is invalid` | 仅允许有序类型化 violations | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_EXECUTION_FLOW_DEPENDENCY_INVALID` | `INVALID_ARGUMENT` | `execution flow dependency resolution is invalid` | 仅允许有序类型化 violations | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_ELEMENT_TARGET_INVALID` | `INVALID_ARGUMENT` | `element target content is invalid` | 仅允许有序类型化 violations | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_ELEMENT_TARGET_HISTORY_INVALID` | `FAILED_PRECONDITION` | `element target version history is invalid` | 仅允许有序类型化 violations | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_FLOW_FRAGMENT_INVALID` | `INVALID_ARGUMENT` | `flow fragment content is invalid` | 仅允许有序类型化 violations | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_FLOW_FRAGMENT_HISTORY_INVALID` | `FAILED_PRECONDITION` | `flow fragment version history is invalid` | 仅允许有序类型化 violations | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_ENVIRONMENT_INVALID` | `INVALID_ARGUMENT` | `environment content is invalid` | 仅允许有序类型化 violations | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_AGGREGATE_TRANSITION_INVALID` | `FAILED_PRECONDITION` | `automation aggregate transition is invalid` | 仅允许有序类型化 violations | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_FOLDER_NOT_FOUND` | `NOT_FOUND` | `automation folder was not found` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_FOLDER_INVALID` | `INVALID_ARGUMENT` | `automation folder is invalid` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_FOLDER_TREE_INVALID` | `FAILED_PRECONDITION` | `automation folder tree is invalid` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_FOLDER_NOT_EMPTY` | `FAILED_PRECONDITION` | `automation folder must be empty` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_AGGREGATE_DELETED` | `FAILED_PRECONDITION` | `automation aggregate has been deleted` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_VERSION_NUMBER_EXHAUSTED` | `RESOURCE_EXHAUSTED` | `automation version number capacity is exhausted` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_PERSISTED_VERSION_NUMBER_INVALID` | `FAILED_PRECONDITION` | `persisted version number must be positive` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_CONFIGURATION_INVALID` | `FAILED_PRECONDITION` | `automation service is not configured` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_CANDIDATE_STALE_BASE` | `CONFLICT` | `heal candidate base version is no longer current` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_REVISION_CONFLICT` | `CONFLICT` | `automation revision conflicts with current state` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_PERSISTED_REVISION_INVALID` | `FAILED_PRECONDITION` | `persisted revision must be non-zero` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_REVISION_EXHAUSTED` | `RESOURCE_EXHAUSTED` | `revision value is exhausted` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_CANDIDATE_IDENTITY_INVALID` | `INVALID_ARGUMENT` | `heal candidate identity is invalid` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_CANDIDATE_STATE_INVALID` | `FAILED_PRECONDITION` | `heal candidate state does not allow this operation` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_CANDIDATE_REVIEW_STATUS_INVALID` | `INVALID_ARGUMENT` | `heal candidate review status is invalid` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_CANDIDATE_REVIEW_COMMAND_INVALID` | `INVALID_ARGUMENT` | `heal candidate review command is invalid` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_APPROVAL_STATUS_INVALID` | `INVALID_ARGUMENT` | `heal approval status is invalid` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_DECISION_BAND_INVALID` | `INVALID_ARGUMENT` | `heal decision band is invalid` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_CONFIDENCE_INVALID` | `INVALID_ARGUMENT` | `heal confidence is invalid` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_STREAK_STATE_INVALID` | `FAILED_PRECONDITION` | `persisted heal streak state is invalid` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_OBSERVATION_INVALID` | `INVALID_ARGUMENT` | `heal observation is invalid` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_SEQUENCE_CONFLICT` | `CONFLICT` | `heal sequence conflicts with persisted ordering` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_PROVENANCE_CONFLICT` | `CONFLICT` | `heal observation conflicts with persisted provenance` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_STREAK_REJECTION_INVALID` | `FAILED_PRECONDITION` | `heal streak cannot be rejected in its current state` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_REVIEW_IDENTITY_CONFLICT` | `CONFLICT` | `heal review command conflicts with an existing request` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_REVIEW_DECISION_CONFLICT` | `FAILED_PRECONDITION` | `heal candidate is no longer available for review` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_REVIEW_AUTHORITY_CONFLICT` | `CONFLICT` | `heal review authority changed before the operation completed` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_HEAL_REVIEW_CONTRACT_VIOLATION` | `INTERNAL` | `heal review could not be completed` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_SAMPLING_PUBLICATION_CONTENT_INVALID` | `INVALID_ARGUMENT` | `sampling publication content is invalid` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_SAMPLING_PUBLICATION_DIGEST_MISMATCH` | `INVALID_ARGUMENT` | `sampling publication digest does not match the request payload` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_SAMPLING_PUBLICATION_UNAVAILABLE` | `UNAVAILABLE` | `sampling publication service is unavailable` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_SAMPLING_PUBLICATION_ADAPTER_CONTRACT_VIOLATION` | `INTERNAL` | `sampling publication adapter returned an invalid outcome` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |
| `AUTOMATION_SAMPLING_PUBLICATION_AUTHORITY_CONFLICT` | `CONFLICT` | `sampling publication authority changed before the publication could be applied` | 无 | 自动化上下文当前分类；聚合身份、版本、载荷与适配器细节不进入公开文本。 |

## 采样、证据、指纹、插值与参数错误码

| 错误码 | 分类 Kind | 安全文案 Safe message | 允许参数 / violations | 说明 |
|---|---|---|---|---|
| `SAMPLING_PUBLICATION_IDENTITY_CONFLICT` | `CONFLICT` | `sampling publication identity conflicts with an existing request` | 无 | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_PUBLICATION_AUTHORITY_INVALID` | `INVALID_ARGUMENT` | `sampling publication authority is invalid` | 无 | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_PUBLICATION_COMMAND_INVALID` | `INVALID_ARGUMENT` | `sampling publication command is invalid` | 无 | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_SESSION_INPUT_INVALID` | `INVALID_ARGUMENT` | `sampling session input is invalid` | 仅允许有序类型化 violations | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_SESSION_STATE_INVALID` | `FAILED_PRECONDITION` | `sampling session state does not allow this operation` | 无 | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_CAPTURE_INVALID` | `INVALID_ARGUMENT` | `sampling capture is invalid` | 仅允许有序类型化 violations | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_DRAFT_INVALID` | `INVALID_ARGUMENT` | `sampling draft is invalid` | 仅允许有序类型化 violations | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_DRAFT_STEP_NOT_FOUND` | `NOT_FOUND` | `sampling draft step was not found` | 无 | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_DRAFT_NODE_NOT_FOUND` | `NOT_FOUND` | `unpublished element target was not found` | 无 | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_DRAFT_NODE_IN_USE` | `FAILED_PRECONDITION` | `unpublished element target is still referenced` | 无 | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_DRAFT_INDEX_OUT_OF_RANGE` | `OUT_OF_RANGE` | `sampling draft index is out of range` | 无 | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_PUBLICATION_MAPPING_INVALID` | `INVALID_ARGUMENT` | `sampling publication mapping is invalid` | 仅允许有序类型化 violations | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_WORKSPACE_INVALID` | `INVALID_ARGUMENT` | `sampling workspace is invalid` | 仅允许有序类型化 violations | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `SAMPLING_INTERNAL` | `INTERNAL` | `sampling operation could not be completed` | 无 | 采样上下文当前分类；工作区身份、映射、载荷与适配器细节不进入公开文本。 |
| `EVIDENCE_STEP_TRANSITION_COMMIT_INVALID` | `INVALID_ARGUMENT` | `step transition commit is invalid` | 仅允许有序类型化 violations | 证据上下文当前分类；事实身份、观测值与载荷细节不进入公开文本。 |
| `EVIDENCE_COMMIT_FACT_LIMIT_EXCEEDED` | `OUT_OF_RANGE` | `step transition commit exceeds its fact limit` | 无 | 证据上下文当前分类；事实身份、观测值与载荷细节不进入公开文本。 |
| `EVIDENCE_STEP_PROGRESS_EVENT_INVALID` | `INVALID_ARGUMENT` | `step progress event is invalid` | 仅允许有序类型化 violations | 证据上下文当前分类；事实身份、观测值与载荷细节不进入公开文本。 |
| `EVIDENCE_STEP_FACT_INVALID` | `INVALID_ARGUMENT` | `step fact is invalid` | 仅允许有序类型化 violations | 证据上下文当前分类；事实身份、观测值与载荷细节不进入公开文本。 |
| `EVIDENCE_HEAL_OBSERVATION_INVALID` | `INVALID_ARGUMENT` | `heal observation is invalid` | 仅允许有序类型化 violations | 证据上下文当前分类；事实身份、观测值与载荷细节不进入公开文本。 |
| `EVIDENCE_VALIDATION_OBSERVATION_INVALID` | `INVALID_ARGUMENT` | `validation observation is invalid` | 仅允许有序类型化 violations | 证据上下文当前分类；事实身份、观测值与载荷细节不进入公开文本。 |
| `EVIDENCE_VALIDATION_GROUP_OBSERVATION_INVALID` | `INVALID_ARGUMENT` | `validation group observation is invalid` | 仅允许有序类型化 violations | 证据上下文当前分类；事实身份、观测值与载荷细节不进入公开文本。 |
| `FINGERPRINT_SELECTOR_INVALID` | `INVALID_ARGUMENT` | `element selector is invalid` | 无 | 指纹上下文当前分类；选择器、框架值与身份细节不进入公开文本。 |
| `FINGERPRINT_ELEMENT_TARGET_SPEC_INVALID` | `INVALID_ARGUMENT` | `element target spec is invalid` | 仅允许有序类型化 violations | 指纹上下文当前分类；选择器、框架值与身份细节不进入公开文本。 |
| `FINGERPRINT_DESCRIPTOR_INVALID` | `INVALID_ARGUMENT` | `element fingerprint descriptor is invalid` | 仅允许有序类型化 violations | 指纹上下文当前分类；选择器、框架值与身份细节不进入公开文本。 |
| `FINGERPRINT_FRAMEWORK_STACK_INVALID` | `INVALID_ARGUMENT` | `framework stack is invalid` | 仅允许有序类型化 violations | 指纹上下文当前分类；选择器、框架值与身份细节不进入公开文本。 |
| `FINGERPRINT_FRAMEWORK_DETECTOR_FAILED` | `INTERNAL` | `framework detection could not be completed` | 无 | 指纹上下文当前分类；选择器、框架值与身份细节不进入公开文本。 |
| `PARAMETER_NAME_INVALID` | `INVALID_ARGUMENT` | `parameter name is invalid` | 无 | 参数上下文当前分类；参数名、类型、值与约束细节不进入公开文本。 |
| `PARAMETER_VALUE_INVALID` | `INVALID_ARGUMENT` | `parameter value is invalid` | 无 | 参数上下文当前分类；参数名、类型、值与约束细节不进入公开文本。 |
| `PARAMETER_CONSTRAINT_UNSATISFIED` | `INVALID_ARGUMENT` | `parameter value does not satisfy its constraint` | 无 | 参数上下文当前分类；参数名、类型、值与约束细节不进入公开文本。 |
| `PARAMETER_BINDING_INVALID` | `INVALID_ARGUMENT` | `parameter binding is invalid` | 无 | 参数上下文当前分类；参数名、类型、值与约束细节不进入公开文本。 |
| `PARAMETER_BINDING_UNRESOLVABLE` | `FAILED_PRECONDITION` | `parameter binding cannot be resolved` | 无 | 参数上下文当前分类；参数名、类型、值与约束细节不进入公开文本。 |
| `INTERPOLATION_RESOLVER_REQUIRED` | `FAILED_PRECONDITION` | `variable resolver is required` | 无 | 插值上下文当前分类；表达式、变量名与展开值不进入公开文本。 |
| `INTERPOLATION_EXPRESSION_INVALID` | `INVALID_ARGUMENT` | `variable expression is invalid` | 无 | 插值上下文当前分类；表达式、变量名与展开值不进入公开文本。 |
| `INTERPOLATION_VARIABLE_UNDEFINED` | `NOT_FOUND` | `referenced variable is not defined` | 无 | 插值上下文当前分类；表达式、变量名与展开值不进入公开文本。 |
| `INTERPOLATION_EXPANSION_TOO_LARGE` | `RESOURCE_EXHAUSTED` | `expanded value exceeds the size limit` | 无 | 插值上下文当前分类；表达式、变量名与展开值不进入公开文本。 |

新增错误码必须与对应生产方的边界变更原子提交，并同时补充 `Kind`、固定安全兜底文案、允许的参数/violation 结构、重试语义、公开消费者和持久化映射。

## 不单独拥有错误码家族的上下文

### `domain/heal`（自愈上下文）

`domain/heal` 不拥有 `HEAL_*` 家族；其内部不跨越 Core 公开边界的普通 error 属于允许的实现错误。只有跨越 Core 公开边界的业务失败才需要注册 code。

已核对其导出错误面的可达性：

| 导出入口 | `domain/heal` 外部调用方 |
|---|---|
| `Assess` | 2 处，均在 `domain/node`（`step.go`、`validation.go`） |
| `Decision.Validate` | 仅 `domain/node`（`step.go`、`validation.go`） |
| `NewDefaultHealerWithPolicy` | 无 |
| `ValidateSamples` | 无 |
| `Thresholds.Validate`, `Weights.Validate` | 包外无调用方 |

这些入口只经 `domain/node` 到达宿主；`domain/node` 负责 `EXECUTION_*` 家族。因此分类应位于 `domain/node` 边界，不另建 Heal 家族，避免同一失败出现两个 code。

以下两项边界约束保持该归属：

- **`domain/node` 在边界分类。** `step.go` 与 `validation.go` 的自愈失败统一经过 `classifyNodeFault`（[`fault_classification.go`](../../domain/node/fault_classification.go)），已有 code 原样透传，其余映射到 `EXECUTION_*`。
- **Heal 文本不得包含观测值。** `domain/heal` 不回显 selector、页面 URL、origin 或 fingerprint；策略阈值属于调用方配置，不是页面内容。

## 执行证据持久化要求

宿主持久化执行事实时必须保存 `fault.Kind` 与 `fault.Code`，并保持以下当前分类语义：

| 事实场景 | Kind | code |
|---|---|---|
| 元素目标缺失 | `NOT_FOUND` | `EXECUTION_ELEMENT_NOT_FOUND` |
| 节点操作超时 | `DEADLINE_EXCEEDED` | `EXECUTION_OPERATION_TIMEOUT` |
| 节点操作被取消 | `CANCELED` | `EXECUTION_OPERATION_CANCELED` |
| Driver 明确声明瞬态不可用 | `UNAVAILABLE` | `EXECUTION_TRANSIENT_DRIVER` |
| 未分类节点操作失败 | `INTERNAL` | `EXECUTION_OPERATION_FAILED` |

Core 只定义当前 `Kind + Code` 事实；SQLite、schema 转换和兼容读取均属于宿主，必须由宿主单独验证。
