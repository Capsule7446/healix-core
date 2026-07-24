# C04 — Typed 参数执行

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：authoring 类型存在；execution 显式拒绝，未闭环。**

## 业务不变量

WorkflowVersion 专门保存参数 schema：`Name` 是 `${Name}` 使用的稳定变量名，`DisplayName` 是人工建立 TestTask 时的字段标题，`Description` 是字段说明，`Type` 决定输入控件与值类型。TEXT、NUMBER、BOOLEAN、SINGLE_SELECT、MULTI_SELECT 的 definition/default/input 必须同类型；`Required=false` 必须声明一个合法默认值。`Required=true` 不允许依靠默认值兜底：根 Workflow 由 TestTask 人工提供，嵌套 Workflow 可由父级 binding 提供。所有规则必须在 TestTaskVersion 发布及 Run 创建前确定。

## 当前证据

- `domain/automation/assets.go`：`ParameterType`、`ParameterDefinition`
- `domain/automation/test_task_types.go`：`ParameterValues map[string]any`
- `domain/execution/plan.go`：string-only `Parameter`
- `application/scheduling/plan_mapper.go`：`rejectUnmappedParameters`、lossy mapping guard

## 调整清单

- [x] 保留 WorkflowVersion 的专用参数 schema，并正式固定字段语义：`Name`→`${}` 变量名、`DisplayName`→TestTask 表单 title、`Description`→表单 desc、`Type`→控件和值类型。
- [x] 为默认值增加显式 presence，区分“未提供默认值”和 TEXT 的合法空字符串默认值。
- [x] 设计封闭 typed value union，使 `DefaultValue` 与 TestTask 输入都按 Type 表达，并移除密封边界的任意 `any`。
- [x] 强制 `Required=false` 时存在合法 typed default；`Required=true` 不允许默认值兜底，根 Workflow 由 TestTask 人工输入，嵌套 Workflow 可由父级 binding 提供。
- [x] TestTask 编辑/发布根据所选精确 WorkflowVersion 的 parameter schema 生成字段并校验；只保存稳定 `Name`→typed value，不复制显示元数据。
- [x] 拒绝 unknown、duplicate 和缺失 required 参数，并验证输入值类型。
- [x] NUMBER 使用确定精度和 canonical representation。
- [x] BOOLEAN 定义精确接受形式。
- [x] SINGLE/MULTI 校验 options；默认值和 TestTask 输入都必须属于 options，并定义 MULTI 顺序与重复规则。
- [x] execution snapshot 保留 parameter definition identity、type 和最终 resolved value。
- [x] 稳定错误码包含 parameter path，不泄露值。

## 测试与验收

- [x] `Name` 可用于 `${Name}`，重命名显示名称或说明不改变变量绑定。
- [x] TestTask authoring schema 使用 `DisplayName` 作为 title、`Description` 作为 desc，并按 `Type` 提供对应控件语义。
- [x] 每种类型覆盖 explicit/default/missing required/invalid value。
- [x] `Required=false` 且没有默认值时 WorkflowVersion 发布失败；合法空字符串默认值可与“无默认值”区分。
- [x] `Required=true` 的根参数在 TestTask 未输入时发布失败；嵌套参数在父级 binding 未提供时解析失败，二者均不以默认值静默兜底。
- [x] unknown parameter 被拒绝。
- [x] SINGLE/MULTI 的默认值和输入值不属于 options 时被拒绝。
- [x] typed snapshot clone/round-trip 不丢语义。
- [x] item、binding、default 三种来源产生相同 canonical value。

## 依赖与风险

影响公共 API 和存储；需要固定 JSON number 的规范形式，并将调用方与存储直接改为 typed values，不保留旧字符串参数入口。

## 审核

- [x] 批准
- [x] 修改：________________
