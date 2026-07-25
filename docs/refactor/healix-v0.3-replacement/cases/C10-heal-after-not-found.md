# C10 — 未找到后自愈

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

只有全部已配置选择器明确返回 ElementNotFound 后才能触发自愈；timeout、canceled、driver/system error 不得伪装成 NotFound。

## 当前证据

- `domain/node/step.go`：普通 locate → NotFound → healing
- `domain/node/errors.go`：错误分类
- `domain/node/safety_rejection_business_test.go`
- `domain/heal/healer.go`

## 调整清单

- [x] 先执行常规定位，再尝试自愈。
- [x] optional absence 与 heal 区分。
- [x] 明确多选择器聚合何时形成“全部 NotFound”。
- [x] driver adapters 提供一致 typed errors。
- [x] timeout/cancel/system error 永不触发自愈。
- [x] 增加 adapter error conformance tests。
- [x] 自愈 evidence staging 失败不得静默，若其用于治理则视为执行失败。

## 测试与验收

- [x] 任一选择器 success 不触发自愈。
- [x] 全部 NotFound 才触发一次自愈。
- [x] timeout/cancel/driver error 不触发自愈。
- [x] optional NotFound 按策略 skip 且不错误晋升。

## 依赖与风险

风险在于各浏览器适配器错误分类不一致；执行证据不能同时是“可丢 telemetry”和“权威晋升输入”。

## 审核

- [x] 批准
- [x] 修改：________________
