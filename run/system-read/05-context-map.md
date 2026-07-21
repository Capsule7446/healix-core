# 05 上下文映射

- **Planning → Application**: `CompileExecution` consumes workspace plans and produces an execution Program. This is a published-language relationship through Go structs, with compiler validation as translation.
- **Application → Execution**: `RunProgram` supplies `node.Runtime` dependencies and delegates to the node tree.
- **Execution → Fingerprint**: Node runtime and compiler use `NodeSpec`, selectors, and fingerprints as shared value objects.
- **Execution → Healing**: node invokes `heal.Healer`, `heal.Assess`, and healing policies. Current relationship is direct dependency rather than ACL.
- **Healing → Fingerprint**: scoring compares target/candidate fingerprint values and framework stacks.
- **Execution → Evidence**: node emits `ExecutionSink`, `OperationObserver`, and `HealSampleObserver`; host adapters translate them to workspace evidence.
- **Host browser adapter → Core**: implements Driver/Element and supplies sanitized observations/snapshots. Core remains upstream of adapter contracts.
- **Workspace persistence → Application/host**: workspace ports are outbound contracts; concrete storage is outside this repository.

## G1 门禁

上下文映射解释了主要代码路径。 主要边界弱点在于执行上下文拥有过多面向自愈的词汇，而证据和读侧投影虽有契约，却没有在 Core 中实现具体同步。
