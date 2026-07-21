# 10 业务视图

Core 不拥有业务读模型。消费项目基于 Automation 资产和不可变 Evidence 事实建立自己的投影、查询 DTO、缓存与 API。

| 视图 | 消费者 | 建议事实来源 | 所有者 |
|---|---|---|---|
| Node / Workflow / TestTask 列表与详情 | authoring UI/API | Automation 聚合事件或持久化资产 | 消费项目 |
| Run dashboard | monitoring UI | Execution Run/Entry 状态事实 | 消费项目 |
| Execution timeline | monitor/replay UI | Evidence progress 与原子 terminal commits | 消费项目 |
| Healing review | review UI | durable heal candidate 与 Evidence observations | 消费项目 |
| Heal quality report | analytics UI | immutable Evidence observations | 消费项目 |
| Framework diagnostics | debugging UI | sanitized sampling/fingerprint observations | 消费项目 |

Core 不提供 `NodeQueryResult`、`WorkflowQueryResult`、`TestTaskQueryResult`、`Dashboard`、`ExecutionDetail` 或 `metrics.Query`。这些已删除的类型不能作为集成契约。凭据值只允许在执行期通过 Application credential boundary 解析，不能进入投影、日志或 Evidence。
