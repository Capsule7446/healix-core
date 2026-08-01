# W1 — 执行授权与 fence 契约

来源：`findings-consolidated.md` §三 R5，以及本轮核查在同一条边界上发现的「fence 错误无 code」。
两者合并为一个流，因为它们是同一个 fence 在同一条路径上的两个缺陷。

## 独占文件

```
domain/execution/worker_fence.go
domain/execution/fault_codes.go
domain/execution/worker_fence_fault_test.go              （新建）
application/execution/entry_executor.go
application/execution/entry_executor_test.go
application/execution/entry_authorization_test.go        （新建）
docs/refactor/business-error-contract/error-code-registry.md   （全局独占）
```

**明确不碰：**

- `application/execution/ports.go` 与 `ports_test.go`、`heal_governance*`、`conformancetest/**` —— **W2**。
  本流对 fence 的修法刻意落在源头，使 `ports.go` 零改动（见 §3）。
- `domain/execution/status.go` 与 `domain/execution` 下**既有**的测试文件（含 `state_matrix_test.go`）—— **W3**。
  本流的 fence 测试新建独立文件。
- `application/scheduling/instance_command_services.go` —— **W5**。本流的改动会波及那里的
  `validateAbort`，这是一条**交接项**，不是本流的编辑（见 §5）。

无前置，可立即开工。

---

## 1. 现状证据

### 1.1 浏览器在任何鉴权之前就开了

```
entry_executor.go:120  Execute(ctx, fence, entry)
entry_executor.go:123    fence.Validate()          ← 只是形状检查
entry_executor.go:126    → executeEntry
entry_executor.go:130      factory.Create(...)     ← 浏览器在这里打开
entry_executor.go:175      runner.RunEntry(...)    ← Host 在这里面才会走到 engine.RunProgram
engine.go:129                VerifyExecutionAuthority(...)   ← 全仓唯一的授权校验
```

`WorkerFence.Validate()`（`worker_fence.go:18-23`）只检查 InstanceID 合法且 ClaimToken 非空。
过期或伪造的 claim 会走完 `Create` → `Close` 一整轮，实打实开一个浏览器进程。

`BrowserSessionFactory` 与 `EntryRunner` 都是 Host 端口，核心不做装配，因此核心**无法**验证
Host 是否在 `RunEntry` 里真的调了 `RunProgram`。核心能负责的是：在交出任何 Host 资源之前，
自己先问一次。

### 1.2 fence 形状错误逃逸时没有 code

`WorkerFence.Validate()` 返回裸 `errors.New`。两处公共边界原样返回它：

| 位置 | 现有注释 | 事实 |
|---|---|---|
| `entry_executor.go:121-122` | 「The fence returns its own classified fault」 | 假。返回的是裸 error |
| `ports.go:249-252` | 「Both validators return their own classified faults」，并写明后果是 hosts 只能回落到 blanket INTERNAL | 假。`fence.Validate()` 那一个不是 |

两条注释都在论证「所以这里不要包一层」，而它们据以论证的前提不成立。
另外两个调用点没问题：`scheduling/coordinator.go:129` 丢弃该错误并产出
`schedulingClaimInvalidError`；`instance_command_services.go:238` 包进
`abortInstanceCommandInvalidError`。

没有测试能看见：两份 fence conformance 只断言 `CodeWorkerFenceStale`（那是 Host 通过
`NewStaleWorkerFenceError()` 产的**另一个**错误），`state_matrix_test.go:113` 只断言有错/无错。

---

## 2. 修复步骤

### 步骤 1 — 新增 code（`domain/execution/fault_codes.go` + 注册表）

在 `fault_codes.go` 现有 const 块追加：

```go
// CodeWorkerFenceInvalid covers a malformed worker fence: the caller supplied
// an instance id or claim token this process cannot act on, so the remediation
// is to supply a well-formed fence. Distinct from EXECUTION_WORKER_FENCE_STALE,
// which means a well-formed fence no longer holds authority.
CodeWorkerFenceInvalid fault.Code = "EXECUTION_WORKER_FENCE_INVALID"
```

同一次提交在 `error-code-registry.md` 的 `## Execution` 表加一行：

| Code | Kind | Safe message | Allowed params | Retry / notes |
|---|---|---|---|---|
| `EXECUTION_WORKER_FENCE_INVALID` | `INVALID_ARGUMENT` | `worker execution authority is invalid` | none | Supply a well-formed fence — a valid instance id and a non-empty claim token — before retrying. Distinct from `EXECUTION_WORKER_FENCE_STALE`, which means a well-formed fence no longer holds authority. No instance id or claim token reaches public text. |

> `architecture/fault_contract_guard_test.go` 对「注册表有行但无常量」是硬失败，
> 所以常量和行必须同一次提交。message 必须与代码里的字面量逐字节相同。

### 步骤 2 — `WorkerFence.Validate()` 在源头分类

`worker_fence.go:18-23`：

```go
func (f WorkerFence) Validate() error {
	if f.InstanceID.Validate() != nil || f.ClaimToken == "" {
		return mustExecutionFault(fault.InvalidArgument, CodeWorkerFenceInvalid, "worker execution authority is invalid")
	}
	return nil
}
```

`mustExecutionFault` 已在同包 `fault_codes.go:24` 存在，直接用。
删掉现在唯一用到 `errors` 的那一行后，`worker_fence.go` 的 `errors` import 也要去掉。

这一步同时让 `entry_executor.go:121-122` 与 `ports.go:249-252` 的注释**自动变成真话**——
两处消费点一行都不用改，这正是选它的原因（见 §3）。

### 步骤 3 — 授权先于 `factory.Create`（R5 本体）

在 `entry_executor.go` 新增端口：

```go
// EntryAuthorizer answers whether this worker still holds the authority named
// by the fence, before any Host resource is created for the entry. The
// executor cannot rely on engine.RunProgram's own verification: that runs
// inside the Host's EntryRunner, which is reached only after a browser
// already exists.
type EntryAuthorizer interface {
	AuthorizeEntry(context.Context, domainexecution.WorkerFence, domainexecution.Entry) error
}
```

- `NewEntryExecutor` 增加首个参数 `authorizer EntryAuthorizer`，nil 走既有的
  `entryExecutorConfigurationInvalidError`，与 factory / runner 的检查并列。
- `Execute` 顺序改为：`fence.Validate()` → `authorizer.AuthorizeEntry(...)` → `executeEntry(...)`。
- 授权失败**原样返回**，不加任何包裹。Host 通常在这里返回 `CodeWorkerFenceStale`；
  再包一层会把 Host 需要的 code 埋掉，也违反本仓「never wrap an already-coded fault」的规则
  （`fault_codes.go:41-45` 的注释亲自点名了这条）。

**【裁决 1】** 授权失败要不要区分「无权」与「适配器不可达」。
- 选项 A（推荐）：不区分，原样透传。Host 已经能用 `CodeWorkerFenceStale` 与
  `CodeSchedulingAdapterUnavailable` 表达两者，核心不该替它归类。
- 选项 B：像 `classifySchedulingAdapterFailure` 那样给裸错误兜一个 code。
  代价是多一个 code，且会把「无权」误判成「不可用」，让 Host 去重试一个不该重试的调用。

**【裁决 2】** `NewEntryExecutor` 签名变化是公共 API 破坏性变更。v0.6.0 内直接改，
还是加一个 `NewEntryExecutorWithAuthorizer`？本仓既有约定是**直接替换、不留兼容外壳**，
推荐直接改签名。选了哪个写进 §5。

---

## 3. 为什么修在源头而不是两个消费点

另一条路是：`worker_fence.go` 不动，在 `entry_executor.go:123` 与 `ports.go:253`
各包一层 `classifyWorkerFence`。否掉的原因有两条：

1. **`ports.go` 是 W2 的独占文件。** 同工作树并行下没有三方合并兜底，跨界改动会直接覆盖。
   修在源头让消费点零改动，是唯一能让 W1 与 W2 彻底不相交的走法。
2. **两个消费点的修法会漏第三个。** 今天恰好只有两处泄漏；下一个直接返回
   `fence.Validate()` 的新边界不会有任何东西提醒它也要包。分类落在唯一的产生点，
   这个类别就关掉了。

代价是一条交接项：`instance_command_services.go` 的 `validateAbort` 会出现分支不一致，
见 §5。那是 4 行的事，且落在 W5 自己的文件里。

---

## 4. 验收

### `domain/execution/worker_fence_fault_test.go`（新建）

1. **畸形 fence 的错误带 code。** 空 ClaimToken、空 InstanceID、超长 InstanceID 三种输入，
   断言 `fault.IsCode(err, CodeWorkerFenceInvalid)`。
2. **它与 stale 是两个 code。** 断言 `CodeWorkerFenceInvalid != CodeWorkerFenceStale`，
   且畸形 fence 的错误 **不**命中 `CodeWorkerFenceStale`——两者的补救动作不同，
   Host 按 code 分流时必须分得开。
3. **合法 fence 返回 nil。**

> 不要改 `state_matrix_test.go`（W3 的文件）。它 L113 只断言有错/无错，本流改完仍绿。

### `application/execution/entry_authorization_test.go`（新建）

4. **授权失败时浏览器计数为 0。** 用一个记录 `Create` 调用次数的假 factory；
   authorizer 返回 `execution.NewStaleWorkerFenceError()`；断言
   `fault.IsCode(err, domainexecution.CodeWorkerFenceStale)` 且计数 `== 0`。
   **这条必须在步骤 3 之前先写、先看它红**，否则证明不了它抓得住。
5. **授权成功时顺序正确。** 断言 authorizer 的调用发生在 factory 之前（共享一个递增序号）。
6. **authorizer 为 nil 时构造失败**，code 为 `CodeEntryExecutorConfigurationInvalid`。
7. **畸形 fence 从 `Execute` 返回时命中新 code**，且 factory 计数为 0
   ——`fence.Validate()` 在 authorizer 之前，所以这条走的是更早的分支。

门禁：

```bash
go test ./domain/execution/... ./application/execution/... ./architecture/...
test -z "$(gofmt -l .)" && go vet ./... && go build ./... && go test ./... && go test -race ./...
```

`architecture/fault_contract_guard_test.go` 会自动核对新 code 的 Kind 与 message 是否与
注册表逐字节一致——它变红就是这一条没对齐，**改代码不改注册表**。

---

## 5. 交接与完成记录

### 给 W5（必须做，本流不动手）

步骤 2 之后，`application/scheduling/instance_command_services.go` 的 `validateAbort`
（L234-242）两个分支的错误链深度不再一致：

```go
if strings.TrimSpace(command.CommandID) == "" || … {
    return abortInstanceCommandInvalidError(nil)      // 链里只有 ABORT_INSTANCE_COMMAND_INVALID
}
if err := command.Fence.Validate(); err != nil {
    return abortInstanceCommandInvalidError(err)      // 链里还多一个 WORKER_FENCE_INVALID
}
```

顶层 `fault.CodeOf` 两支相同，公共文本相同，**没有任何既有测试能看见**；
但 `fault.IsCode` 走整条链，于是同一个 code 的两条产生路径对
`IsCode(err, CodeWorkerFenceInvalid)` 一真一假。这正是
`findings-consolidated.md` §F12 在 `validateCreateInstanceCommand` 上已经修过的那一类。

三个可选处置，请 W5 裁决并落字：
- **透传**：fence 分支改成 `return err`，让「无权/身份不对」与「命令字段不对」用不同的顶层 code。
  语义最准，但改变了已发布行为。
- **统一为不带内层 code**：fence 分支也传 `nil`，保住顶层契约、消掉分支差异，代价是丢掉私有 cause。
- **接受并测试固定**：保持现状，补一条测试把「fence 分支的链里有 WORKER_FENCE_INVALID」
  钉成契约的一部分。

### 给 W6（文档）

本流不碰 `docs/`（注册表除外）。需要落的公共变更：

- `docs/integration/public-contract.md`、`docs/application/execution/README.md`：
  `NewEntryExecutor` 新增必填端口 `EntryAuthorizer`，签名变为
  `NewEntryExecutor(authorizer, factory, runner, closeTimeout)`。
- 新增公共 code `EXECUTION_WORKER_FENCE_INVALID`（`INVALID_ARGUMENT`）。
- `Execute` 与 `StepTransitionService.Commit` 在 fence 畸形时的返回值从无 code 变为有 code。

### 给 W2（无需动作，确认即可）

`ports.go:249-252` 的注释在本流落地后自动成立，**不要重写它**。

### 完成记录

> 由执行者填写：实际改了什么 / 跑过哪些验收 / 裁决 1 与裁决 2 各选了什么、为什么。
