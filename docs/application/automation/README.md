# Automation Application Use Cases

本目录逐一记录已实现的 Automation 写用例；适配器能力不被虚构为核心实现。

## 通用约束

- 领域对象负责业务不变量；应用服务负责用例编排。
- 更新操作的预检不能替代仓储事务内 CAS。
- `delete-folder` 与 `approve-heal-candidate` 是多资源原子提交。

## 索引

- [创建环境](create-environment.md) — `create-environment`
- [更新环境](update-environment.md) — `update-environment`
- [删除环境](delete-environment.md) — `delete-environment`
- [恢复环境](restore-environment.md) — `restore-environment`
- [创建节点](create-node.md) — `create-node`
- [更新节点](update-node.md) — `update-node`
- [发布节点版本](publish-node-version.md) — `publish-node-version`
- [删除节点](delete-node.md) — `delete-node`
- [恢复节点](restore-node.md) — `restore-node`
- [创建工作流](create-workflow.md) — `create-workflow`
- [更新工作流](update-workflow.md) — `update-workflow`
- [发布工作流版本](publish-workflow-version.md) — `publish-workflow-version`
- [删除工作流](delete-workflow.md) — `delete-workflow`
- [恢复工作流](restore-workflow.md) — `restore-workflow`
- [创建测试任务](create-test-task.md) — `create-test-task`
- [保存已发布测试任务](save-published-test-task.md) — `save-published-test-task`
- [发布采样](publish-sampling.md) — `publish-sampling`
- [创建文件夹](create-folder.md) — `create-folder`
- [移动文件夹](move-folder.md) — `move-folder`
- [删除文件夹](delete-folder.md) — `delete-folder`
- [批准修复候选](approve-heal-candidate.md) — `approve-heal-candidate`
- [拒绝修复候选](reject-heal-candidate.md) — `reject-heal-candidate`

## 源码

- [应用层](../../../application/automation/)
- [领域层](../../../domain/automation/)
