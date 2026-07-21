# Engine Application

Engine 将 sealed execution plan 编译为内存 `node.Program`，并为每次运行建立全新的 `node.Runtime`。

- [编译计划](compile-plan.md)
- [运行程序](run-program.md)

## 当前边界与延期能力

以下能力当前**不受支持或明确延期**：lease heartbeat 与过期恢复、active cancellation registry、完整队列实现、参数优先级合并、生产级 adapters 与 read projections。调用方不得从现有接口推断这些能力已经存在。

## 源码与测试

- 源码：[`application/engine/doc.go`](../../../application/engine/doc.go)
- 测试：[`application/engine/engine_contract_matrix_test.go`](../../../application/engine/engine_contract_matrix_test.go)
