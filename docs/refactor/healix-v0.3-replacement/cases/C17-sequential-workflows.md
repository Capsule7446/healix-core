# C17 — 多工作流顺序执行

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：`EntryExecutor` 的直接生命周期行为已有测试覆盖；持久化/失败策略集成与浏览器隔离一致性仍待宿主适配器完成。以下证据、清单与验收项按当前模型解释。**

## 业务不变量

顶层执行项是测试任务直接编排的顶层工作流执行入口。顶层执行项严格按已封存 plan 顺序串行；Core 对每个顶层执行项调用一次 `BrowserSessionFactory.Create`，并在该顶层执行项结束后关闭返回的会话，再推进下一顶层执行项。同一顶层执行项内的根工作流与嵌套 WorkflowRef 复用该会话。每次创建全新且相互隔离的浏览器实例/会话上下文、使用不同会话身份，并隔离 cookies、LocalStorage、SessionStorage、IndexedDB、页面、登录态及其他浏览器状态，是宿主适配器的义务；当前不透明接口本身不强制或验证这些属性。重复工作流由 ExecutionID 区分。

## 当前证据

- `application/scheduling/decision.go`：`DecideAdvance`、serial shape
- `application/scheduling/coordinator.go`
- `domain/execution/plan.go`：有序执行项
- `application/engine/compiler.go`：按出现位置区分的执行项

## 调整清单

- [x] plan order 唯一权威。
- [x] state input 按 ExecutionID 归一化。
- [x] malformed serial shape 拒绝。
- [x] 宿主对 next execution 采用 CAS/fence 领取执行权。
- [x] `EntryExecutor` 校验 `WorkerFence`，并对调用方提供的顶层执行项顺序执行；每项恰好调用一次 `BrowserSessionFactory.Create`，同步关闭返回的会话后才允许继续。
- [x] 浏览器创建失败时不调用 `EntryRunner`；如返回部分会话则同步尝试关闭，并在错误或 panic 时停止后续执行项。`EntryExecutor` 不持久化 Entry/Run 状态，也不应用 `FailurePolicy`。
- [ ] 宿主适配器为每次 `Create` 提供全新隔离的浏览器实例/会话上下文、不同身份，并隔离 cookies、LocalStorage、SessionStorage、IndexedDB、cache、页面和登录态；当前不透明接口不强制或验证这些属性，尚无具体跨浏览器存储隔离一致性测试。
- [x] 同一顶层执行项的嵌套 WorkflowRef 按 `EntryRunner` 契约复用该顶层执行项的浏览器会话，不能为每个子工作流再次调用 `Create`。
- [x] 明确已封存 array order 与 SequenceNumber 一致性。
- [x] repeated 工作流不得以 WorkflowID 作为 execution identity。
- [x] 增加并发 scheduler adapter tests。

## 测试与验收

- [x] 两 scheduler 竞争仅一个开始 frontier entry。
- [ ] 宿主适配器契约测试验证每个顶层执行项获得不同浏览器会话身份，且前一顶层执行项的 cookies/LocalStorage/SessionStorage/IndexedDB/登录态在下一顶层执行项不可见；当前没有具体的宿主/跨浏览器存储隔离测试矩阵。
- [x] 按 `EntryRunner` 契约，顶层执行项内根工作流与多层嵌套 WorkflowRef 使用同一浏览器会话。
- [x] `EntryExecutor` 在 `EntryRunner` 成功、返回错误或 panic 后均同步调用关闭，并在关闭返回后才可能启动下一顶层执行项；错误或 panic 会停止后续执行项，实际隔离清理由宿主适配器保证。
- [ ] 浏览器创建失败后的 Entry/Run 状态持久化与 `FailurePolicy` 集成；`EntryExecutor` 仅停止执行并返回错误，不知道持久化终态。
- [x] running entry 阻止后续。
- [x] success 后准确选择下一 entry。
- [x] 同一工作流的多次出现彼此独立。
- [x] 持有失效租约的工作器租约不能提交。

## 依赖与风险

纯决策正确不等于并发安全；宿主 CAS/fencing 是验收必要条件。

## 审核

- [x] 批准保留现有设计
- [x] 修改：________________
