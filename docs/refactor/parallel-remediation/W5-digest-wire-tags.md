# W5 — 摘要 wire tag 纪律

来源：`findings-consolidated.md` §三 R9 的前半（Snapshot/Evidence 编码）。
核查确认两个 tag 属实，**但「编码未动」这个结论不成立**——同一分支已经改过三个同类摘要。

本流的核心产出不是修一处 bug，而是**让这类静默变更不可能再发生**。

## 独占文件

```
application/scheduling/instance_command_services.go
application/scheduling/instance_command_digest_test.go
architecture/digest_wire_tag_test.go     （新建）
docs/refactor/digest-wire-tags.md        （新建）
```

**明确不碰：** `application/scheduling/decision.go` 与 `coordinator.go`（W3）、
`application/scheduling/create_instance_service.go` 与 `domain/execution/instance_snapshot.go`
（无人认领，只读引用——本流只读它们的 tag 值，一个字节都不改）、
`application/scheduling/TEST_CASES.md`（W6）。

**本流还承接一条来自 W1 的交接项**（`validateAbort` 的错误链分支不一致，见 §5），
因为它落在本流独占的同一个文件里。它与摘要工作互不依赖，摘要部分可以先做。

无前置，可立即开工；交接项等 W1 落地。

---

## 1. 现状证据

### 1.1 R9 说对的部分

两个 tag 都在，注释也都在：

| 字节 | 位置 | 谁依赖 |
|---|---|---|
| `"healix.run-snapshot"` | `domain/execution/instance_snapshot.go:662`（注释在 L659-661） | 每一份已封存快照的 digest |
| `"create-run-request-v1"` | `application/scheduling/create_instance_service.go:21`（注释在 L17-20） | 每一条已存的创建幂等记录 |

`HydrateInstanceSnapshot`（`instance_snapshot.go:171-180`）确实会在重算 digest 对不上时
返回 `createInstanceSnapshotConflictError()`。

还有一处 R9 没提、但更能证明这条纪律曾经是认真的：
`encodeCanonical`（`instance_snapshot.go:702-724`）为 `InstanceID` / `EntryID` /
`StepExecutionID` / `InvocationPath` 各开了一个 case，专门编码成它们替换掉的裸字符串，
就为了值对象化不移动 digest。

### 1.2 R9 说错的部分

`5ecfde2`（`fix: stop the cancel and abort digests from dropping the instance identity`）
把三个同类摘要**重新编码了**：

```
CancelInstanceRequestDigest   sha256(json.Marshal(cmd))  →  逐字段编码器 + "cancel-instance-request-v1"
AbortInstanceRequestDigest    同上                        →  同上 + "abort-instance-request-v1"
ReorderQueueRequestDigest     同上                        →  同上 + "reorder-queue-request-v1"
```

`git log -S` 确认这三个字符串是该 commit 新引入，历史上从不存在 `cancel-run-request-v1`
之类的旧拼写——也就是说这三条摘要**在此之前根本没有域标签**，现在有了，
所有已存的取消 / 中止 / 重排幂等记录的摘要从此永远算不回来。

这正是 R9 用来论证「不能动 snapshot/create」的那个后果。

那次改动本身是正当的 bug 修复（`json.Marshal` 把只含未导出字段的值对象 `InstanceID`
编成 `{}`，不同实例的取消会撞摘要）。问题在于 **commit message 只讲了撞摘要，
没提存量记录失效**，而 `findings-consolidated.md` §R9 把「不破坏已存摘要」
写成了一条一贯遵守的原则。实际是：两个 tag 保留并加注释，三个重编码且无任何说明。

### 1.3 全仓 wire tag 清单（本流要落成文档的东西）

```
domain/execution/instance_snapshot.go:662              "healix.run-snapshot"          有注释  未变更
application/scheduling/create_instance_service.go:21   "create-run-request-v1"        有注释  未变更
application/scheduling/instance_command_services.go:299 "cancel-instance-request-v1"  无注释  5ecfde2 引入
application/scheduling/instance_command_services.go:300 "abort-instance-request-v1"   无注释  5ecfde2 引入
application/scheduling/instance_command_services.go:301 "reorder-queue-request-v1"    无注释  5ecfde2 引入
application/automation/heal_candidate_repository.go:17  "heal-review-v1"              无注释  未变更
application/automation/sampling_publication_transaction.go:16 "sampling-publication-v1" 无注释 未变更
```

复现：

```bash
grep -rn 'DigestV1 *=\|e\.str("healix' --include=*.go . | grep -v _test.go
```

---

## 2. 修复步骤

### 步骤 1 — 给三个常量补 wire-tag 注释

`instance_command_services.go:290-301` 现有的注释解释了**为什么改成逐字段编码**，
但没说这三个字符串本身是持久化格式。补上与 `create_instance_service.go:17-20`
同款的措辞，让下一次改名的正则不敢碰它们：

```go
// These three strings are wire tags, not Go names. They are the domain
// separation prefix of a stored idempotency digest, so a later rename must not
// touch them. They were introduced by the field-by-field rewrite below, which
// changed the digest of every cancel, abort, and reorder record stored before
// it — see docs/refactor/digest-wire-tags.md for that break and its status.
```

### 步骤 2 — 写下那次未记录的破坏（新建 `docs/refactor/digest-wire-tags.md`）

内容至少包含：

1. §1.3 的完整清单（tag 字节、位置、依赖方、是否在本分支变更过）。
2. `5ecfde2` 造成的存量失效：影响哪三条命令、失效方向是什么
   （重放一条旧命令时摘要不再命中 → 被当作新请求 → **重复执行**一次取消 / 中止 / 重排）。
3. 为什么 snapshot 与 create 两个 tag 反而不能动——它们的失效方向更糟
   （`HydrateInstanceSnapshot` 会把**所有**已存快照判为
   `EXECUTION_CREATE_INSTANCE_SNAPSHOT_CONFLICT`）。
4. 规则：任何 wire tag 的变更都必须同时提供迁移方案（重算或双读窗口），
   并在本文件登记；不得作为改名的副产品滑进去。

### 步骤 3 — 裁决存量记录怎么办

**【裁决 1】** `5ecfde2` 已经让三条命令的存量幂等记录失效。现在怎么处理？

- **选项 A — 记录并接受（推荐，若尚无生产 Host）。**
  v0.6.0 尚未发布，若没有任何 Host 存着 v0.5 的取消/中止/重排幂等记录，
  这次破坏就没有实际受害者。写进 §2 步骤 2 的文档，作为发布说明的一条。
- **选项 B — 双读窗口。** 保留旧的 `sha256(json.Marshal(cmd))` 作为
  `*RequestDigestLegacy`，Host 在查找幂等记录时两个都试，过渡一个版本后删掉。
  代价：旧算法是**已知有 bug 的**（丢 InstanceID），双读等于把那个 bug 的
  碰撞面又打开一个版本。若选 B，必须同时限制旧算法只用于**查找**、绝不用于**写入**。
- **选项 C — 一次性重算。** Host 侧遍历存量记录重算摘要。最干净，但需要 Host 配合，
  且要能枚举存量。

选 A 是最可能正确的答案，但它必须是**明说的选择**而不是默认沉默。

**【裁决 2】** 要不要给这三个 tag 加 `-v2` 后缀，把这次破坏显式化。
- 反对：现在改就是**第二次**破坏，把已经失效的记录再失效一次，没有收益。
- 赞成：`-v1` 这个后缀在撒谎——它暗示这是第一版编码，实际是第二版。
- 推荐：**不改字节，在文档与注释里说明版本号的实际含义**。
  字节的稳定性比命名的诚实重要，这正是本条缺口的教训本身。

### 步骤 4 — 让它不可能再静默发生（本流最有价值的产出）

新建 `architecture/digest_wire_tag_test.go`（`package architecture_test`，
复用 `repositoryRoot` / `walkProductionGo`，**新增的包级标识符一律加 `w5` 前缀**）：

```go
// w5wireTagInventory pins every persisted digest domain-separation tag in the
// tree. These bytes are a storage format: changing one silently invalidates
// every record hashed with it, and no existing test can see that happen — the
// digest simply becomes a different, equally valid-looking string. Changing a
// tag must therefore be impossible without also editing this list, which is
// where the reviewer is told to demand a migration.
var w5wireTagInventory = map[string]string{
    "domain/execution/instance_snapshot.go":                    "healix.run-snapshot",
    "application/scheduling/create_instance_service.go":        "create-run-request-v1",
    // …其余五条
}
```

守卫要断言**两个方向**：

1. 清单里的每个 tag 字节在对应文件中确实存在（有人改了 tag → 红）。
2. 清单外没有新的 digest tag（有人加了新 digest 而没登记 → 红）。
   识别方式：`sha256.New()` / `sha256.Sum256` 所在函数里出现的、
   被写进哈希的字符串字面量。做不到完全精确也没关系——
   宁可让新增 digest 时多改一行清单，也不要漏掉一个。

只做方向 1 就等于没做一半：`5ecfde2` 是**新增**三个 tag，方向 1 抓不到它。

### 步骤 5 — 承接 W1 的 `validateAbort`（等 W1 落地后做）

W1 会让 `WorkerFence.Validate()` 返回 coded fault。之后本文件的 `validateAbort`
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
但 `fault.IsCode` 是 `errors.Is`、走整条链，于是同一个顶层 code 的两条产生路径对
`IsCode(err, CodeWorkerFenceInvalid)` 一真一假。按更深 code 分支的 Host 会把同一类请求
路由到两条路上。这正是 `findings-consolidated.md` §F12 在 `validateCreateInstanceCommand`
上已经修过的那一类。

**【裁决 3】** 三选一，必须落字：

- **透传**：fence 分支改成 `return err`。语义最准——「身份不对」与「命令字段不对」的
  补救动作不同，压成一个顶层 code 本来就丢了区分。代价：改变已发布行为，
  原本收到 `ABORT_INSTANCE_COMMAND_INVALID` 的 Host 会看到新 code。
- **统一为不带内层 code**：fence 分支也传 `nil`。保住顶层契约、消掉分支差异，
  代价是丢掉私有 cause 里的诊断信息。
- **接受并测试固定**：保持现状，补一条测试把「fence 分支的链里有 `WORKER_FENCE_INVALID`、
  畸形命令分支没有」钉成契约的一部分。选它就等于宣布这个分支差异是有意的。

无论选哪个，都要补一条测试同时覆盖**两个分支**并断言各自的链——
今天的测试只看顶层 code，对这一类完全失明。

---

## 3. 验收

1. **新守卫在改动 tag 时变红。** 临时把 `"heal-review-v1"` 改成 `"heal-review-v2"`，
   确认守卫报错，然后改回。把输出贴进 §4。
2. **新守卫在新增未登记 digest 时变红。** 临时加一个带新 tag 的 digest 函数，
   确认守卫报错，然后删掉。**这一条是关键**——它是 `5ecfde2` 那次变更的真实形态。
3. `instance_command_digest_test.go` 现有的逐字段断言全绿（本流不改它的行为，
   只在需要时补充）。
4. `docs/refactor/digest-wire-tags.md` 里 §1.3 的清单与守卫的 inventory 逐条对齐。

门禁：

```bash
go test ./application/scheduling/... ./architecture/...
test -z "$(gofmt -l .)" && go vet ./... && go build ./... && go test ./... && go test -race ./...
```

---

## 4. 交接与完成记录

**给 W6：** `findings-consolidated.md` §三 R9 的表述需要改——
「编码未动，这是决策不是遗漏」不成立，实际是「两个保留、三个已改且未记录」。
本流不碰那个文件，把更正后的措辞写在这里由 W6 落。

**给 W3：** 本流只碰 `instance_command_services.go`，不碰 `decision.go` / `coordinator.go`。

**来自 W1：** §2 步骤 5 的 `validateAbort` 交接项。W1 不动这个文件，改由本流收尾。
若 W1 尚未落地，本流其余四步照做，步骤 5 留到最后。

**给 W2：** 若 evidence 结构体新增字段影响到 `encodeCanonical`
（`instance_snapshot.go:702-724`）的编码结果，那会移动快照 digest——
那是本流清单里的第一条 tag。W2 动手前应确认新增字段不进入快照编码路径；
若会进入，**停下来找本流对齐**，那是一次真正的存储迁移。

### 完成记录

**裁决 1 — 存量记录处理：选 A（记录并接受）。**
v0.6.0 尚未发布，若没有 Host 存着 v0.5 的取消/中止/重排幂等记录，这次破坏没有实际受害者。
已在 `docs/refactor/digest-wire-tags.md` 中记录失效方向，供发布说明引用。

**裁决 2 — 是否加 `-v2` 后缀：不改字节。**
字节的稳定性比命名的诚实重要，这正是本条缺口的教训本身。在注释与文档中说明版本号实际含义即可。

**裁决 3 — validateAbort 错误链分支不一致：未执行。**
依赖 W1 先落地 `WorkerFence.Validate()` 的 coded fault。本流跳过步骤 5（按 runbook 指示），
在报告 §5 交接段中给出精确改法建议，由人在 W1 落地后应用。

**验收 1（改 tag 变红）输出：**
```
=== RUN   TestW5DigestWireTagsAreRegistered
    digest_wire_tag_test.go:55: tag "heal-review-v1" no longer found in application/automation/heal_candidate_repository.go
    digest_wire_tag_test.go:71: found 5 files with wire tag literals in production code
    digest_wire_tag_test.go:91: untracked wire tag "heal-review-v2" in application/automation/heal_candidate_repository.go
--- FAIL: TestW5DigestWireTagsAreRegistered (0.35s)
```

**验收 2（新增未登记 digest 变红）输出：**
```
=== RUN   TestW5DigestWireTagsAreRegistered
    digest_wire_tag_test.go:71: found 5 files with wire tag literals in production code
    digest_wire_tag_test.go:91: untracked wire tag "test-untracked-digest-v1" in application/scheduling/instance_command_services.go
--- FAIL: TestW5DigestWireTagsAreRegistered (0.19s)
```

**清单最终条目数：7 条（5 个文件）。**
与 `w5wireTagInventory` 和 `docs/refactor/digest-wire-tags.md` 完全对齐。

**步骤 5（validateAbort）：未完成，依赖 W1。**
详见报告 §5 交接段。
