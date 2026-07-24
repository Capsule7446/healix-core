# C24 — 采样会话生命周期

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

Created→Recording→Paused/Ended，活动状态可 Interrupted；终态不可继续。CaptureID 第一次结果固定，重试不得生成新动作/节点。

## 当前证据

- `domain/sampling/session.go`：Start/Record/Pause/Resume/End/Interrupt
- `domain/sampling/session_test.go`
- `domain/sampling/session_matrix_test.go`

## 调整清单

- [x] 生命周期转换。
- [x] `CaptureID` 为必填项并采用首次结果胜出语义。
- [x] 暂停/恢复保留身份映射。

本次保持单会话内存状态机边界，不引入容量/保留策略、跨进程恢复或并发 Record 协议。

## 测试与验收

- [x] 单线程命令序列下同 CaptureID 重试只产生一个结果。
- [x] 不同捕获的顺序与身份正确。
- [x] 暂停/恢复后重试返回原结果。
- [x] 终态后拒绝捕获。

## 依赖与风险

当前 Session 是单会话内存状态机；容量限制、并发调用与跨进程持久化均不属于本次重构。

## 审核

- [x] 批准保持单会话内存状态机
- [x] 修改：________________
