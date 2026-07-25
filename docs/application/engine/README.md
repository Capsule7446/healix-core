# 执行引擎应用层

执行引擎是单个已冻结顶层执行项的内存执行边界。它编译从 `execution.RunSnapshot` 中选定的顶层执行项，依据冻结的调用作用域和 `env.` 属性解析带类型的参数绑定，创建全新的 `node.Runtime`，并使用执行模块提供的浏览器运行生成的 `node.Program`。

- [编译执行计划](compile-plan.md)
- [运行执行程序](run-program.md)

## 所有权边界

- 执行引擎不创建执行实例、不解析 `latest` 版本、不读取可变的自动化资产，也不拥有队列或领取执行权状态；调度模块在创建执行实例时冻结这些决策。
- 执行引擎不获取或关闭浏览器。执行模块为每个测试任务顶层执行项提供一个浏览器；嵌套工作流调用在同一运行时和浏览器中执行。
- 环境是通过 `env.` 暴露的普通快照 `Properties`。Core 中不存在凭据授权器、密钥提供器或运行期间的环境回查。
- 绑定保留 `domain/parameter.Value` 类型。缺少必填值、绑定不兼容或参数未声明时，在节点操作运行前失败。
- 执行引擎返回执行结果；执行模块和执行证据模块拥有带栅栏校验的持久化及终态事实。

## 当前边界与延期能力

生产级浏览器适配器和持久化由宿主负责。对于提供的冻结快照、运行时端口和浏览器，执行引擎保持确定性。

## 源码与测试

- 源码：[`application/engine/compiler.go`](../../../application/engine/compiler.go)、[`application/engine/coordinator.go`](../../../application/engine/coordinator.go)
- 测试：[`application/engine/compiler_plan_test.go`](../../../application/engine/compiler_plan_test.go)、[`application/engine/binding_test.go`](../../../application/engine/binding_test.go)
