# 创建执行实例与冻结执行计划

## 目标

`CreateInstanceService.CreateInstance` 在一个事务中解析并冻结一次完整执行输入：TestTask 版本、fixed/latest workflow 依赖、typed parameters、invocation bindings、Environment Variables、失败/截图/修复策略，以及串行 entry identities。输出是带摘要的不可变 `execution.InstanceSnapshot` 和 `QUEUED` 执行实例。

## 输入

- `CreateInstanceCommand`：包含 command/instance/task/environment identity、创建时间和策略。
- `Entries map[itemID]map[name]parameter.Value`：每个顶层 TestTask item 的 typed 参数值。
- `CreateInstanceStore`：在 transaction 内提供幂等命令查询、资产解析和原子插入。

## 冻结流程

1. 复制并规范化 command，校验资源上限及 typed values。
2. 计算稳定 request digest；相同 command ID + 相同 digest 返回既有结果，不同 digest 返回冲突。
3. 解析指定 TestTask version、Environment revision/Variables 和所有 workflow dependencies。
4. 对 `LATEST` 项和嵌套引用读取当时 current published version，并把解析结果写入 snapshot；此后执行、重试均不得重新解析 latest。
5. 校验 parameter declarations、required/type/options、parent bindings 和 invocation graph。
6. `SealInstanceSnapshot` 深拷贝并计算 canonical digest。
7. 在同一事务中插入执行实例、snapshot、entries 和 command 幂等结果。

## 不变量

- `EnvironmentSnapshot` 只有 identity、revision、base URL 和带类型的 `Variables map[string]parameter.Value`（V1 快照另带字符串 `Properties`）；没有凭据引用或 secret。Engine 将这些值只读注入 `env.`。
- 参数使用 `domain/parameter.Value`（text、number、boolean、single-select、multi-select）及 typed `Binding`；不得字符串化或在执行时回源。
- 每个顶层 TestTask item 生成独立 entry identity。Execution 随后为每个 entry 获取独立浏览器；该 entry 内的嵌套 workflow 共享它。
- persisted snapshot 必须能通过 stored digest hydrate；adapter 返回的 identity、digest 或 entry 顺序不一致属于 contract error。

## 边界

Scheduling 只拥有执行实例创建、队列/claim 和状态推进。它不打开浏览器、不执行 Program、不提交 Evidence。Engine 只消费冻结 snapshot；Execution 协调浏览器和 fenced Evidence 写入。

## 源码与测试

- 源码：[`application/scheduling/create_instance_service.go`](../../../application/scheduling/create_instance_service.go)、[`application/scheduling/create_instance_builder.go`](../../../application/scheduling/create_instance_builder.go)
- 快照：[`domain/execution/instance_snapshot.go`](../../../domain/execution/instance_snapshot.go)
- 测试：[`application/scheduling/create_instance_test.go`](../../../application/scheduling/create_instance_test.go)、[`application/scheduling/create_instance_transaction_test.go`](../../../application/scheduling/create_instance_transaction_test.go)
