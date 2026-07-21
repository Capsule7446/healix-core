# 公开契约

## 所有权

- Domain 类型、领域错误、端口和 Engine Command/Result 由 `healix-core` 所有。
- 官方 go-rod 端口实现由 `github.com/Capsule7446/healix-rod` 所有，Core 不反向依赖适配器。
- Wails View Request/Response、UI 事件、数据库 schema 和技术资源由宿主所有。

## 兼容性

- 导出的类型、字段、方法、错误语义和状态值是公共 API。
- 新字段应保持零值向后兼容；破坏性修改需要 Core 主版本升级。
- Repository/Driver/Recorder/ExecutionFact 端口只表达业务所需最小能力。

## 执行契约

- `CompilePlan` 只消费已封存的 `execution.Plan`，不解析 `LATEST`，不访问 I/O。
- `RunProgram` 每次创建独立 Runtime，只通过注入端口访问浏览器、录屏和事实写入。
- Step 终态、final Validation 和 Heal 事实的原子性由宿主实现 `application/execution.FactCommitter` 保证；所有 Worker 写入必须校验 `WorkerScope.ClaimToken`。
