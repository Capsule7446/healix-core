# healix-core

`healix-core` 是完整的 HealiX Product Domain + Execution Core，可由 Desktop、CLI、CI Runner 或 Server 复用。当前导出 API 属于 v0，尚未承诺 v1 兼容性。

## 公开包

| 包 | 职责 |
|---|---|
| `domain/fingerprint` | NodeSpec、Selector、Fingerprint、框架元数据和标准采样投影共享值对象 |
| `domain/interpolation` | `${name}` 变量表达式 |
| `domain/heal` | 确定性自愈决策、LCS 与 scorer |
| `domain/node` | Program、Step 状态机、Runtime 与执行端口 |
| `domain/sampling` | 临时采样 Session、Capture 幂等与编辑状态 |
| `domain/automation` | 持久化资产、不可变版本、TestTask 与 Revision 聚合规则 |
| `domain/execution` | 封存多入口 Plan、Run/Entry 生命周期与执行预算 |
| `domain/evidence` | 不可变执行事实与原子终态提交契约 |
| `application/automation` | 资源命令、Revision CAS 与采样发布编排 |
| `application/scheduling` | Plan 构造、顺序/失败策略决策与 claim 协调 |
| `application/engine` | 从封存 Plan 独立编译并执行每个 Entry Program |

## 依赖方向

```text
host application / infrastructure adapters
                   ↓
       application/engine
                   ↓
              domain/*
```

- Core 生产代码只依赖 Go 标准库和本模块包。
- Core 不包含 Wails、Svelte、SQLite、Rod、rrweb、文件路径或桌面设置。官方 go-rod 实现由独立模块 `github.com/Capsule7446/healix-rod` 提供，Core 不反向依赖它。
- 基础设施由宿主实现 Core 定义的 Driver、Recorder、Repository 与事实端口。
- UI 框架检测在宿主浏览器适配器完成；Core 只接收 `PageObservation`、`FrameworkStack` 和标准化 `NodeSpec`，不携带原始 DOM 或框架 SDK 对象。

## 验证

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

## 使用与非目标

远端版本发布前，Healix Desktop 通过本地替换集成：

```go
require github.com/Capsule7446/healix-core v0.0.0
replace github.com/Capsule7446/healix-core => ../healix-core
```

Core 不内置 Rod、Playwright、Wails、SQLite、rrweb、凭据存储、文件存储或 UI。宿主可以使用 `github.com/Capsule7446/healix-rod`，也可以实现其他 Driver；适配器负责把技术错误翻译为 Core 语义。

## 运行语义

- `Program`、Config 中的 map/slice 和封存 Execution Plan 在执行期间由调用方视为不可变；同一份可变对象不得并发修改。
- Driver 定位不到目标时必须返回或包装 `node.ErrElementNotFound`，只有该错误允许触发确定性自愈。
- 调用 Context 控制正常执行；Recorder Stop 等有界终态清理会脱离取消并最多继续 5 秒。
- `CompilePlan` 只消费已封存的 Execution Plan；`RunProgram` 不访问数据库、文件或网络资源存储。
