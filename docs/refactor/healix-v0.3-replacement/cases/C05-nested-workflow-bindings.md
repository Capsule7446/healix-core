# C05 — 嵌套 Workflow 参数绑定

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：字符串 binding 和运行期 scope restore 存在；typed lexical scope 缺失。**

## 业务不变量

每次 Workflow invocation 拥有独立、不可变作用域；child binding 只读取 parent scope，再按 child schema 转换、校验和补 default。

## 当前证据

- `domain/automation/assets.go`：`WorkflowReference.ParameterBindings`
- `application/engine/compiler.go`：`compileWorkflowCall`
- `domain/node/composite.go`：shared scratchpad overlay/restore
- `domain/interpolation/variables.go`：表达式解析

## 调整清单

- [x] 建立 invocation graph 与稳定 call-path/scope ID。
- [x] 为每个 invocation 创建 typed `ParameterScope`。
- [x] 定义 binding precedence、shadowing 和 conversion matrix。
- [x] sibling/repeat invocation 不共享 mutable parameter state。
- [x] 参数 context 与 extraction scratchpad 分离。
- [x] 移除任意值 `fmt.Sprint` 的隐式类型转换。
- [x] compiler 只消费已解析 scope，不补默认值。
- [x] 错误包含 call path + parameter name。

## 测试与验收

- [x] parent 可绑定 child 的全部参数类型。
- [x] binding > child default；缺 required 失败。
- [x] 同名 parent/child 不泄漏。
- [x] 同一 Workflow 两次调用可拥有不同快照。
- [x] repeat 每次 invocation scope 独立。

## 依赖与风险

依赖 C04；C05 先定义稳定 typed scope，C06 再冻结已解析 scope。需要谨慎拆分现有 Scratchpad 中的 extraction 数据。

## 审核

- [x] 批准
- [x] 修改：________________
