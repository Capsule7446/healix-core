# AGENTS.md

使用简体中文，先给结论，再给必要依据；回复简洁、直接、准确。

## 项目定位

`healix-core` 是 HealiX 的公开 Go Core Module，只包含领域模型、端口和框架无关执行用例。

## 架构规则

```text
application/engine -> domain/*
host infrastructure -> domain ports
```

- `domain` 只依赖标准库或其他 `domain` 包。
- `application` 只依赖标准库和 `domain`，不得依赖宿主或第三方 SDK。
- 端口在 Core 内定义，实现和依赖装配在宿主最外层。
- 公共类型和错误属于兼容性契约，修改前必须补契约测试。
- Bounded Context：`fault/fingerprint/interpolation/parameter` 为 shared kernel，`automation` 拥有持久化资产，`execution` 拥有 Plan/Instance 生命周期，`evidence` 拥有事实，`sampling` 拥有临时采样状态。该划分由 `architecture/dependencies_test.go` 的 `domainContext` 强制。
- 新增导出符号必须写明消费者场景、Context、nil、错误、并发与所有权语义；provider 接口只增加当前用例实际需要的最小能力。
- 新增状态、动作或断言不得使用无常量约束的裸字符串。
- 业务失败一律走 `domain/fault`：不导出哨兵 error 变量，错误码是各包 `fault_codes.go` 中导出的 `fault.Code` 常量，前缀由所属上下文独占，且只能新增或墓碑化、不得重命名或复用。字段级原因只用 `fault` 的 `VALIDATION_FIELD_*` 违规码，不下沉为顶层 `Code`。新增或修改错误码必须同步 `docs/contracts/error-code-registry.md`，否则 `architecture/fault_contract_guard_test.go` 会失败。

## 禁止事项

- 不引入 Wails、Svelte、SQLite、Rod、rrweb、文件路径、桌面设置或 UI 事件名。
- Domain 不携带 JSON/YAML/XML/SQL/ORM tag。
- 不建立第二套执行入口；只保留 `application/engine.CompilePlan + RunProgram`。
- 不在 Core 建立业务读投影；消费项目基于 Evidence 事实拥有查询和指标。
- 不把 Wails View、UI 事件名、数据库行模型或宿主配置放入 Core；采样工作区（`sampling.UnpublishedFlowFragment`）当前不携带 URI，若确需引入也只能作为宿主解释的 opaque locator。
- `v1` 以后按 SemVer 管理导出 API；破坏性修改必须提升主版本。

## 验证

```bash
gofmt -l .
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

同时必须保留外部包形式的公开 API 契约测试，并在 Healix 宿主完成消费侧 smoke、browser E2E 与完整闭环验证。

<!-- CODEGRAPH_START -->
## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow. Name a file or symbol in the query to read its current line-numbered source. If it's listed but deferred, load it by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` prints the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->
