# 摘要字段标签清单

本文件登记所有持久化摘要域分隔标签（wire tag）。这些字节是存储格式的一部分：
更改一个标签或其载荷编码都会使已存摘要无法命中；摘要仍是合法字符串，普通测试无法自动发现该兼容性破坏。

## 完整清单

| 文件 | 标签字节 | 依赖方 | 当前状态 |
|---|---|---|---|
| `domain/execution/instance_snapshot.go` | `healix.run-snapshot` | 每一份已封存快照的 digest | 当前标签 |
| `application/scheduling/create_instance_service.go` | `create-run-request-v1` | 每一条已存的创建幂等记录 | 当前标签 |
| `application/scheduling/instance_command_services.go` | `cancel-instance-request-v1` | 每一条已存的取消幂等记录 | 当前标签 |
| `application/scheduling/instance_command_services.go` | `abort-instance-request-v1` | 每一条已存的中止幂等记录 | 当前标签 |
| `application/scheduling/instance_command_services.go` | `reorder-queue-request-v1` | 每一条已存的重排幂等记录 | 当前标签 |
| `application/automation/heal_candidate_repository.go` | `heal-review-v1` | 每一条已存的 heal review 记录 | 当前标签 |
| `application/automation/sampling_publication_transaction.go` | `sampling-publication-v1` | 每一条已存的 sampling publication 记录 | 当前标签 |
| `application/execution/entry_completion_transaction.go` | `complete-entry-request-v1` | 每一条已存的 entry completion 幂等记录 | 当前标签 |
| `application/execution/abort_request_transaction.go` | `request-abort-v1` | 每一条已存的 abort request 幂等记录 | 当前标签 |

## 当前摘要兼容边界

- `SealInstanceSnapshot`、创建实例命令和各事务命令都使用表中固定的域分隔标签与规范化字段编码。
- `HydrateInstanceSnapshot` 会重新计算摘要；标签或参与摘要的载荷字段不一致时返回
  `EXECUTION_CREATE_INSTANCE_SNAPSHOT_CONFLICT`，宿主不得把该失败当作可重试的普通冲突。
- 命令摘要用于幂等回放；相同命令身份和摘要才允许复用既有结果，摘要不同必须按新请求处理并由应用层返回冲突。
- 载荷新增字段、调整字段顺序或修改编码均视为持久化格式变更，必须提供重算或双读策略后才能上线。

## 规则

1. 任何 wire tag 的变更都必须同时提供兼容处理（重算或双读窗口），并在本文件登记。
2. **载荷变更同样适用第 1 条**，即使标签字节没动。判据是"已存 digest 还能不能
   重算命中"，不是"标签有没有改"。
3. 不得因符号命名调整而改变摘要字段。
4. 本清单必须与 `architecture/digest_wire_tag_test.go` 的 inventory 逐条对齐。

## 守卫覆盖范围

`TestW5DigestWireTagsAreRegistered` 校验已登记标签，并按命名约定扫描新增标签；守卫结果仍需结合本表人工复核。

**方向 1 — 已登记标签被改动：可靠。** 清单中的字节必须在对应文件中原样存在，改一个字符就会失败。

**方向 2 — 新增未登记标签：启发式。** 扫描名字以 `DigestV1` 结尾的常量和 `e.str("healix.*")` 调用，因此不覆盖任意命名的常量或直接内联的字符串：

- 使用其他名称声明的常量可能漏检。
- 直接内联进哈希调用的字符串字面量不会被识别。

方向 2 只覆盖按现有约定编写的新 digest；未按约定编写的 digest 仍可能静默进入，结构性缓解方式是把标签集中到受控的注册点，而不是依赖扫描反推。

并发修改导致解析失败时，方向 2 会跳过该文件；因此**守卫通过不代表扫描完整**。定稿前应在无并发修改的树上重跑完整测试。

扫描面同样要记清楚：方向 2 只看**非 `_test.go` 的 `.go` 文件**，且只跳过 `vendor/` 一处。
`architecture/` 不需要单独排除——该包只有 `_test.go`，已被前一条过滤掉。
