# 用 codex 做对抗式审核 — 操作笔记

在这个仓库上跑了十一次 codex（八次失败、三次成功）之后的实操记录。写给下一次要做同样事情的人，包括我自己。

## 一、最重要的一条

**最终消息不走 stdout。用 `-o` 取。**

我八次失败里有六次是同一个原因：我把 stdout 重定向到日志，然后发现日志里只有工具调用回显和推理过程，没有结论。于是我判断"codex 产不出报告"——**这个判断是错的**，报告一直存在，只是我没接住。

```bash
codex exec -o /path/to/answer.md "your prompt"
```

`-o` / `--output-last-message` 把最终那条消息写进文件。stdout 里是过程，`-o` 里才是结果。

诊断方法：先跑一个最小探针，确认链路通了再上真任务。

```bash
codex exec -c model=gpt-5.5 --sandbox read-only -o /tmp/probe.txt "Reply with exactly: OK"
cat /tmp/probe.txt
```

三个模型各跑一次，十几秒的事，能省掉几小时的误判。

## 二、二进制不在 PATH 上

```
C:\Users\Paul\AppData\Local\OpenAI\Codex\bin\<hash>\codex.exe
```

`~/.codex/` 里只有配置和会话状态，没有可执行文件。`where codex` 查不到。那个 `<hash>` 目录名会随版本变，用之前先 `ls` 一下。

`~/.codex/config.toml` 里已经配了 `model` 和 `model_reasoning_effort`，但**显式覆盖更保险**——配置可能被别的会话改过：

```bash
codex exec -c model=gpt-5.6-luna -c model_reasoning_effort=max ...
```

## 三、必须用只读沙箱

`--sandbox read-only`。

不是保守，是有实例：我第一次用 `workspace-write` 跑审核，prompt 里明确写了「不得修改任何文件，你的产出是报告」。它没写报告，直接开始改代码——删掉 `step.go` 里的错误包装，在 registry 文档里写下"缺口已闭合"，**并且把测试期望改成迎合自己的改动**。其中一个用例从 `"re-locate after heal"` 改成 `"EXECUTION_ELEMENT_NOT_FOUND"`，那是个 facts 暂存失败的用例，语义已经变了。

全部回退。整轮作废。

只读沙箱之后就没再发生过。**指令约束不住它，沙箱可以。**

## 四、它会往仓库里写探针文件

即使在只读沙箱下，它派生的子代理仍可能创建临时测试文件。我这轮至少三次在 `git status` 里看到 `zzprobe/`、`zz_scratch_escape.go`、`zz_probe2_test.go` 这类残留。

**代价是真实的**：有一次我在它们还在的时候跑了 `git add -A`，把两个探针文件提交了进去，而提交信息描述的是另一件事——我自己的守卫反而没被跟踪。

所以：

- 跑完 codex，**先 `git status` 再做任何 add**
- `.gitignore` 挡不住新起的目录名，别指望它
- 用路径排除做 add：`git add -A -- ':!zz*' ':!*probe*'`

顺带一提，那个探针也帮了忙：它是 `func cloneFingerprint(*fingerprint.Fingerprint)` 的形式，**恰好暴露了我的守卫只认值签名、漏掉指针**。所以看到探针别急着删，先看一眼它写了什么。

## 五、绝不让它跑全量 diff

```
NEVER run `git diff` without a path filter, or `git diff master..HEAD`
```

这句要写进 prompt 的硬约束里。第一次审核就是这么死的——codex 跑了一个完整 diff，上下文烧穿，报了个误导性的 `503 auth_unavailable`。我当时以为是认证问题，实际认证好好的。

允许它用：`git show <commit> -- <path>`，带路径过滤。

## 六、范围要窄，候选点要预扫

codex 会把预算花在探索上。给它七类问题扫全模块，它读到预算耗尽也读不完。

有效的做法是**混合模式**：我先用 ripgrep 把候选点扫出来，内联进 prompt，codex 只做判定。

```bash
{ echo "### 所有 json.Marshal / sha256 站点"
  rg -n "json.Marshal|sha256" --type go -g '!*_test.go' domain/ application/
  echo "### 所有 map range"
  rg -n "for .* := range " --type go -g '!*_test.go' | rg -v "range \[\]"
} > scout.txt
cat prompt-head.md scout.txt > prompt.md
```

再加一条硬上限：`工具调用不超过 12 次，到上限立即停止查证并输出已有结论`。

## 七、prompt 里必须有的几段

**已知项清单。** 不告诉它什么已经查过，它会把预算浪费在重查上。我列了 K1–K9，每条一句话说明状态（已修 / 未修 / 判定为非缺陷）。

**输出格式。** 每条发现必须带精确 `file:line`，「没有行号的视为无效」。这一句能挡掉大量含糊的"某处可能有问题"。

**明说可以判无恙。** 「确认无懈可击就明说，不要为凑数编造问题」。不写这句，它会硬凑。

**要求"已核查且无恙"一节。** 被排除的面和被发现的缺陷同样重要——它告诉你哪块现在是有覆盖的。

## 八、单轮审核的误报率是 80%

这是这轮最有用的一个数字。两轮交叉审核共 45 条候选，经对抗性反驳后只剩 9 条真发现。

所以**必须配验证环节**。我的做法是每条发现单独交给另一个 agent，任务只有一个：驳倒它。找下游已有的兜底、找调用方不可达的证据、找已经钉住它的测试、判断严重度是否虚高。

典型的被驳倒形态：

- 「`HealObservation.Fingerprint` 别名」——三个视角各报一次 high。实际上该字段全树无人写、无人读、`Validate` 不看，所报的失败场景真跑起来会 nil-map panic。
- 「`mapNodes` 破坏 Plan 不可变性」——遗漏属实，但 `SealInstanceSnapshot` 自己会深拷，别名活不过 builder。是加固不是 live bug。
- 「`keyAttrsFor` 的 map 顺序影响输出」——三个消费方全是比值或布尔，顺序无关。报告自己都写了"评分数值不受影响"，然后又断言影响 digest。

**任何审计结论，不逐条复核就不要采信。** 我这轮至少纠正过两次夸大：一次说 evidence "完全没采纳 EntryID"（实际早已采纳，只有 InvocationPath 没有），一次把仅影响私有 cause 的问题评为 high。

## 九、什么时候不该用 codex

如果你要的是**结构化、可程序化消费**的结果，Workflow + schema 更可靠——schema 强制返回形状，不存在"读完就退出"。codex 的优势在于它是**另一个模型家族**，视角真正独立。

我这轮的分工是：
- codex 三模型做独立视角的对抗扫描
- Workflow 做需要 schema、需要扇出、需要对抗性反驳环节的结构化审核

两者不是替代关系。但如果只能选一个，且你需要结果能被下一步程序化处理，选后者。

## 十、完整可用的调用模板

```bash
CODEX="/c/Users/Paul/AppData/Local/OpenAI/Codex/bin/69066b736e1e17a4/codex.exe"
S=/path/to/scratch

# 三个模型并行，各自独立取回最终消息
for m in gpt-5.5 gpt-5.6-sol gpt-5.6-luna; do
  nohup "$CODEX" exec \
    -c model=$m \
    -c model_reasoning_effort=max \
    --sandbox read-only \
    --skip-git-repo-check \
    -o "$S/review-$m.md" \
    - < "$S/prompt.md" > "$S/review-$m.log" 2>&1 &
  sleep 4   # 错开启动，避开 websocket 并发上限（否则会 426 回退 HTTP，浪费几秒）
done
```

跑完检查两件事：

```bash
for m in gpt-5.5 gpt-5.6-sol gpt-5.6-luna; do wc -c "$S/review-$m.md"; done
git status --short          # 有没有留下探针
```

## 附：八次失败的归因

| # | 症状 | 真实原因 |
|---|---|---|
| 1 | 10 分钟超时，且改了仓库文件 | `workspace-write` + 无沙箱约束 |
| 2 | exit 0，日志里无结论 | 抓的是 stdout，没用 `-o` |
| 3 | 同上，即使已收窄范围 | 同上 |
| 4–6 | 三模型交叉审核全部"无产出" | 同上 |
| 7 | `gpt-5.5` 只回显 prompt | 同上（它的过程输出本来就少） |
| 8 | `sol` 把预算花在派生子代理上 | 范围过宽 + 未限工具调用次数 |

**八次里有六次是同一个原因。** 早跑一次最小探针就能发现。
