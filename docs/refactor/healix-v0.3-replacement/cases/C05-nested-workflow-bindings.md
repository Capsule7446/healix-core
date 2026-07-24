# C05 — 嵌套工作流参数绑定

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

每次工作流 invocation 拥有独立、不可变作用域；child binding 只读取 parent scope，再按 child schema 转换、校验和补 default。

## 当前证据

- `domain/automation/assets.go`：`WorkflowReference.ParameterBindings`
- `application/engine/compiler.go`：`compileWorkflowCall`
- `application/engine/compiler.go`：递归编译嵌套调用并生成 按出现位置区分的作用域
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
- [x] 同一工作流两次调用可拥有不同快照。
- [x] repeat 每次 invocation scope 独立。

## 依赖与风险

依赖 C04；C05 先定义稳定 typed scope，C06 再冻结已解析 scope。需要谨慎拆分现有 Scratchpad 中的 extraction 数据。

## 审核

- [x] 批准
- [x] 修改：________________
