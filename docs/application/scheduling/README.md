# 调度应用层

调度模块拥有执行实例命令、不可变的执行实例创建、队列/领取执行权决策以及执行实例生命周期转换。`CreateInstanceService` 只解析一次请求的测试任务版本、所有固定版本/`latest` 工作流依赖、带类型的顶层执行项参数和环境，封存 `execution.InstanceSnapshot`，并原子插入执行实例、快照和顶层执行项。

## 用例

- [创建执行实例与冻结执行计划](build-execution-plan.md)
- [冻结并注入环境属性](freeze-environment-properties.md)
- [决定下一顶层执行项](decide-next-entry.md)
- [处理下一次领取执行权](process-next-claim.md)

## 所有权边界

- 创建执行实例是唯一的 `latest` 冻结点；运行和重试必须使用持久化快照，不能再次查询 `current`/`latest`。
- 环境以带类型的 `Variables map[string]parameter.Value` 复制进快照，并在执行引擎绑定时成为只读 `env.` 参数；不存在凭据或密钥解析路径。
- 顶层执行项参数和嵌套调用参数使用 `domain/parameter` 的带类型值与绑定。创建时完成必填项、类型、选项和绑定校验，不能静默转换为字符串。
- 调度模块拥有队列、领取执行权、栅栏令牌与执行实例状态；`DecideAdvance` 在选出下一个 entry 时同时产出 Pending→Running 转移，停止级联时产出 Pending→Skipped，且两类转移都先过 `execution.ValidateEntryStatusTransition`；不打开浏览器、不运行节点、不写执行证据。
- `CreateInstanceStore` 的事务必须原子保存命令幂等记录、执行实例、快照与顶层执行项身份；同一命令 ID 的不同请求摘要属于冲突。
- 本包所有遍历 map 后"返回第一处失败"的校验都统一经 `sortedKeys` 排序（[`ordered_keys.go`](../../../application/scheduling/ordered_keys.go)）。这不是排版偏好：这些循环里既有返回未分类错误的分支，也有返回参数自带码的分支，Go 的随机 map 迭代会让同一份输入在不同运行里落到不同分支，从而改变 `fault.IsCode` 沿错误链能查到的码。

## 已知的错误码歧义

`AbortInstanceService` 的命令校验（[`instance_command_services.go`](../../../application/scheduling/instance_command_services.go)）在同一个顶层码 `EXECUTION_ABORT_INSTANCE_COMMAND_INVALID` 下返回两条不同的错误链：CommandID / InstanceID / ExpectedRevision / At / `Fence.InstanceID` 任一不合法时是 `abortInstanceCommandInvalidError(nil)`，链里只有这一个码；而 `Fence.Validate()` 失败时是 `abortInstanceCommandInvalidError(err)`，链里还带着 `EXECUTION_WORKER_FENCE_INVALID`。`fault.CodeOf` 两种情况一致，但 `fault.IsCode(err, execution.CodeWorkerFenceInvalid)` 会给出不同答案。宿主应按顶层码分支，不要用 `IsCode` 去区分"是不是栅栏的问题"。

## 当前边界与延期能力

以下能力当前**不受支持**，各用例文件不重复该清单：租约心跳与过期恢复、活动取消注册表、完整队列实现、参数优先级合并、生产级适配器与读取投影。调用方不得从现有接口推断这些能力已经存在。Core 只定义事务和栅栏校验契约及其一致性测试套件。

## 源码与测试

- 源码：[`application/scheduling/create_instance_service.go`](../../../application/scheduling/create_instance_service.go)、[`application/scheduling/create_instance_builder.go`](../../../application/scheduling/create_instance_builder.go)
- 测试：[`application/scheduling/create_instance_test.go`](../../../application/scheduling/create_instance_test.go)、[`application/scheduling/create_instance_transaction_test.go`](../../../application/scheduling/create_instance_transaction_test.go)
