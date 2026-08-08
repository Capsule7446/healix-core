// Package engine 从不可变的 execution.InstanceSnapshot 编译单个入口，解析其类型化参数绑定（包括
// 冻结的 env. 属性），并使用 Execution 提供的运行时端口执行生成的 node.Program。调度上下文负责
// 创建 Run 和冻结最新版本；Execution 负责浏览器生命周期、工作线程隔离和 Evidence 持久化。
package engine
