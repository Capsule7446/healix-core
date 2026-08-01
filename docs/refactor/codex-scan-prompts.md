# Codex 对抗式扫描 prompt

三次失败的成因与对策：

| 失败 | 成因 | 对策 |
|---|---|---|
| 改了仓库文件，还把测试期望改成迎合自己的改动 | `--sandbox workspace-write` | 必须 `--sandbox read-only` |
| 读完文件直接 exit 0，没有报告 | 预算全花在探索上 | 硬性工具调用上限 + **边查边输出**，不许攒到最后 |
| 范围太宽（七类扫全模块） | 一次一个目标 | 三份 prompt 分开跑 |

最重要的一条是**边查边输出**。前两次都是"读完再写"，预算耗尽时一个字都没落地。下面的 prompt 要求每确证一条就立刻打印，报告是增量长出来的。

## 命令

```bash
CODEX="/c/Users/Paul/AppData/Local/OpenAI/Codex/bin/69066b736e1e17a4/codex.exe"
cd /c/Users/Paul/workspace/healix-core.worktrees/refactor-error-code
"$CODEX" exec -c model=gpt-5.6-luna -c model_reasoning_effort=max \
  --sandbox read-only --skip-git-repo-check - < PROMPT_FILE > scan-out.log 2>&1
```

`~/.codex/config.toml` 里已经是 `gpt-5.6-luna` / `max`，`-c` 只是显式覆盖，防止配置被改过。

---

## 共用前言（每份 prompt 都以这段开头）

```
你在 github.com/Capsule7446/healix-core 的工作树里（分支 refactor/error-code）。
Go，纯 stdlib，go.mod 零 require。六边形/DDD：application/* 依赖 domain/*，domain 只依赖
stdlib 或其他 domain 包。

## 三条硬约束

1. 只读沙箱。不得写、改、暂存、提交任何文件。你唯一的产出是打印到 stdout 的文本。
   （上一轮扫描无视这条，开始改 step.go 和测试期望，全部被回退，整轮作废。
   你的任务是报告缺陷，不是修复缺陷。）

2. **边查边输出。** 每确证一条缺陷，立刻按下面的格式打印出来，然后再继续查下一条。
   不要攒到最后一次性输出。上一轮扫描把预算全花在读文件上，退出时一个字都没写。
   宁可只报三条完整的，也不要读了二十个文件后什么都没报。

3. 工具调用不超过 12 次。到达上限立刻停止查证，把已有结论写完。
   永远不要跑不带路径过滤的 git diff、git diff master..HEAD，或任何会 dump 整个 diff
   的命令——更早的一轮就是这么把上下文烧穿的。

可以跑的只读命令：rg、go build ./...、go vet ./...、go test ./...、gofmt -l .，
以及带路径过滤的 git log / git show <commit> -- <path>。

## 每条缺陷的输出格式

### 标题（一行说清是什么）
- 位置：<file:line，必须精确，没有行号的发现视为无效>
- 错在哪：2-3 句
- 怎么触达：具体调用路径
- 后果：用户或 Host 实际观察到什么
- 凭据：让这条成立的推理或命令输出
- 修法：具体到文件和改动
- 严重度：blocking | high | medium | low

查不实的放到最后一节"存疑未证"，并写明差什么才能定论。不要为凑数把存疑的写进确证里。

## 已知项——不要重复报告

K1. 坐标值对象（InstanceID/EntryID/StepExecutionID/InvocationPath）与所有
    parameter.Value/OptionalValue/Binding 都是只含未导出字段的结构体，json.Marshal
    一律编码成 {}。已在 instance_command_services.go 修复；
    sampling_publication_transaction.go 的 SamplingPublicationRequestDigest 未修。
K2. 手写深拷贝漏引用字段。fingerprint 四份、两份是浅的——已收敛为 Fingerprint.Clone。
    ParameterDefinition.Options 两处浅拷贝（含发布 mapper，导致已发布不可变版本与
    可变草稿共享数组）——已修。
K3. 永不可能失败的守卫。walkProductionGo 用 parser mode 0，ast.File.Comments 恒为 nil，
    弃用守卫是空转——已修。其他守卫用精确字段名匹配，漏掉近似拼写
    （SavedVersionNumber vs VersionNumber）。
K4. 不变量只在一条路径上成立。REUSE 节点跳过 ElementTargetAggregate.Validate()，
    而 Publish 接受调用方自建的 SamplingPublication，绕过 mapper 兜底。未修。
K5. 单向校验。ElementTargetID != "" 才要求 ElementTargetVersionID，无反向检查，
    孤儿版本 ID 可进入正式版本。automation.ElementTargetAggregate.Validate 重写了
    fingerprint 的检查，漏掉 SiblingIndex >= 0 和 Framework 校验。未修。
K6. 测试 fixture 没填充它声称覆盖的分支。
K7. EntryExecutor 在真正授权前就创建 BrowserSession；Scheduling 从不提交 Entry 的
    Running→终态；Evidence 不带 InvocationPath，repeat/retry occurrence 无法独立标识。
K8. 快照规范编码器的浮点面已查干净：NaN/Inf 在封存路径被拒（environment_snapshot_
    validation.go:105-118），normalizeHealerZeros 确实把 -0.0 规整为 +0.0。不要再查。
K9. fault.New / fault.Wrap 的公共消息里没有格式化动词；mustViolation 里的 %d 都是
    契约允许的 0-based 集合索引，%q 都在私有 cause 里。公共文本泄漏这一类已查过。

把 K1-K9 当作"这个代码库会犯哪些种类的错"的分类法，去找同类的**其他实例**，
以及没人查过的种类。不要复述 K1-K9 本身。
```

---

## Prompt 1：别名与所有权（最高价值）

K2 只修了 fingerprint 和 ParameterDefinition 两处。同类的其余实例大概率还在。

```
目标：exported 边界上的引用泄漏。

Core 的契约是「返回值归调用方所有，内部状态不可被调用方改动」。找出每一处违反：

1. 返回 slice、map，或含有 slice/map 的结构体的**导出方法**，其返回值与接收者共享底层
   数组或 map。重点看名字里没有 Clone/Copy 的那些——真正的漏洞在"看起来像取值器"的方法上。
2. 存储调用方传入的 slice/map 而不复制的**导出构造函数或 setter**。调用方之后改动自己
   那份，就改到了已构造对象的内部。
3. **只深了一层的 Clone()**。复制了外层 slice，但元素里的 slice/map 仍共享。
4. 已封存/不可变的类型（Plan、InstanceSnapshot、已发布的 Version）把内部结构交出去。
   这一类最严重：不可变性是它们存在的理由。

先跑这两条把候选面铺开，再逐个读：

  rg -n "func \([a-z]+ [A-Z]\w*\) [A-Z]\w*\(\) (\[\]|map\[)" --type go -g '!*_test.go'
  rg -n "^\t[A-Z]\w+ +(\[\]|map\[)" --type go -g '!*_test.go' domain/ application/

对每个候选，判断它是否已经复制。只报告确实共享引用的。
对每条给出具体的变异序列：调用方拿到 X 后改 Y，接收者的 Z 随之改变。
```

---

## Prompt 2：非确定性进入输出

已知 `domain/evidence/commits.go` 有过一处 map 迭代顺序决定返回哪个错误（`3e56ba2` 修的）。同类的其余实例。

```
目标：同一输入产生不同输出。

1. 对 map 的 range，其迭代顺序能影响：返回哪个 error、violation 的排序、返回 slice 的
   顺序、digest 的字节、或"选中哪一个"。判据是**输出**是否依赖顺序，不是是否 range 了 map。
   一个 range map 只做累加或只做存在性检查是安全的；一个 range map 里带 return 或
   append 到返回值的，需要逐个查。

2. sort.Slice 的比较函数不是全序：相等元素的相对顺序未定义（sort.Slice 不稳定）。
   找出用 sort.Slice 而非 sort.SliceStable、且比较键可能相等的地方。

3. time.Now()、goroutine 调度、select 多路就绪，任何进入返回值或 digest 的。

4. 浮点累加顺序影响结果的地方（K8 已查过 healer 权重，不要再查那处）。

先跑：
  rg -n "for .*range " --type go -g '!*_test.go' domain/ application/ | rg -v "range \[\]"
  rg -n "sort\.Slice\(" --type go -g '!*_test.go'

契约要求：violation 的顺序必须是输入的函数，绝不能由 map 迭代驱动。
对每条给出：同一输入的两次运行分别会产生什么。
```

---

## Prompt 3：并发与状态机

`go test -race` 是绿的，但无竞态不等于正确。

```
目标：race detector 抓不到的并发缺陷。

1. check-then-act：先检查条件再动作，中间状态可能已变。尤其是 fence/claim/lease 的
   校验与随后的副作用之间。
2. 多步状态迁移不是原子的：中途失败会留下半完成状态，且没有补偿。
3. Host 实现的接口（Driver、Facts、Recorder、各种 Repository、Transaction）背后共享的
   map 或 slice，Core 侧假定单线程但没有任何东西强制。
4. 文档写着"单顺序执行器访问"的假设——找出这些注释，然后确认有没有任何代码强制它。
   一个只靠注释维持的不变量，在 Host 换实现时就会破。
5. context 取消后仍继续的副作用；defer 里的清理在 panic 路径上被跳过的。

起点：
  rg -n "单个顺序执行器|single sequential|not safe for concurrent|goroutine" --type go
  domain/node/runtime.go 与 application/execution/entry_executor.go 的生命周期

对每条说明：需要什么样的交错才会出问题，以及 Host 有多容易无意中制造出这种交错。
```

---

## 建议跑法

一次一个，各存一份日志：

```bash
"$CODEX" exec -c model=gpt-5.6-luna -c model_reasoning_effort=max \
  --sandbox read-only --skip-git-repo-check - < prompt1.md > scan-aliasing.log 2>&1
```

跑完把日志给我，我来核实每条（前几轮的经验是审计结论有夸大，我不会不复核就采信），
然后和已有的两份审计合并做统一修复。
