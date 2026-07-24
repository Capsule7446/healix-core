# C10 — NotFound 后自愈

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：已实现，需固定错误聚合契约。**

## 业务不变量

只有全部已配置 selector 明确返回 ElementNotFound 后才能触发 Heal；timeout、canceled、driver/system error 不得伪装成 NotFound。

## 当前证据

- `domain/node/step.go`：普通 locate → NotFound → healing
- `domain/node/errors.go`：错误分类
- `domain/node/safety_rejection_business_test.go`
- `domain/heal/healer.go`

## 调整清单

- [x] normal locate 先于 heal。
- [x] optional absence 与 heal 区分。
- [x] 明确多 selector 聚合何时形成“全部 NotFound”。
- [x] driver adapters 提供一致 typed errors。
- [x] timeout/cancel/system error 永不触发 Heal。
- [x] 增加 adapter error conformance tests。
- [x] Heal evidence staging 失败不得静默，若其用于治理则视为执行失败。

## 测试与验收

- [x] 任一 selector success 不触发 Heal。
- [x] 全部 NotFound 才触发一次 Heal。
- [x] timeout/cancel/driver error 不触发 Heal。
- [x] optional NotFound 按策略 skip 且不错误晋升。

## 依赖与风险

风险在于各浏览器适配器错误分类不一致；Evidence 不能同时是“可丢 telemetry”和“权威晋升输入”。

## 审核

- [x] 批准
- [x] 修改：________________
