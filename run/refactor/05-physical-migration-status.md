# 05 物理迁移状态

## 已完成

- `domain/workspace` 与 `domain/metrics` 已物理删除；Core 不再提供业务查询投影。
- 持久化资产、不可变版本、TestTask、Folder 和自愈候选治理归属 `domain/automation`。
- 封存多入口 Plan、顺序失败策略和运行生命周期归属 `domain/execution`。
- 不可变事实、非终态进度与原子终态提交契约归属 `domain/evidence`。
- 临时浏览器采样与编辑状态归属 `domain/sampling`。
- Application 使用局部小端口编排 Automation 命令、Plan 构造、Worker 调度、Evidence 写入和凭据解析。
- Environment 只保存 `CredentialReference`；`CredentialService` 通过携带 RunID 与 ClaimToken 的 WorkerScope 调用 `CredentialAuthorizer` 和 `SecretProvider`。
- Node Runtime 的非终态进度与终态提交已分离；终态及其暂存的验证/自愈事实只能通过 fenced 原子提交端口发布。
- Engine 直接编译 `execution.Plan`；旧 `RootVersionID`、`CompileExecution`、Workspace mapper 和大 Scheduler 接口已删除。

## 消费项目职责

生产适配器负责 Repository、事务、CAS、幂等、claim fencing、秘密存储、网络策略、实际运行配额，以及基于 Evidence 的业务读模型。Core 不提供兼容别名、旧入口或生产存储实现。
