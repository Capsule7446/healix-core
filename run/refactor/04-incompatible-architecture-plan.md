# 不兼容后端重构执行计划

## 目标架构

```text
domain/
  workspace/      资产、版本、发布不变量
  execution/      运行领域状态与执行计划值对象
  evidence/       验证、自愈、网络和步骤事实
  fingerprint/    目标身份共享内核
  heal/           候选评分与安全决策
  node/           可执行树和浏览器执行状态

application/
  engine/         编译和运行编排
  execution/      执行端口、用例和协调器
  readmodel/      UI/API 查询 DTO 与 Reader
```

## 已完成

1. 查询 DTO 从 `domain/automation` 移至 `application/readmodel`。
2. 旧 QueryResult、Dashboard 和 WorkspaceReader 组合接口删除。
3. 应用层新增执行事实提交、进度写入和运行生命周期端口。
4. 旧执行路径仍由 `domain/automation` 类型支撑，保证当前工程可编译和可回滚。

## 后续不兼容切片

1. 将 `sealed execution plan`、`WorkflowExecutionPlan`、依赖快照迁移至 `domain/execution`，由编译器改用新包。
2. 将执行事实类型迁移至 `domain/evidence`，应用端口改用 evidence 契约。
3. 将运行状态和运行生命周期规则从 workspace 资产包迁移至 `domain/execution`。
4. 将 workspace ports 收缩为资产写入和发布端口；执行/证据端口只保留在 application/evidence 边界。
5. 将 EnvironmentSnapshot 拆成 UI-safe descriptor 与 execution-only credential reference。
6. 将 node/heal 交互收缩为 node-local HealingPort，移除 Node 对完整 heal 决策实现的依赖。
7. 删除 workspace 中已迁出的执行/证据类型和兼容别名。

## 门禁

每个切片必须通过：

- `go test -race ./...`
- `go vet ./...`
- 架构依赖测试
- 旧行为矩阵测试迁移后的等价版本
- 不允许 UI/API DTO 含凭据
- 不允许 Runtime 成为读侧查询源

## 当前状态

已完成查询读侧和应用端口边界；下一片是执行计划迁移。该迁移会产生不兼容包路径变化，必须一次性更新 Core 内部消费者和测试。
