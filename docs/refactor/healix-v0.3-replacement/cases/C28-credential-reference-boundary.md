# C28 — Environment Properties 注入

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。本文按本次审核决定简化原评估中的凭据模型。

## 状态

**当前：Core 存在 Variables/Properties 与 CredentialReference 两套模型；本次决定统一简化为普通 Properties 注入。**

## 业务不变量

Environment 是一组命名 Properties。Run 执行时将所选 Environment 的 Properties 注入变量上下文，供 Workflow 通过 `${env.<name>}` 使用；Core 不区分变量是否敏感，不负责 CredentialReference、Provider、Vault、轮换、reveal 或专用授权流程。

## 当前证据

- `domain/automation/assets.go`：`Environment.Variables`、`Environment.Properties`、`CredentialReferences`
- `domain/execution/environment.go`：Environment execution descriptor
- `domain/interpolation/variables.go`：`${}` 插值
- `domain/automation/environment_keys.go`：environment key 提取

## 调整清单

- [x] 将 Environment 收敛为一组规范化 Properties，消除 Variables/Properties/CredentialReferences 的重复职责。
- [x] 移除 credential-like key 保留字校验，允许任意合法 property name。
- [x] 移除 Core 的 CredentialReference、CredentialService、Provider resolution 和 reveal 相关公共契约。
- [x] 明确变量命名空间与插值语法，统一使用 `${env.<name>}`。
- [x] 创建 Run 时选择 Environment，并将其 Properties 作为本次 Run 的环境变量输入。
- [x] 执行每个 Entry 时把同一组 Environment Properties 注入该 Entry 的变量上下文。
- [x] 普通 Workflow 参数与 Environment Properties 使用不同命名空间，禁止同名覆盖造成歧义。
- [x] 缺失的 `${env.<name>}` 在执行前或插值时返回明确错误，不静默替换为空字符串。
- [x] Environment 修改对已创建 Run 是否生效，按 C07 的快照决策统一处理，不引入凭据轮换例外。

## 测试与验收

- [x] 任意合法 property key/value 可以保存，包括 password、token、username 等名称。
- [x] `${env.base_url}`、`${env.username}` 等能在 Workflow 执行时正确展开。
- [x] 缺失 Environment 或缺失 property 时执行失败并指出变量名。
- [x] 参数 `${name}` 与环境变量 `${env.<name>}` 不互相覆盖。
- [x] 每个顶层 Entry 的新浏览器都重新注入 Environment Properties，不依赖前一 Entry 的浏览器状态。
- [x] Core 执行路径不再要求 CredentialProvider、Reviewer/Fence credential authorization 或 secret-specific error。

## 依赖与风险

依赖 C02/C07/C17。该简化不提供敏感值隔离、轮换或专用脱敏保证；如果未来需要安全凭据管理，应作为新的独立需求设计，不在本次替换范围内预埋复杂抽象。

## 审核

- [x] 批准 Environment 仅作为 Properties 注入
- [x] 修改：________________
