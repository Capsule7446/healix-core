# Execution Application

该模块定义 worker fencing 下的凭据解析、非终态进度写入和终态事实提交边界。只有凭据解析拥有具体 service；进度与终态提交当前是 port 契约，由 adapter 实现。

- [解析凭据](resolve-credential.md)
- [记录进度](record-progress.md)
- [提交步骤迁移](commit-step-transition.md)

## 当前边界与延期能力

以下能力当前**不受支持或明确延期**：lease heartbeat 与过期恢复、active cancellation registry、完整队列实现、参数优先级合并、生产级 adapters 与 read projections。调用方不得从现有接口推断这些能力已经存在。

## 源码与测试

- 源码：[`application/execution/ports.go`](../../../application/execution/ports.go)
- 测试：[`application/execution/ports_test.go`](../../../application/execution/ports_test.go)
