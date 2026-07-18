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
- Bounded Context：`fingerprint/interpolation` 为 shared kernel，`heal/node` 为 execution，`sampling/workspace` 为 product workspace，`metrics` 为只读 projection。
- 新增导出符号必须写明消费者场景、Context、nil、错误、并发与所有权语义；provider 接口只增加当前用例实际需要的最小能力。
- 新增状态、动作或断言不得使用无常量约束的裸字符串。

## 禁止事项

- 不引入 Wails、Svelte、SQLite、Rod、rrweb、文件路径、桌面设置或 UI 事件名。
- Domain 不携带 JSON/YAML/XML/SQL/ORM tag。
- 不建立第二套执行入口；只保留 `application/engine.CompileExecution + RunProgram`。
- 不让 `metrics` 成为写账本；它只提供只读质量投影。
- 不把 Wails View、UI 事件名、数据库行模型或宿主配置放入 Core；Workspace 中的 URI 只能作为宿主解释的 opaque locator。
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
