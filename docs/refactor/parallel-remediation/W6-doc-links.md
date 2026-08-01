# W6 — 文档链接与测试台账

来源：`findings-consolidated.md` §三 R9 的后半（「术语清理完成」）。
核查结论：**Go 符号改完了，指着它们的文档没跟上。182 条链接是断的。**

本流不碰任何生产代码，因此可以从头到尾与其他五流并行；但**台账部分建议最后跑**，
因为 W1–W5 都会新增和改名测试。

## 独占文件

```
所有 TEST_CASES.md
docs/**  （三个例外见下）
architecture/doc_links_test.go     （新建）
```

**三个例外——这些不归本流：**

| 路径 | 归属 |
|---|---|
| `docs/refactor/business-error-contract/error-code-registry.md` | W1 |
| `docs/refactor/digest-wire-tags.md`（新建） | W5 |
| `docs/refactor/parallel-remediation/W1..W5.md` | 各流自己 |

**明确不碰：** 任何 `.go` 生产文件；`architecture/` 下除自己新建的那一个之外的测试。

---

## 1. 现状证据

全仓 markdown 里指向 `.go` 的相对链接共 1186 条，**其中 182 条目标文件不存在**：

| 文件 | 断链数 |
|---|---|
| `application/scheduling/TEST_CASES.md` | 91 |
| `domain/execution/TEST_CASES.md` | 51 |
| `docs/architecture/end-to-end-execution.md` | 10 |
| `docs/domains/execution.md` | 9 |
| `docs/application/scheduling/build-execution-plan.md` | 5 |
| `docs/application/scheduling/README.md` | 4 |
| `docs/application/scheduling/freeze-environment-properties.md` | 3 |
| `docs/architecture/system-overview.md` | 3 |
| `docs/integration/adapter-responsibilities.md` | 3 |
| `docs/integration/public-contract.md` | 3 |
| **合计** | **182** |

全部来自 `refactor/execution-instance` 那一串重命名提交
（`ade72b2` → `813a545`）。典型形态是台账行里的链接目标仍写
`application/scheduling/create_run_test.go`，而该文件早已改名为 `create_instance_test.go`。

> 本文件刻意**不**写出完整的 markdown 链接示范。步骤 1 的守卫是纯文本正则，
> 它分不清「一条死链」和「一个描述死链长什么样的例子」——写了就会让守卫在自己的
> runbook 上永远红一条。

台账里同时还有**已不存在的测试名**：`TestCreateRunRequestDigestMatrix`、
`TestBuildRunSnapshot*`、`TestHydrateRunSnapshot*`、`TestNewCreateRunServiceRejectsNilStore` 等。
链接坏了是可检测的，测试名坏了目前无法检测——见 §2 步骤 3 的裁决。

### 复现命令

```bash
perl -e '
use strict; use warnings; use File::Find; use File::Basename; use File::Spec;
my @md; find(sub{ push @md, $File::Find::name if /\.md$/ && $File::Find::dir !~ /\.git/ }, ".");
my (%broken, $total);
for my $f (@md) {
  open my $fh, "<", $f or next; my $dir = dirname($f);
  while (my $l = <$fh>) {
    while ($l =~ /\]\(([^)\s#]+\.go)(?:#[^)]*)?\)/g) {
      my $t = $1; next if $t =~ m{^https?://};
      $total++;
      my $p = File::Spec->canonpath("$dir/$t");
      push @{$broken{$f}}, $t unless -e $p;
    }
  }
}
print "total .go links: $total\n";
my $b=0; for (sort keys %broken){ printf "%6d  %s\n", scalar @{$broken{$_}}, $_; $b += scalar @{$broken{$_}}; }
print "TOTAL BROKEN: $b\n";
'
```

（Windows 上用仓库自带的 Git Bash 跑；PowerShell 里这段是解析错误。）

---

## 2. 修复步骤

**顺序反过来做：先建守卫，再修链接。** 否则修完 182 条只是把计数清零，
下一次重命名照样静默积累回来——而这次的教训恰恰是它已经静默积累了一整轮。

### 步骤 1 — 先建守卫，先看它红

新建 `architecture/doc_links_test.go`（`package architecture_test`，
复用 `repositoryRoot`，**新增的包级标识符一律加 `w6` 前缀**）：

```go
// TestEveryDocLinkToSourceResolves fails when a markdown link points at a Go
// file that does not exist. Renames land in the tree and the docs pointing at
// them do not: the v0.4 execution-instance rename left 182 dead links behind
// and nothing noticed, because a dead relative link in markdown is invisible
// to the compiler, to go vet, and to every test in this suite.
```

要求：

- 走遍全仓 `.md`（跳过 `.git`），匹配 markdown 内联链接中目标以 `.go` 结尾的那些，
  允许尾随 `#anchor`。
- 跳过 `http://` / `https://`。
- 相对于**该 markdown 文件所在目录**解析，不是仓库根。
- 报错信息给出：markdown 文件、行号、链接目标。182 条一次全报出来，
  不要 `t.Fatalf` 在第一条就停——修的人需要完整清单。

跑一次，把它报出 182 条的输出存下来当工作清单：

```bash
go test ./architecture/ -run TestEveryDocLinkToSourceResolves 2>&1 | tee /tmp/w6-broken-links.txt
```

### 步骤 2 — 按守卫的清单修链接

映射关系全部来自 `ade72b2`..`813a545` 那串重命名。逐条用 `git log --follow` 确认目标，
不要靠猜：

```bash
git log --oneline --follow --name-status -- application/scheduling/create_instance_test.go | head -20
```

已知的主要改名（动手前自己再核一遍，这只是起点）：

| 旧路径 | 新路径 |
|---|---|
| `application/scheduling/create_run_test.go` | `application/scheduling/create_instance_test.go` |
| `domain/execution/run_snapshot*.go` | `domain/execution/instance_snapshot*.go` |
| `domain/execution/run.go` / `run_test.go` | `domain/execution/instance.go` / `instance_test.go` |

修到守卫变绿。

### 步骤 3 — 台账里的测试名

链接修完之后，台账里仍有指向**已不存在的测试函数**的行
（`TestCreateRunRequestDigestMatrix` 等）。

**【裁决 1】** 台账怎么维护下去？

- **选项 A — 加一条测试名守卫（推荐）。**
  扩展 `doc_links_test.go`：链接带 `· TestXxx` 后缀时，
  用 `go/ast` 解析目标文件，断言该 `TestXxx` 函数存在。
  这把台账从「写完就烂」变成「烂了会红」，与步骤 1 是同一个思路。
  代价：其他五流新增测试时，台账不补就只是**不完整**（守卫不管这个），
  改名却不补就会红——正好是想要的取舍。
- **选项 B — 改成生成物。** 写一个 `scripts/gen-test-cases.sh` 从 AST 生成台账，
  CI 检查 `git diff --exit-code`。最彻底，但把台账里人工写的
  「输入/状态/边界」描述列冲掉了——那些是有信息量的，不该丢。
  若选 B，必须先想清楚描述列怎么保留。
- **选项 C — 只修这一次。** 不推荐：这条缺口的本质不是「有 182 条断链」，
  而是「断了一整轮没人发现」。

**【裁决 2】** 是否补齐 W1–W5 新增的测试台账行。
建议：**补**，但放在本流最后一次提交，等其余五流落定。
若某流尚未合入，在完成记录里写明台账停在哪个 commit。

### 步骤 4 — 更正 `findings-consolidated.md` §三

本流独占该文件。按本轮核查结论改两条表述：

- **R6**：「retry 无独立 occurrence」不成立。occurrence 存在且是真计数器
  （`domain/node/runtime.go:341-372`），`StepProgressEvent` / `StepPhaseEvent`
  都带它并校验 > 0。真实缺口是 occurrence 只到 event 为止，
  全部 observation 与 fact 都没有；且 `node.OperationObservation` 连 EntryID 都没有。
  详见 `W2-evidence-coordinates.md` §1。
- **R9**：「编码未动——这是决策，不是遗漏」不成立。`5ecfde2` 已经重新编码了
  cancel / abort / reorder 三个摘要并引入三个新 tag，存量幂等记录随之失效，
  且未记录。实际是「两个保留、三个已改」。详见 `W5-digest-wire-tags.md` §1.2。

另外把 §三的五条标注上各自的 runbook 链接，以及本轮核查新增的两条
（fence 错误无 code → W1；182 条断链 → 本流）。

### 步骤 5 — 承接其余五流的文档交接

各流 runbook 的「交接」段列了需要落到 `docs/` 的公共变更。最后一次扫一遍：

| 来自 | 需要更新 |
|---|---|
| W1 | `docs/integration/public-contract.md`、`docs/application/execution/README.md`——`NewEntryExecutor` 新增 `EntryAuthorizer` 参数；新 code `EXECUTION_WORKER_FENCE_INVALID` |
| W2 | `docs/domains/evidence.md`、`docs/application/execution/*.md`、`docs/architecture/end-to-end-execution.md`——证据身份三元组、`ExecutionOutcome` 常量改名 |
| W3 | `docs/application/scheduling/decide-next-entry.md`、`README.md`——Decision 现在产出 Pending→Running |
| W4 | `docs/domains/*.md`——`UnpublishedFlowFragment` 去掉三个字段 |
| W5 | 从 `docs/refactor/README` 类入口链到新建的 `digest-wire-tags.md` |

---

## 3. 验收

1. **守卫在修复前红，报满 182 条。**（步骤 1 的输出贴进 §4。）
2. **修复后守卫绿。**
3. **回归验证。** 临时把某个 `.md` 里一条好链接改坏，确认守卫报错，改回。
4. **§1 的 perl 复现命令输出 `TOTAL BROKEN: 0`**——两套独立实现互相印证，
   与 `conformance-audit.md` §2 的双实现交叉验证同款做法。
5.（若裁决 1 选 A）**测试名守卫在名字对不上时红。**

门禁：

```bash
go test ./architecture/...
test -z "$(gofmt -l .)" && go vet ./... && go build ./... && go test ./... && go test -race ./...
```

> 注意：本流不改 `.go` 生产代码，全量门禁失败时先确认失败点不在自己身上——
> 并行期间别人的半成品会让全量构建红（见 `README.md` §3.3）。

---

## 4. 交接与完成记录

本流是最后收口的一环，没有对外交接。

### 完成记录

> 由执行者填写：守卫红色输出的前后计数 / 裁决 1 与裁决 2 各选了什么、为什么 /
> 台账补到哪个 commit 为止 / §2 步骤 5 的五条交接各自落在哪个文件。
