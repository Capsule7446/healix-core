# C07 — 环境与策略冻结

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

执行实例创建时确定所选环境，并将其一组属性与 ScreenshotPolicy、HealerPolicy 纳入执行输入；执行顶层执行项时直接注入 `${env.<name>}`。环境不区分普通值与敏感值，不使用 CredentialReference 或运行时 Provider 解析。后续环境修改是否影响旧执行实例，以本次批准的快照语义为准。

## 当前证据

- `domain/automation/assets.go`：单一 `Environment.Properties` 模型
- `domain/execution/run_snapshot.go`：`EnvironmentSnapshot`
- `application/scheduling/create_run_builder.go`：执行实例创建输入组装
- `domain/interpolation/variables.go`：`${env.<name>}` 插值
- `domain/automation/screenshot_policy.go`
- `domain/automation/healer_policy.go`

## 调整清单

- [x] RunSnapshot 保存 environment ID/revision/base URL/完整属性。
- [x] 将环境的 Variables/Properties/CredentialReferences 收敛为单一属性模型；不做机敏分类。
- [x] 顶层执行项执行时把属性注入 `${env.<name>}` 变量上下文。
- [x] 冻结 ScreenshotPolicy 和 versioned HealerPolicy。
- [x] 定义 per-step capture intent 与 run policy precedence。
- [x] 增加 policy schema version/digest。
- [x] 由 mapper 生成 runtime healing/screenshot config，禁止宿主临时注入可变策略。
- [x] 校验 required environment keys。

## 测试与验收

- [x] 执行实例创建后修改 Environment/Policy 不影响执行。
- [x] 所有环境 property（不论名称）按快照值注入每个顶层执行项。
- [x] 缺失 `${env.<name>}` 返回明确错误。
- [x] versioned zero values 不被误当默认值。
- [x] invalid policy 在执行实例创建时失败。

## 依赖与风险

依赖 C02/C17/C28；主要风险是属性命名冲突和快照大小。旧 CredentialReference API 直接移除，不保留兼容入口。

## 审核

- [x] 批准
- [x] 修改：________________
