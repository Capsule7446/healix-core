# C17 — 多 Workflow 顺序执行

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：决策层已实现；Host 必须原子 claim。**

## 业务不变量

Entry 是 TestTask 直接编排的顶层 Workflow 执行入口，同时也是浏览器会话隔离边界。Entry 严格按 sealed plan 顺序串行；每个 Entry 启动时由 Host 创建全新的浏览器实例/上下文，Entry 结束后必须关闭。同一 Entry 内的根 Workflow 与嵌套 WorkflowRef 共享该浏览器；不同 Entry 不共享 cookies、LocalStorage、SessionStorage、IndexedDB、页面、登录态或其他浏览器会话状态。重复 Workflow 由 ExecutionID 区分。

## 当前证据

- `application/scheduling/decision.go`：`DecideAdvance`、serial shape
- `application/scheduling/coordinator.go`
- `domain/execution/plan.go`：ordered entries
- `application/engine/compiler.go`：occurrence-specific entries

## 调整清单

- [x] plan order 唯一权威。
- [x] state input 按 ExecutionID 归一化。
- [x] malformed serial shape 拒绝。
- [x] Host 对 next execution 采用 CAS/fence claim。
- [x] 定义 Entry browser-session lifecycle port/contract：claim 成功后创建全新 browser，Entry terminal 后关闭，再允许启动下一 Entry。
- [x] 浏览器创建失败时 Entry 明确失败且不得启动 Workflow；关闭失败需记录并完成隔离清理，不能复用污染会话。
- [x] 禁止跨 Entry 复用 browser/profile/user-data-dir；cookies、LocalStorage、SessionStorage、IndexedDB、cache、页面和登录态均不得继承。
- [x] 同一 Entry 的嵌套 WorkflowRef 必须复用该 Entry 的 browser session，不能为每个子 Workflow 重开浏览器。
- [x] 明确 sealed array order 与 SequenceNumber 一致性。
- [x] repeated Workflow 不得以 WorkflowID 作为 execution identity。
- [x] 增加并发 scheduler adapter tests。

## 测试与验收

- [x] 两 scheduler 竞争仅一个开始 frontier entry。
- [x] 每个 Entry 获得不同 browser session identity；前一 Entry 的 cookies/LocalStorage/SessionStorage/IndexedDB/登录态在下一 Entry 不可见。
- [x] Entry 内根 Workflow 与多层嵌套 WorkflowRef 使用同一 browser session。
- [x] Entry 成功、失败或 abort 后均执行关闭；关闭完成或隔离确认前不得启动下一 Entry。
- [x] browser 创建失败不执行任何 Workflow step，且 Entry/Run 按 failure policy 进入确定状态。
- [x] running entry 阻止后续。
- [x] success 后准确选择下一 entry。
- [x] 相同 Workflow 多 occurrence 独立。
- [x] stale worker lease 不能提交。

## 依赖与风险

纯决策正确不等于并发安全；Host CAS/fencing 是验收必要条件。

## 审核

- [x] 批准保留现有设计
- [x] 修改：________________
