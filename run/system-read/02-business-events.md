# 02 业务事件

- **执行已规划**: workspace plan is compiled into a Program and indexed NodeSpecs.
- **运行已开始**: engine validates configuration, snapshots variables, and optionally starts recording.
- **节点阶段已转换**: Runtime emits phase events under guarded transitions.
- **操作已观测**: browser operation outcome, timing, attempt, and error kind may be emitted.
- **元素缺失**: Driver exhausts selectors and returns the explicit not-found signal.
- **已提出自愈方案**: Healer returns candidates, outcome, scores, and replay samples.
- **已完成自愈评估**: Safety policy decides allow, review, or block.
- **已应用自愈**: a candidate is re-located, decision is recorded, then run-local selector overlay is installed.
- **已观测验证结果**: polling records actual/expected outcome and reason, with sensitive values masked.
- **Run 完成d/failed/canceled**: root result and Recorder stop outcome determine final application result.
- **证据已提交**: host sink may project events, observations, and samples into workspace evidence.

这些是根据契约和测试推断出的领域相关转换；由于存储位于外部，它们不构成有持久性保证的事件流。
