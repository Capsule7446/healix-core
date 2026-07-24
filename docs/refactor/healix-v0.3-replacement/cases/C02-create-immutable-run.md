# C02 — 创建不可变 Run

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：部分实现。`execution.Plan` 可密封，但 Run 没有完整快照闭环。**

## 业务不变量

Run 创建时一次冻结 TestTaskVersion、Workflow/Node/Reference 图、参数、Environment 和策略；排队、重试和后续资产变更不得改变执行内容。

## 当前证据

- `domain/execution/run.go`：Run 生命周期
- `domain/execution/plan.go`：`Draft`、`Seal`、`Plan` 深拷贝
- `application/scheduling/plan_mapper.go`：`BuildExecutionPlan`
- `application/scheduling/ports.go`：Run 创建端口

## 调整清单

- [x] 新增 durable `RunSnapshot`，绑定 RunID/TestTaskVersionID/schema version。
- [x] Run 记录 snapshot identity/digest。
- [x] 快照纳入参数、环境、ScreenshotPolicy、HealerPolicy。
- [x] CreateRun 服务原子持久化 Run、entries、snapshot、queue membership。
- [x] 定义 command ID、幂等重放与同 ID 异内容冲突。
- [x] Worker 只能从 RunSnapshot 编译，禁止回读 current assets。
- [x] 新契约直接替换旧 Run 创建路径；无有效 RunSnapshot 的既有 queued/running Run 不迁入新执行链路。

## 测试与验收

- [x] 创建后发布新资产或修改环境，旧 Run 编译结果不变。
- [x] 无有效 snapshot 的 Run 不能进入 runnable。
- [x] 持久化任一点失败不留下半成品 Run。
- [x] 重试/重新 claim 得到相同 compiled plan。

## 依赖与风险

是 C03–C07 的根依赖；主要风险是快照体积和 schema 演进。旧执行路径与不兼容数据不做兼容层。

## 审核

- [x] 批准
- [x] 修改：________________
