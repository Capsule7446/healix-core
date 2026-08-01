# 缺陷整合与裁定

汇总五次独立扫描的全部发现，逐条给出裁定与依据。分支 `refactor/error-code`。

来源：
- **A1** unified-domain-language 全局审计（10 agent）
- **A2** Phase 2/3 逐属性审计（12 agent，含对抗性反驳）
- **C1** codex `gpt-5.6-luna` max（别名 / 非确定性 / 并发）
- **C2** codex `gpt-5.6-sol` max（同上三份）
- **C3** codex 第三轮（automation 侧非确定性与排序）
- **X1** 第一轮交叉审核（4 问 × 3 个模型层级，27 agent）
- **X2** 第二轮交叉审核（针对 X1 的修复本身，3 视角 × 3 层级，23 agent）

## 裁定标准

三条，全程一致适用：

1. **非确定性只有到达输出才算缺陷。** range 一个 map 本身不是问题；判据是输出（返回哪个错误、violation 顺序、返回 slice 顺序、digest 字节、选中哪一个）是否随迭代顺序变。比值、计数、布尔、存在性检查都是顺序无关的。
2. **别名只有引用能被调用方观察到才算缺陷。** 局部临时值不逃逸的不算；下游有完整深拷贝兜住的降级为加固。
3. **公共文本与私有 cause 是两套规则。** 契约禁止公共 message/params/violations 携带原因、值、身份；私有 cause 按设计可以携带细节。

裁定为「不成立」的条目一律写明依据，不省略——被排除的面和被发现的缺陷一样重要。

---

## 一、已修复

### F1 取消/中止命令摘要丢失实例身份（本轮回归）

- 位置：`application/scheduling/instance_command_services.go`
- 根因：`canonicalDigest` 用 `json.Marshal`。坐标值对象只含未导出字段，Go 编码为 `{}`。
- 实测：`digest(instance-a) == digest(instance-b) == sha256:c7e4dfb5…`
- 后果：同 CommandID 取消两个**不同实例**摘要相同；Host 用它做重放检测会把 A 的结果返给 B。
- 修法：三个摘要改为逐字段长度前缀编码，各带域分隔标签。
- 守卫：逐字段变异断言摘要必变；对旧实现在 `instance_id` 一项失败。
- 提交：`5ecfde2`

### F2 弃用守卫永不可能失败

- 位置：`architecture/dependencies_test.go` `walkProductionGo`
- 根因：`parser.ParseFile(..., 0)` 丢弃注释，`ast.File.Comments` 恒为 nil。
- 修法：`parser.ParseComments`。注入 `// Deprecated:` 验证能正确报错。
- 提交：`5ecfde2`

### F3 fingerprint 深拷贝散落五份，两份是浅的

- `cloneUnpublishedFlowFragment` **完全没拷** `Fingerprint`
- `cloneSpec` 漏 `Framework`
- `ParameterDefinition.Options` 两处浅拷贝，其中发布 mapper 那处导致**已发布的不可变版本与可变草稿共享底层数组**
- 修法：`fingerprint.Fingerprint.Clone()` 成为唯一一份；`automation.CloneParameterDefinitions` 导出。
- 守卫：拒绝 `domain/fingerprint` 之外任何「取 Fingerprint 返回 Fingerprint 且名含 clone/copy」的函数，以及任何逐字段组装 Fingerprint 的复合字面量。**一跑就抓出第五份**（`heal_candidate_repository.go:327`）。
- 提交：`92b643e`、`ce3277e`

### F4 四处 map 顺序决定报告哪个失败

- `heal_review_service.go` 构造器（多个 nil 依赖）
- `create_instance_builder.go` 命令身份检查（多个空白字段）
- `execution/validation.go` 参数快照未知键
- `execution/validation.go` 绑定校验——最重的一处，四个分支提前返回，两个坏绑定时连**失败种类**都会变
- 依据：项目已在 `3e56ba2` 裁定这一类是缺陷，其提交信息写明「同一个 commit 在不同运行上会被不同的错误拒绝」，并把确定性列为「给这个信封稳定 code 的前置条件」。
- 守卫：重复调用 200 次断言错误文本恒定。单次调用永远通过，这是唯一能抓住它的断言形式。
- 提交：`2c7251a`

### F5 heal 决策把调用方的 fingerprint 交出去

- `DefaultHealer.Heal` 是导出方法，Host 保留自己的 DOM snapshot。每个 `Candidate` 直接带走 snapshot 的 map 与 slice；`Best := scored[0]` 是浅拷贝，与 `Candidates[0]` 第二次共享同一份。
- 提交：`2c7251a`

### F6 `HealStreak` 的两处应用层拷贝漏掉 `ConsumedObservations`

- 域内 `clone()` 两个字段都拷，但**未导出**，所以 application 层两处各自手搓，都只拷了 `Contributions`。
- 后果：治理决策能反向改写它自己派生自的那份快照。
- 修法：导出 `HealStreak.Clone()`，两处调用方委托。
- 提交：`2c7251a`

### F7 automation 侧的同类非确定性 + `Seal` 排序无全序

- `EnvironmentVariables.Validate`、`validateReferenceBindings` 排序后再校验
- `Seal` 的 `sort.Slice` 只比 `SequenceNumber`。`Validate` 先拒重复序号所以今天触达不到，但补 `ID` 作次级键让排序不再依赖远处的一条不变量。
- 提交：`94dd993`

### F8 发布请求摘要对参数值完全失明

- `SamplingPublicationRequestDigest` 用 `json.Marshal` 整个载荷；三个参数类型编码为 `{}`
- 后果：改掉参数默认值后用同一 `PublicationID` 重发 → 摘要命中 → 返回上一次结果且 `err=nil`。**用户以为改动已发布，实际从未写入。** 同时击穿停止条件「同命令 ID 不同内容必须冲突」
- 首次尝试把 `MarshalJSON` 加到域类型上，**被架构守卫正确拒绝**（domain 不得知晓 JSON 编码）。编码是应用层关注点，规范遍历因此落在应用层，经公共访问器读取那三个类型
- 遍历保留了 `json.Marshal` 免费给的两条性质：每个变长值带长度前缀（边界不能在字段间移动），map key 排序（无序容器不能让摘要依赖迭代顺序）。结构体先写字段数，所以新增字段会改变摘要而不是滑进邻居的字节
- **摘要值会变。** 原值是错的（会碰撞），但已存的发布幂等记录将不匹配，属 Host 侧事项
- 提交：`097be65`

### F9 发布路径的三处校验缺口

- **孤儿 `ElementTargetVersionID`**：规则单向，所有下游闸门以 `ElementTargetID != ""` 为条件，navigate/press 步骤上残留的版本 ID 一路进入正式版本。Wait/Repeat/FragmentRef 早已直接拒绝
- **REUSE 跳过内容校验**：`Publish` 接受调用方自建载荷，是绕过 mapper 兜底的第二条路径。改为校验后暴露出跳过的**真实原因**——REUSE 投影故意让 `Current` 是选中的历史版本、`CurrentVersionID` 是活指针，整聚合规则由构造决定不可能成立。`FlowFragmentVersion.ValidateFor` 已为同一形状确立解法，补上对称的 `ElementTargetVersion.ValidateFor`
- **fingerprint 校验漂移**：`assets.go` 重写检查而非委托，漏掉 `SiblingIndex >= 0` 与 Framework。采样期会拒的负索引能落进不可变版本再被 heal 打分读回。消除重复需要导出违例追加器，范围大于缺口本身，因此补齐两条规则并加**一致性守卫**：两个校验器必须接受和拒绝同一批 fingerprint
- 提交：`7bd9061`

### F10 交叉审核发现的四条（修复自身的缺陷）

X1 提出 28 条候选，22 条被反驳，6 条存活。**存活的没有一条是那八个提交造成的行为回归**——两条是同类漏网、两条是新测试的覆盖缺口、一条是我的提交卫生事故、一条是守卫的匹配洞。

- **`ownStepTransitionCommit` 的 JSON 往返抹平全部身份**——同一类的第三个实例，且此前从未被看过。交给 Host 的 owned 副本里 step execution id、entry id、每条观察的身份全为空，`ValidationGroupTerminalObservation` 的未导出 `expectedMembers` 也丢失。校验跑在**原件**上、发生在往返之前，出口无人复查——owned 自己都过不了 `Validate`。字节预算同样错：它量的是 `len(json.Marshal)`，把每个坐标算成 `{}` 的两字节。改为 `StepTransitionCommit.Clone()` + 走值计算字节。提交：`85c72f6`
- **`validateSuppliedRootValues`**——在 `2c7251a` 改的那个 map 下方 45 行、同一文件，仍是 map 顺序，且两个提前返回**错误种类不同**（无 code vs 带 code），同一输入的 fault code 会翻转。提交：`f681945`
- **两个我写的空转测试**。`TestEnvironmentVariablesReportTheSameBadNameEveryRun` 用三个坏名字，而 `ValidateName` 的错误不带参数名，三者产出逐字节相同的字符串——200 次循环在拿常量比自己，删掉它守护的 `sort.Strings` 照样绿。`TestPublicationDigestIsStableAcrossRuns` 的载荷里没有任何多条目 map，删掉 canonical walk 的键排序全仓绿。均已重写并变异验证。提交：`f681945`
- **提交卫生事故**：`09b6816` 的标题描述的守卫不在提交里——审核 agent 正在往工作树写探针，我的 `git add -A` 扫进了它们的文件，我的守卫反而未跟踪。其中一个探针是 `func cloneFingerprint(*fingerprint.Fingerprint) *fingerprint.Fingerprint`，**恰好暴露了 `TestFingerprintHasExactlyOneDeepCopy` 只认值签名、漏掉指针形式**。已清除探针、落地真守卫、并让守卫解包 `StarExpr`。提交：`a4a1973`

### F11 把 JSON 摘要这一类整体钉住

本模块三处 `json.Marshal` 摘要里两处出过同一个缺陷。剩下两处（`HealReviewRequestIdentityDigest`、`HealReviewStreakDigest`）目前正确，因为载荷全是导出字段——而这正是出事前那两处的状态。守卫现在遍历全部三个载荷的类型图，出现零导出字段类型即失败。提交：`3eb6d21`

### F12 第二轮交叉审核的三条

X2 提出 17 条，13 条被驳倒，**3 条存活**（1 medium / 2 low），无一是修复引入的回归。

- **`validateCreateInstanceCommand` 的 fault 链会翻转**（medium）。两个分支返回的**失败种类不同**：畸形 item id 是无 code 的裸 error，坏参数值带 `PARAMETER_VALUE_INVALID`。外层信封把两者都包成 `CodeCreateInstanceCommandInvalid`，所以 `CodeOf` 与渲染文本纹丝不动——**没有任何既有测试能看见它**。但 `fault.IsCode` 是 `errors.Is`、走整条链，于是同一份字节的输入，`IsCode(err, parameter.CodeValueInvalid)` 时真时假。按更深的 code 分支的 Host 会把同一请求路由到两条路上。`domain/fault` 的注释里亲自点名了这个陷阱。
- **application 层是未清扫的类，不是漏网单点**（low）。X2 另点了四处同型：环境变量遍历、两处预算遍历、以及 service 侧的已存计划比对。前两轮各自逐个修，各自漏了兄弟。现在把有序迭代命名为 `sortedKeys` 一次，所有站点统一使用。
- **`TestOwnedCommitKeepsEveryIdentity` 部分空转**（low）。共享 fixture 只有 Event，两个遍历观察项的循环零次迭代，而"值为空即无意义"的自检对空集合不触发。已换成带观察项的 fixture 并拒绝在无观察项时运行。

提交：`2cb10f3`

### F13 字节预算的量纲漂移（自查）

把 `len(json.Marshal)` 换成走值遍历后，**量纲变了而上限没变**。实测同一 commit：JSON 340 字节 vs 走值 75 字节，倍率 4.53——`1<<20` 静默放宽了约 4.5 倍，而全部测试照过，因为它们断言的是"边界被强制"而非"边界在哪"。改为 `1<<18` 并在常量与遍历上都标注单位。同时给遍历加深度上限：旧的 `json.Marshal` 遇环报错，新的会爆栈。提交：`34b5f3b`

---

## 二、裁定为不成立（附依据）

| 来源 | 条目 | 依据 |
|---|---|---|
| C1 | `keyAttrsFor` map 顺序影响输出 | 三个消费方 `simKeyAttrs`、`simKeyAttributes`、`hasKeyAttrs` 全是 `matched/总数` 比值或布尔，**顺序无关**。其输出到不了 violation 或 digest。C1 自己也写了「评分结果数值不受影响」，后半句断言无依据 |
| C1/C2 | `Runtime` 单线程假设只有注释 | 观察准确。但计划把并发调度明确列为延期能力，这是 port 契约约束不是遗漏。强制它要给 `Runtime` 加锁或时钟端口，属于公共 API 变更 |
| C2 | `ObservedAtMS` / `DurationMS` 读墙钟致 Evidence 不可复现 | 记录「何时观察到」「耗时多久」本就需要时钟；时间戳不变才是 bug。分层上 `domain/node` 15 处 `time.Now()` 与 `application/automation` 的注入式 `ReviewClock` 不一致，值得讨论，但不是缺陷 |
| C1 | `mapNodes` 破坏 Plan 不可变性 | 遗漏属实已修，但严重度非 high：`SealInstanceSnapshot` → `cloneNodes` 自己会 `Fingerprint.Clone()`，别名活不过 builder，`buildExecutionDraft` 未导出且中间产物不逃逸 |
| A1 | evidence/node「完全没采纳 EntryID/InvocationPath」 | EntryID 与 StepExecutionID 在 `ff00476` 已采纳；未采纳的只有 InvocationPath |
| C3 | `EnvironmentVariables.Validate` 严重度 high | 公共 code 与消息固定，`Environment.Validate` 压成一条 `variables` violation，仅私有 cause 变。已按类别一致性修复，但不是 high |
| 自查 | 快照编码器浮点面 | NaN/Inf 在封存路径对 caps 与全部权重被拒；`normalizeHealerZeros` 实测把 `-0.0`（`0x8000000000000000`）规整为 `+0.0`。干净 |
| 自查 | 公共文本泄漏 | `fault.New`/`fault.Wrap` 消息无格式化动词；`mustViolation` 的 `%d` 是契约允许的 0-based 索引，`%q` 均在私有 cause |
| A2 | `mapSamplingNode` 不拷 Selectors/Fingerprint | `NewElementTarget` 内部 `cloneNodeAggregate` 兜住 |
| A2 | `execution/validation.go:306` spec 别名 | 局部值仅供 `spec.Validate()`，不逃逸 |
| X1 | 22 条候选被对抗性反驳驳倒 | 多为"下游已有兜底""调用方不可达""严重度虚高" |
| X2 | 13 条候选被驳倒 | 含三个 lens 各报一次的 `HealObservation.Fingerprint` 别名——该字段全树无人写、无人读、Validate 不看，所报的失败场景实际会 nil-map panic。另有数条是对着过期工作区状态写的 |

两轮反驳环节合计把 45 条候选压到 9 条真发现。**单轮审核的误报率是 80%**——这是必须配对抗性验证环节的直接依据，也是我不逐条复核就不采信任何审计结论的原因。

---

## 三、未修复（已证实，按阻塞度排序）

### R5 EntryExecutor 在授权前创建 BrowserSession —— 高

- `Execute` 只做 `fence.Validate()`（非空形状检查）；真正的租约校验在 `engine.go:129`，发生在 `factory.Create` 之后
- 过期或伪造的 claim 会实打实开一个浏览器再关掉

### R6 Evidence 缺 InvocationPath，retry 无独立 occurrence —— 高

- 同一 Entry 内经不同调用点两次调用同一 fragment，证据无法区分
- retry 只体现为 `OperationObservation.Attempt`，而它不带任何 step-execution 身份

### R7 Scheduling 从不提交 Entry 终态 —— 高

- 唯一的 transition 产出点只做 Pending→Skipped

### R8 `UnpublishedFlowFragment.Saved*` 三字段 —— 中

- 未发布资产上的正式身份，全仓无读无写，与 `SamplingPublicationResult` 重复
- 现有守卫因精确字符串比较（`"VersionNumber"` vs `SavedVersionNumber`）抓不到

### R9 Phase 6「Snapshot/Evidence 新编码」—— 决策项

- 术语改名已完成，编码刻意未动：`"healix.run-snapshot"` 与 `"create-run-request-v1"` 保留原字节
- 计划第 6 行验收写着「无旧 digest」，严格说未达标。改它会作废所有已存快照与幂等记录，需要 Host 侧迁移，应是独立步骤

---

## 四、需要裁决

**REUSE 语义：计划与代码实质冲突。** 计划说 REUSE 固定采样时的具体 Version、之后出新版不影响发布；代码要求整聚合 CAS（`ExpectedRevision` 与 `ExpectedCurrentVersionID` 都须匹配），且 `conformancetest/suite.go:135-136` 把「reuse revision 过期必须 AUTHORITY_CONFLICT」写成了适配器强制契约。

两人对同一页面采样、一人先发布，后者会被判失败。改代码要改那条 conformance 契约，改计划要承认 REUSE 不是「固定」。这是决策不是编辑。

---

## 五、方法论记录

三处值得留档，因为它们解释了为什么这些缺陷能长期存在：

1. **假绿的测试比没有测试更糟。** `TestDraftEditingNeverAliasesStepContentWithItsSource` 直接调用了出问题的函数，但 fixture 里既没 Parameters 也没 Nodes，被测的两个分支从未执行。同理弃用守卫扫的是空集。
2. **修实例不等于消除类别。** fingerprint 修了两处之后仍有三处；结构守卫一跑就抓出第五份。
3. **一次调用永远通过。** map 顺序缺陷只能靠重复调用断言，单次调用总会选中某一个。
4. **公共契约稳定不等于没有非确定性。** `validateCreateInstanceCommand` 的顶层 code 和消息全程不动，唯一会变的是错误链深处——而 `IsCode` 走整条链。只断言顶层 code 的测试对这类缺陷完全失明。
5. **换度量要连同阈值一起换。** 预算从 JSON 长度改成内容字节后，`1<<20` 就不再是同一个意思了。所有测试断言的是"边界被强制"，没有一条断言"边界在哪"，所以 4.5 倍的放宽是静默的。
6. **收敛是可观测的。** 两轮交叉审核：28 条 → 6 条真发现，17 条 → 3 条真发现，且第二轮的三条全部是第一轮已知类的旁支，无新类出现。
