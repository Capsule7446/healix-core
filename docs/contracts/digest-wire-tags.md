# Digest Wire Tags

本文件登记所有持久化摘要域分隔标签（wire tag）。这些字节是存储格式的一部分：
更改一个标签会静默使所有已存摘要失效，且没有现有测试能发现——摘要只会变成另
一个同样合法的字符串。

## 完整清单

| 文件 | 标签字节 | 依赖方 | 本分支是否变更过 |
|---|---|---|---|
| `domain/execution/instance_snapshot.go:662` | `healix.run-snapshot` | 每一份已封存快照的 digest | 标签未变更，**但载荷在 v0.8.0 变过**（见下） |
| `application/scheduling/create_instance_service.go:21` | `create-run-request-v1` | 每一条已存的创建幂等记录 | 未变更 |
| `application/scheduling/instance_command_services.go:305` | `cancel-instance-request-v1` | 每一条已存的取消幂等记录 | `5ecfde2` 引入 |
| `application/scheduling/instance_command_services.go:306` | `abort-instance-request-v1` | 每一条已存的中止幂等记录 | `5ecfde2` 引入 |
| `application/scheduling/instance_command_services.go:307` | `reorder-queue-request-v1` | 每一条已存的重排幂等记录 | `5ecfde2` 引入 |
| `application/automation/heal_candidate_repository.go:17` | `heal-review-v1` | 每一条已存的 heal review 记录 | 未变更 |
| `application/automation/sampling_publication_transaction.go:16` | `sampling-publication-v1` | 每一条已存的 sampling publication 记录 | 未变更 |
| `application/execution/entry_completion_transaction.go:20` | `complete-entry-request-v1` | 每一条已存的 entry completion 幂等记录 | 本分支新增 |
| `application/execution/abort_request_transaction.go:17` | `request-abort-v1` | 每一条已存的 abort request 幂等记录 | 本分支新增 |

## `5ecfde2` 造成的存量失效

### 影响命令

- `CancelInstanceCommand`（取消）
- `AbortInstanceCommand`（中止）
- `ReorderQueueCommand`（重排）

### 失效方向

`5ecfde2` 将以上三条命令的摘要从 `sha256(json.Marshal(cmd))` 改为逐字段编码
+ 域标签。旧算法会因 `json.Marshal` 将未导出字段编码为 `{}` 而丢失实例身份；
新算法修正了这个问题，但代价是**所有旧摘要都无法匹配新摘要**。

后果：重放一条旧命令时，摘要不再命中已有记录 → 被当作新请求 → **重复执行**
一次取消 / 中止 / 重排。

### 为什么 snapshot 和 create 两个 tag 反而不能动

`HydrateInstanceSnapshot`（`instance_snapshot.go:171-180`）在重算 digest 对不上时
会返回 `EXECUTION_CREATE_INSTANCE_SNAPSHOT_CONFLICT`。若移动 `healix.run-snapshot`，
**所有**已存快照都会被判为冲突。同理，`create-run-request-v1` 的移动会失效所有
已存创建幂等记录——这两个的失效方向比前面三条更糟。

## v0.8.0 造成的快照失效（标签没动，载荷动了）

修复器策略新增了第十一个权重维度 `Framework`。它此前只存在于 `heal.Weights`：
评分器读它，而 `execution.HealerWeightsSnapshot` 与 `automation.HealerWeightsSnapshotV1`
都不携带它，`encodeHealerDigest` 也不哈希它。后果是**同一份快照 digest 下，
宿主可以给 `Framework` 设不同的值并得到不同的评分**——冻结策略要买的正是这个
不会发生，而它一直是部分成立的。

修复要求把第十一个权重纳入哈希，于是 `healix.run-snapshot` 下的载荷变了：

- `HydrateInstanceSnapshot` 对所有 v0.7 及更早的已存快照返回
  `EXECUTION_CREATE_INSTANCE_SNAPSHOT_CONFLICT`
- 失效范围与"移动 `healix.run-snapshot` 标签"完全相同

**注意这类变更 wire tag 守卫抓不到。** `TestW5DigestWireTagsAreRegistered`
比对的是标签字节，而标签字节没变。抓住它的是
`domain/execution/instance_snapshot_test.go` 里那个被钉住的 golden digest，
以及 `architecture/healer_weight_parity_test.go` 的两个维度对齐守卫。

## 规则

1. 任何 wire tag 的变更都必须同时提供迁移方案（重算或双读窗口），并在本文件登记。
2. **载荷变更同样适用第 1 条**，即使标签字节没动。判据是"已存 digest 还能不能
   重算命中"，不是"标签有没有改"。
3. 不得作为改名的副产品滑进去。
4. 本清单必须与 `architecture/digest_wire_tag_test.go` 的 inventory 逐条对齐。

## 守卫抓得到什么，抓不到什么

`TestW5DigestWireTagsAreRegistered` 有两个方向，强度不同。把它当成保证之前先读这一节。

**方向 1 — 已登记的 tag 被改动：可靠。** 清单里的字节必须在对应文件中原样存在，
改一个字符就变红。这一条不依赖任何命名约定。

**方向 2 — 新增未登记的 tag：启发式，有已知绕过。** 识别规则只有两条：
名字以 `DigestV1` 结尾的常量，以及 `e.str("healix.*")` 调用。因此它能抓住
`5ecfde2` 那种形态（新增 `cancelInstanceRequestDigestV1` 这类常量），但**抓不到**：

- 换个名字声明的常量。实测：`const w5probeTag = "zz-untracked-probe-v1"` 配一个把它
  写进 `sha256` 的函数，守卫通过，不报错。
- 直接内联进哈希调用的字符串字面量。

也就是说，方向 2 只覆盖「按现有约定写的新 digest」。一个不按约定写的新 digest 仍然可以
静默进来——这与 `conformance-audit.md` §6 记的「固定-Kind 辅助函数的识别是启发式的」
是同一类残余风险，结构性解法同样是把 tag 集中到一个受控的注册点，而不是靠扫描反推。

**方向 2 在并发修改时会静默跳过解析失败的文件**（`w5CollectAllWireTags` 的设计取舍，
至今未变、也是有意保留）。这在多 agent 并行期间是必要的，但意味着**守卫绿不代表扫全了**。
定稿前应在无并发的树上重跑一次。

扫描面同样要记清楚：方向 2 只看**非 `_test.go` 的 `.go` 文件**，且只跳过 `vendor/` 一处。
`architecture/` 不需要单独排除——该包只有 `_test.go`，已被前一条过滤掉。