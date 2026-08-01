# 调度应用层

调度模块拥有执行实例命令、不可变的执行实例创建、队列/领取执行权决策以及执行实例生命周期转换。`CreateRunService` 只解析一次请求的测试任务版本、所有固定版本/`latest` 工作流依赖、带类型的顶层执行项参数和环境，封存 `execution.RunSnapshot`，并原子插入执行实例、快照和顶层执行项。

## 用例

- [创建执行实例与冻结执行计划](build-execution-plan.md)
- [冻结并注入环境属性](freeze-environment-properties.md)
- [决定下一顶层执行项](decide-next-entry.md)
- [处理下一次领取执行权](process-next-claim.md)

## 所有权边界

- 创建执行实例是唯一的 `latest` 冻结点；运行和重试必须使用持久化快照，不能再次查询 `current`/`latest`。
- 环境以普通 `Properties map[string]string` 复制进快照，并在执行引擎绑定时成为只读 `env.` 参数；不存在凭据或密钥解析路径。
- 顶层执行项参数和嵌套调用参数使用 `domain/parameter` 的带类型值与绑定。创建时完成必填项、类型、选项和绑定校验，不能静默转换为字符串。
- 调度模块拥有队列、领取执行权、栅栏令牌与执行实例状态；`DecideAdvance` 在选出下一个 entry 时同时产出 Pending→Running 转移，停止级联时产出 Pending→Skipped；不打开浏览器、不运行节点、不写执行证据。
- `CreateRunStore` 的事务必须原子保存命令幂等记录、执行实例、快照与顶层执行项身份；同一命令 ID 的不同请求摘要属于冲突。

## 当前边界与延期能力

生产级适配器、读取投影、完整队列实现以及租约心跳和过期恢复由宿主提供；Core 定义事务和栅栏校验契约及其一致性测试套件。

## 源码与测试

- 源码：[`application/scheduling/create_instance_service.go`](../../../application/scheduling/create_instance_service.go)、[`application/scheduling/create_instance_builder.go`](../../../application/scheduling/create_instance_builder.go)
- 测试：[`application/scheduling/create_instance_test.go`](../../../application/scheduling/create_instance_test.go)、[`application/scheduling/create_instance_transaction_test.go`](../../../application/scheduling/create_instance_transaction_test.go)
