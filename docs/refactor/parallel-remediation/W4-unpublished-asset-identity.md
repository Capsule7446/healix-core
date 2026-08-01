# W4 — 未发布资产的正式身份

来源：`findings-consolidated.md` §三 R8。核查完全确认，但**原文暗示的修法只能抓 3 个里的 2 个**。

本流改动量最小，风险也最低，但它有一个前置。

## 独占文件

```
domain/sampling/*.go                             （含测试）
architecture/unified_language_boundary_test.go
```

**不含 `domain/sampling/TEST_CASES.md`** —— 所有 `TEST_CASES.md` 归 W6。

**前置：** `README.md` §0 必须先完成。`unified_language_boundary_test.go` 现有未提交改动
（fingerprint Clone 守卫加强）。在它被提交之前动手，两边改动会互相覆盖。

**明确不碰：** `application/automation/sampling_publication_mapper.go`（无人认领，只读引用）、
`domain/automation/sampling_publication.go`。

---

## 1. 现状证据

```
domain/sampling/workspace.go:97   SavedWorkflowID    string
domain/sampling/workspace.go:98   SavedVersionID     string
domain/sampling/workspace.go:99   SavedVersionNumber int
```

三个字段在全仓**各出现恰好一次**——就是上面这三行声明。零读、零写、零测试、零文档引用。
它们是 `UnpublishedFlowFragment` 上的正式身份，而正式身份只有 Automation 在发布成功之后
才有权赋予；同样的三元组已经由 `domain/automation.SamplingPublicationResult`
（`sampling_publication.go:30-35`：`FlowFragmentID` / `WorkflowVersionID` / `VersionNumber`）表达。

守卫抓不到的原因在 `architecture/unified_language_boundary_test.go:76-82`：

```go
for _, banned := range forbidden {
    if field == banned {        // ← 精确相等
```

`forbidden` 是 `[]string{"Version", "VersionNumber", "Revision", "CurrentVersionID", "ElementTargetVersionID"}`（L70）。
`"SavedVersionNumber" != "VersionNumber"`，通过。

**关键更正：** 把 `==` 换成 `strings.Contains` 只解决三分之二。
`SavedVersionID` 与 `SavedVersionNumber` 都含 `"Version"`，会被抓到；
**`SavedWorkflowID` 不含 forbidden 列表里的任何一项**，照样通过。

同一个文件里的兄弟守卫 `TestPublishedVersionsCarryNoTemporaryIdentity`（L100）
用的已经是 `strings.Contains`——两条守卫的匹配口径本来就不一致。

---

## 2. 修复步骤

### 步骤 1 — 先让守卫红

**顺序很重要：先改守卫，跑，看它抓到三个字段，再删字段。**
反过来做就证明不了守卫真的管用——这正是 `findings-consolidated.md` §五「假绿的测试比
没有测试更糟」记的那一类。

在 `unified_language_boundary_test.go` 的
`TestUnpublishedSamplingAssetsCarryNoFormalIdentity`（L64-84）加一条**独立于
forbidden 列表**的前缀规则：

```go
// A formal identity does not stop being one because the field name says the
// asset merely "saved" it. Substring matching against the forbidden list is
// not enough on its own: SavedWorkflowID contains none of those words, yet it
// is the same category of leak.
formalPrefixes := []string{"Saved", "Published", "Promoted", "Formal"}
```

对每个字段：命中 `forbidden`（改为大小写敏感的**子串**比对，与 L100 的兄弟守卫一致）
**或**以 `formalPrefixes` 之一开头 → 报错。

两条规则并存，各自的错误信息要能区分是哪一条命中的，否则将来红了不知道该怎么修。

跑，确认它现在报三个字段：

```bash
go test ./architecture/ -run TestUnpublishedSamplingAssetsCarryNoFormalIdentity -v
```

### 步骤 2 — 删字段

删掉 `domain/sampling/workspace.go:97-99` 三行。

编译面确认（应该为空）：

```bash
grep -rn "SavedWorkflowID\|SavedVersionID\|SavedVersionNumber" --include=*.go .
go build ./... && go test ./domain/sampling/... ./application/automation/...
```

`application/automation/sampling_publication_mapper.go:63` 把
`sampling.UnpublishedFlowFragment` 作为 `SamplingPublicationRequest.Workspace` 的字段类型使用——
删掉结构体里没人读写的字段不影响它，但跑一次 `./application/automation/...` 确认。

### 步骤 3 — 记下为什么不是搬家而是删除

在删除处附近或提交信息里写清：这不是把字段挪到别处，是删除。
发布结果的三元组已经由 `domain.SamplingPublicationResult` 表达，
未发布草稿上再存一份，既是重复也是一条能在发布前被读到的正式身份。

**【裁决】** `formalPrefixes` 里要不要包含 `Existing`。
L68-69 的注释明确说 `ExistingElementTargetID` 是「指向已发布物的引用，不是这个资产自己的身份」，
所以 `Existing` **不应**进列表。但 `Saved` 与 `Existing` 的界线是语义而非语法，
新加字段时容易踩错。写一句注释说明这条界线在哪，比列表本身更重要。

---

## 3. 验收

1. **守卫在删除前就红，且报满三个字段。**（步骤 1 已跑过，把输出贴进 §4。）
2. **删除后守卫绿。**
3. **`TestPublishedVersionsCarryNoTemporaryIdentity` 保持绿**——反向那条不受影响。
4. **回归守卫。** 在测试里临时给 `UnpublishedFlowFragment` 加一个
   `SavedFooID string`，确认守卫报错，然后删掉。这一步不留在代码里，
   但要在 §4 记下做过——它是「守卫真的能抓将来的新增」的唯一证据。

门禁：

```bash
go test ./architecture/... ./domain/sampling/... ./application/automation/...
test -z "$(gofmt -l .)" && go vet ./... && go build ./... && go test ./... && go test -race ./...
```

---

## 4. 交接与完成记录

**给 W6：** `docs/domains/*.md` 里若描述过 `UnpublishedFlowFragment` 的字段，需要同步。
本流不碰 `docs/`。

**注意：** 本流是唯一改既有 `architecture/*_test.go` 的流。W2/W3/W5/W6 都在这个包里
新建文件，**不要顺手动 `unified_language_boundary_test.go` 之外的任何既有 architecture 测试**，
也留意别和它们的新 helper 重名（本流不新增包级 helper 即可完全避开）。

### 完成记录

> 由执行者填写：步骤 1 的红色输出 / 回归守卫验证过程 / `Existing` 那条裁决怎么写的。
