# C07 — Environment 与策略冻结

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：值对象存在，但未进入 Run/Plan。**

## 业务不变量

Run 创建时确定所选 Environment，并将其一组 Properties 与 ScreenshotPolicy、HealerPolicy 纳入执行输入；执行 Entry 时直接注入 `${env.<name>}`。Environment 不区分普通值与敏感值，不使用 CredentialReference 或运行时 Provider 解析。后续 Environment 修改是否影响旧 Run，以本次批准的快照语义为准。

## 当前证据

- `domain/automation/assets.go`：Environment Variables/Properties/CredentialReferences（待简化）
- `domain/execution/environment.go`：EnvironmentDescriptor
- `domain/interpolation/variables.go`：`${env.<name>}` 插值
- `domain/automation/screenshot_policy.go`
- `domain/automation/healer_policy.go`

## 调整清单

- [x] RunSnapshot 保存 environment ID/revision/base URL/完整 Properties。
- [x] 将 Environment 的 Variables/Properties/CredentialReferences 收敛为单一 Properties 模型；不做机敏分类。
- [x] Entry 执行时把 Properties 注入 `${env.<name>}` 变量上下文。
- [x] 冻结 ScreenshotPolicy 和 versioned HealerPolicy。
- [x] 定义 per-step capture intent 与 run policy precedence。
- [x] 增加 policy schema version/digest。
- [x] 由 mapper 生成 runtime healing/screenshot config，禁止 Host 临时注入可变策略。
- [x] 校验 required environment keys。

## 测试与验收

- [x] Run 创建后修改 Environment/Policy 不影响执行。
- [x] 所有 Environment property（不论名称）按快照值注入每个 Entry。
- [x] 缺失 `${env.<name>}` 返回明确错误。
- [x] versioned zero values 不被误当默认值。
- [x] invalid policy 在 Run 创建时失败。

## 依赖与风险

依赖 C02/C17/C28；主要风险是 Properties 命名冲突和快照大小。旧 CredentialReference API 直接移除，不保留兼容入口。

## 审核

- [x] 批准
- [x] 修改：________________
