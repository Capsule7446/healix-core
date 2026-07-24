# C28 — 环境属性注入

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。本文按本次审核决定简化原评估中的凭据模型。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

环境是一组命名属性。执行实例执行时将所选环境的属性注入变量上下文，供工作流通过 `${env.<name>}` 使用；核心库不区分变量是否敏感，不负责 CredentialReference、Provider、Vault、轮换、明文读取或专用授权流程。

## 当前证据

- `domain/automation/assets.go`：单一 `Environment.Properties` 模型
- `domain/execution/run_snapshot.go`：不可变 `EnvironmentSnapshot`
- `application/scheduling/create_run_builder.go`：执行实例创建时组装环境输入
- `domain/interpolation/variables.go`：`${}` 插值
- `domain/automation/environment_keys_test.go`：属性 key 约束

## 调整清单

- [x] 将环境收敛为一组规范化属性，消除 Variables/Properties/CredentialReferences 的重复职责。
- [x] 移除 credential-like key 保留字校验，允许任意合法 property name。
- [x] 移除核心库的 CredentialReference、CredentialService、Provider resolution 和明文读取相关公共契约。
- [x] 明确变量命名空间与插值语法，统一使用 `${env.<name>}`。
- [x] 创建执行实例时选择环境，并将其属性作为本次执行实例的环境变量输入。
- [x] 执行每个顶层执行项时把同一组环境属性注入该顶层执行项的变量上下文。
- [x] 普通工作流参数与环境属性使用不同命名空间，禁止同名覆盖造成歧义。
- [x] 缺失的 `${env.<name>}` 在执行前或插值时返回明确错误，不静默替换为空字符串。
- [x] 环境修改对已创建执行实例是否生效，按 C07 的快照决策统一处理，不引入凭据轮换例外。

## 测试与验收

- [x] 任意合法 property key/value 可以保存，包括 password、token、username 等名称。
- [x] `${env.base_url}`、`${env.username}` 等能在工作流执行时正确展开。
- [x] 缺失环境或缺失 property 时执行失败并指出变量名。
- [x] 参数 `${name}` 与环境变量 `${env.<name>}` 不互相覆盖。
- [x] 每个顶层执行项的新浏览器都重新注入环境属性，不依赖前一顶层执行项的浏览器状态。
- [x] 核心库执行路径不再要求 CredentialProvider、Reviewer/Fence 凭据 authorization 或 secret-specific error。

## 依赖与风险

依赖 C02/C07/C17。该简化不提供敏感值隔离、轮换或专用脱敏保证；如果未来需要安全凭据管理，应作为新的独立需求设计，不在本次替换范围内预埋复杂抽象。

## 审核

- [x] 批准环境仅作为属性注入
- [x] 修改：________________
