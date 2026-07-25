# C02 — 创建不可变执行实例

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

执行实例创建时一次冻结 TestTaskVersion、Workflow/Node/Reference 图、参数、环境和策略；排队、重试和后续资产变更不得改变执行内容。

## 当前证据

- `domain/execution/run.go`：执行实例生命周期
- `domain/execution/plan.go`：`Draft`、`Seal`、`Plan` 深拷贝
- `application/scheduling/plan_mapper.go`：`buildExecutionDraft` 将已解析发布数据和执行实例 entries 映射为 `execution.Draft`
- `application/scheduling/ports.go`：执行实例创建端口

## 调整清单

- [x] 新增 durable `RunSnapshot`，绑定 RunID/TestTaskVersionID/schema version。
- [x] 执行实例记录快照 identity/digest。
- [x] 快照纳入参数、环境、ScreenshotPolicy、HealerPolicy。
- [x] CreateRun 服务原子持久化执行实例、entries、快照、queue membership。
- [x] 定义命令 ID、幂等重放与同 ID 异内容冲突。
- [x] 工作器只能从 RunSnapshot 编译，禁止回读 current assets。
- [x] 新契约直接替换旧执行实例创建路径；无有效 RunSnapshot 的既有 queued/running 执行实例不迁入新执行链路。

## 测试与验收

- [x] 创建后发布新资产或修改环境，旧执行实例编译结果不变。
- [x] 无有效快照的 执行实例不能进入 runnable。
- [x] 持久化任一点失败不留下半成品执行实例。
- [x] 重试/重新领取执行权得到相同 compiled plan。

## 依赖与风险

是 C03–C07 的根依赖；主要风险是快照体积和 schema 演进。旧执行路径与不兼容数据不做兼容层。

## 审核

- [x] 批准
- [x] 修改：________________
