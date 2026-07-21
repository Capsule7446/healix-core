# 11 读侧适配评估

| 视图 | 适配评估 | 缺失契约 |
|---|---|---|
| Workflow/test-task queries | Reasonable port boundary | pagination, ordering, projection version/freshness |
| Run timeline | Evidence structs can support it | event identity, ordering key, correlation, retention |
| Healing review | Candidate evidence is a good replay base | query aggregation, review state, authorization boundary |
| Framework diagnostics | Fingerprint values are safe normalized inputs | stored observation provenance and refresh semantics |
| Sampling/evidence | Workspace models are reusable | projection ownership and idempotency key |

当前读侧设计是规范不足，而不是过度设计。 这是评估结论，不构成当前领域核心的阻塞项。
