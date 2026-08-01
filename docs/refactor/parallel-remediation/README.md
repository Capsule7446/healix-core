# 五条结构性缺口的并行修复：分工与冲突边界

配套：`docs/refactor/findings-consolidated.md` §三（R5–R9）、本目录下 6 份 runbook。

**执行模型：同一个工作树、同一个分支 `refactor/error-code`、六个并发工作流。**

这一点决定了本文件的全部约束。不是六个分支最后合并——**没有三方合并这一步**。
两个流写同一个文件，就是后写的直接覆盖先写的，git 不会报冲突，测试也不会变红，
你只会发现另一个人的改动凭空消失了。所以下面的所有权矩阵是**硬约束**，不是建议。

---

## 0. 开工前必须清掉的既有状态

工作树在编写本计划时有四个文件带未提交改动：

```
 M application/execution/ports.go                    （字节预算 1<<18 → 1<<20 的回退与论证）
 M application/execution/ports_test.go
 M application/scheduling/create_instance_builder.go （sortedKeys 补扫）
 M architecture/unified_language_boundary_test.go    （fingerprint Clone 守卫加强）
```

`ports.go` 与 `ports_test.go` 是 **W2** 的独占文件，`unified_language_boundary_test.go` 是
**W4** 的。在它们被提交之前，W2 和 W4 一动手就会把这些改动搅进自己的提交里，
或者被自己的编辑覆盖掉。

**第一步（串行，由一个人做完再放并行）：**

```bash
git add application/execution/ports.go application/execution/ports_test.go application/scheduling/create_instance_builder.go architecture/unified_language_boundary_test.go && git status --short
```

确认这四个文件的改动是想要的，提交掉；不想要就 `git restore`。工作区 `git status --short`
除本目录的新文档外为空，才算基线就绪。

---

## 1. 工作流一览

| 流 | 主题 | 来源 | 前置 |
|---|---|---|---|
| **W1** | 执行授权与 fence 契约 | R5 + 核查新增的「fence 错误无 code」 | §0 |
| **W2** | 证据坐标与 occurrence | R6 + 核查新增的 engine 同名常量 | 无 |
| **W3** | Entry 状态机与终态提交 | R7 | 无（第二阶段需裁决） |
| **W4** | 未发布资产的正式身份 | R8 | §0 |
| **W5** | 摘要 wire tag 纪律 | R9a | 无 |
| **W6** | 文档链接与台账 | R9b + 核查新增的 182 条断链 | 无 |

W5 与 W6 同源于 R9，但落点完全不相交（W5 = scheduling 代码 + 持久化契约，W6 = 全仓 markdown），
拆开才有并行收益，不合并。

---

## 2. 文件所有权矩阵

**不在自己名下的文件，一行都不改。** 需要邻居改动才能完成的，写进自己 runbook 的
「交接」段，不要越界动手。

> 下表里的 `**` 一律只含 `.go`。**所有 `TEST_CASES.md` 无条件归 W6**，
> 包括 `domain/evidence/TEST_CASES.md`、`domain/execution/TEST_CASES.md`、
> `domain/node/TEST_CASES.md`、`domain/sampling/TEST_CASES.md`、
> `application/engine/TEST_CASES.md`、`application/execution/TEST_CASES.md` 等全部 13 个。

### 生产代码

| 路径 | 归属 |
|---|---|
| `domain/execution/worker_fence.go` · `fault_codes.go` | **W1** |
| `application/execution/entry_executor.go` | **W1** |
| `domain/execution/status.go` | **W3** |
| `domain/execution/` 其余 `.go` | **无人** |
| `application/execution/` 除 `entry_executor.go` 外全部 `.go`（`ports.go`、`heal_governance.go`、`conformancetest/**`） | **W2** |
| `domain/evidence/*.go` | **W2** |
| `domain/node/runtime.go` · `step.go` · `composite.go` · `validation.go` | **W2** |
| `application/engine/*.go`（`engine.go`、`coordinator.go`、`compiler.go` 等全部） | **W2** |
| `application/scheduling/decision.go` · `coordinator.go` | **W3** |
| `application/scheduling/instance_command_services.go` | **W5** |
| `domain/sampling/*.go` | **W4** |
| 其余全部 `.go` | **无人**（含 `application/scheduling/create_instance_{service,builder,types}.go`） |

三处同包不同文件的共存，都已核实文件不相交：
`domain/execution`（W1 / W3）、`application/execution`（W1 / W2）、
`application/scheduling`（W3 / W5）。同包不同文件在同一工作树里是安全的。

### 测试

| 路径 | 归属 |
|---|---|
| `application/execution/entry_executor_test.go` | **W1**（已核实：零 `evidence.` 引用，W2 不会碰） |
| `application/execution/entry_authorization_test.go`（新建） | **W1** |
| `domain/execution/worker_fence_fault_test.go`（新建） | **W1** |
| `application/execution/` 其余全部 `_test.go`（`ports_test.go`、`heal_governance*_test.go`、`commit_ownership_test.go`、`fence_conformance_test.go`、`conformancetest/**`） | **W2** |
| `domain/evidence/*_test.go` · `domain/node/*_test.go` · `application/engine/*_test.go` | **W2** |
| `contract/public_api_test.go` | **W2**（仅 L118 的 `coreengine.EntrySucceeded`） |
| `architecture/evidence_coordinate_test.go`（新建） | **W2** |
| `domain/execution/` 既有 `_test.go`（含 `state_matrix_test.go`） | **W3** |
| `application/scheduling/decision_test.go` · `coordinator_test.go` | **W3** |
| `architecture/entry_status_enforcement_test.go`（新建） | **W3** |
| `architecture/unified_language_boundary_test.go` | **W4**（唯一改既有 architecture 测试的流） |
| `domain/sampling/*_test.go` | **W4** |
| `application/scheduling/instance_command_digest_test.go` | **W5** |
| `architecture/digest_wire_tag_test.go`（新建） | **W5** |
| `architecture/doc_links_test.go`（新建） | **W6** |
| `architecture/dependencies_test.go` · `fault_contract_guard_test.go` · `contract/fault_public_api_test.go` | **无人** |

### 文档

| 路径 | 归属 |
|---|---|
| `docs/refactor/business-error-contract/error-code-registry.md` | **W1 独占**（全局唯一有权新增 code 行的流） |
| `docs/refactor/digest-wire-tags.md`（新建） | **W5** |
| `docs/refactor/parallel-remediation/W*.md` | 各流只改自己那份 |
| 其余全部 `docs/**` 与所有 `TEST_CASES.md` | **W6 独占** |
| `docs/refactor/findings-consolidated.md` | **W6 独占**——任何流都不得在此登记自己的完成状态 |

---

## 3. 同工作树并行的操作纪律

前三条不是风格问题，违反任何一条都会静默破坏别人的工作。后两条是时序与命名。

### 3.1 提交只列自己的文件，永远不用 `-A` / `-a`

```bash
# 对
git add application/execution/entry_executor.go application/execution/ports.go
git commit -m "fix: authorize the entry before the host opens a browser"

# 错——会把另外五个流正在编辑的半成品一起提交
git add -A && git commit -m "..."
git commit -am "..."
```

`git stash`、`git checkout .`、`git restore .` 同理：**任何不带精确路径的写操作都禁止**。

### 3.2 覆盖率文件不要写在仓库根

`coverage.out` 在 `.gitignore` 里，但六个流同时写同一个路径会互相截断，
拿到的数字是别人的。每流用自己的路径：

```bash
go test -coverprofile="$TMPDIR/cov-W1.out" ./... && ./scripts/check-coverage.sh "$TMPDIR/cov-W1.out" 80
```

### 3.3 全量 `go build ./...` 在并行期间不是可靠信号

别人半改完的包会让你的全量构建失败，而那和你无关。工作期间只验证自己的包：

```bash
go test ./application/execution/...      # W1
go test ./domain/evidence/... ./domain/node/... ./application/engine/...   # W2
```

全量门禁（§4.6）只在**准备提交的那一刻**跑，且如果失败先确认失败点是不是自己的文件。

### 3.4 两处跨流时序（不是冲突，但要按顺序做）

所有权已经做到文件级不相交，代价是有两件事必须等邻居先落地。两处都很小，
各自在对应 runbook 里也标了。

| 依赖 | 内容 | 谁等谁 |
|---|---|---|
| **W5 等 W1** | W1 让 `WorkerFence.Validate()` 返回 coded fault 之后，`instance_command_services.go` 的 `validateAbort`（L234-242）两个分支的错误链深度会不一致（fence 分支多一个 code，畸形命令分支没有）——即 `findings-consolidated.md` §F12 已修过的那一类。该文件是 W5 的，由 W5 收尾。W5 的摘要工作与此无关，可以先做。 | W5 的最后一步 |
| **W2 自查 W1** | W1 改完后，`application/execution/ports.go:249-252` 那句「Both validators return their own classified faults」自动变成真话，**不需要改**。W2 拿到该文件后确认一次即可，别顺手重写。 | 无阻塞 |

反过来，W1 **不碰** `ports.go`：那里的 fence 泄漏由 W1 在源头（`worker_fence.go`）修掉，
消费点零改动。这是本计划刻意选的路线，理由见 `W1-execution-authorization.md` §3。

### 3.5 `package architecture_test` 的 helper 命名

W2/W3/W5/W6 各自新建一个 `architecture/*_test.go`，四份都在同一个包里。
复用 `dependencies_test.go` 已有的 `repositoryRoot`、`walkProductionGo`、`walkAllGo`；
**新增的包级函数、类型、常量必须带流前缀**（`w2evidenceFields`、`w5tagInventory`…）。
同包重名是直接编译失败，会把四个流一起打红。

---

## 4. 公共规则

1. **新增 error code 只能由 W1 提。** `architecture/fault_contract_guard_test.go` 对
   「注册表有行但无实现」是硬失败，因此 code 行与常量必须同一次提交落地，无法预先占位。
   其余五流的设计已确保零新增 code——复用既有 `VALIDATION_FIELD_*` 违例码与既有顶层码。
   若某流发现确实需要新 code，**停下来找 W1 排队**，不要自己往注册表里写。
2. **不改已发布 code 的 Kind 与 message。** 守卫变红时改代码，不改注册表行；
   理由见 `error-code-registry.md` 的「Registry rules」。
3. **只新建测试文件，不改共享测试文件。** 例外只有矩阵里点名的三处：
   W4 的 `unified_language_boundary_test.go`、W2 的 `contract/public_api_test.go` L118、
   W1/W2/W3/W5 各自包内自己名下的既有测试。
4. **公共 API 的破坏性变更要在自己 runbook 里点名。** W1（`NewEntryExecutor` 签名）、
   W2（evidence 结构体新增必填字段）都会动 Host 侧编译，各自写清楚。
5. **不碰 `findings-consolidated.md`。** 完成状态写在自己的 runbook 末尾，W6 最后统一收口。
6. **提交前门禁**（与 `.github/workflows/ci.yml` 一致）：

```bash
test -z "$(gofmt -l .)" && go vet ./... && go build ./... && go test ./... && go test -race ./...
```

---

## 5. 提交顺序

六流之间没有编译期依赖，先后随意。只有两处偏好：

- **W6 最后收口。** 其他流会新增和改名测试，`TEST_CASES.md` 台账随之不完整
  （注意是不完整，不是断链——没有任何测试对台账完整性设门禁）。W6 在最后跑一次
  链接审计与台账补全，收益最大。
- **W2 早于 W6 的台账部分。** W2 的改动面最大，台账变化最多。

---

## 6. 每流交付物

1. 若干次提交，信息遵循 `<type>: <description>`，每次只列自己的文件。
2. 自己 runbook 末尾的「完成记录」：实际改了什么、哪些验收跑过、哪些决策点怎么裁的。
3. **`【裁决】` 标记的地方必须落字。** 那些是设计选择而不是编辑；选了哪个、为什么，
   写下来。不写就等于下一轮审核要重新推一遍。
