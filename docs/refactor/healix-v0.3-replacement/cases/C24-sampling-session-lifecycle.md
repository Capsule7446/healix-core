# C24 — 采样会话生命周期

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：内存领域状态机和 CaptureID 幂等已实现。**

## 业务不变量

Created→Recording→Paused/Ended，active 状态可 Interrupted；terminal 不可继续。CaptureID 第一次结果固定，重试不得生成新动作/节点。

## 当前证据

- `domain/sampling/session.go`：Start/Record/Pause/Resume/End/Interrupt
- `domain/sampling/session_test.go`
- `domain/sampling/session_matrix_test.go`

## 调整清单

- [x] lifecycle transitions。
- [x] CaptureID required 与 first-result-wins。
- [x] pause/resume 保留 identity maps。

本次保持单会话内存状态机边界，不引入容量/保留策略、跨进程恢复或并发 Record 协议。

## 测试与验收

- [x] 单线程命令序列下同 CaptureID 重试只产生一个结果。
- [x] distinct captures 顺序/identity 正确。
- [x] pause/resume 后 retry 返回原结果。
- [x] terminal 后 capture 被拒绝。

## 依赖与风险

当前 Session 是单会话内存状态机；容量限制、并发调用与跨进程持久化均不属于本次重构。

## 审核

- [x] 批准保持单会话内存状态机
- [x] 修改：________________
