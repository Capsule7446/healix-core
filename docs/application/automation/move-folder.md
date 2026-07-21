# 移动文件夹

## 方法、输入与输出

- 方法：`FolderService.Move(context.Context, kind、id、parentID、expected、at)`
- 输出：`FolderSnapshot`；只有端口提交成功才返回成功结果。
- 领域转换：`domain.NewFolderForest`。目标必须存在；新父节点同类且不得形成环。

## 校验与领域状态转换

应用服务先完成显式参数和并发前置校验，再委托领域模型验证必填字段、生命周期、结构、版本连续性等不变量。`at`、审核身份等可信数据必须由入站适配器提供。领域失败时不调用写端口。

## 端口、事务与 CAS

使用 `FolderRepository.Load/Save`。应用层的 Revision 比对只提供快速失败；仓储适配器必须在存储事务中以 `expected` 执行条件写，竞争窗口中的失败也应映射为冲突。目标必须存在；新父节点同类且不得形成环。

## 错误

- 输入/领域错误：原样保留在错误链中，服务增加 create、transition 或 validate 上下文。
- 读取错误：包装为 `load ...`，不继续转换。
- Revision 不符：`RevisionConflictError`，支持 `errors.Is(err, ErrRevisionConflict)`。
- 写入、事务或 CAS 失败：包装为 persist、publish 或 commit 错误，不能返回部分成功。

## 已实现与适配器责任

已实现的是应用编排、领域调用、错误包装和端口契约。入站适配器负责鉴权、DTO、可信身份/时间及协议错误映射；出站适配器负责数据库事务、CAS、唯一约束、幂等、存储错误翻译和可观测性。核心未宣称存在 HTTP 或数据库实现。

## 时序

```mermaid
sequenceDiagram
    actor A as 入站适配器
    participant S as FolderService.Move
    participant R as 端口/仓储
    participant D as 领域模型
    A->>S: 输入与 expected
    S->>R: Load（如需要）
    alt 读取失败或 Revision 冲突
        R-->>S: error/当前 Revision
        S-->>A: load error 或 RevisionConflictError
    else 可转换
        S->>D: domain.NewFolderForest
        alt 领域校验失败
            D-->>S: domain error
            S-->>A: error（不写入）
        else 转换成功
            S->>R: 事务/CAS 提交
            alt 提交失败
                R-->>S: CAS/transaction error
                S-->>A: 包装错误
            else 提交成功
                R-->>S: persisted result
                S-->>A: FolderSnapshot
            end
        end
    end
```

## 失败流

```mermaid
flowchart TD
    I[接收命令] --> V{输入有效?}
    V -- 否 --> E1[返回校验错误]
    V -- 是 --> L[读取状态或构造聚合]
    L --> LR{读取成功?}
    LR -- 否 --> E2[返回 load 错误]
    LR -- 是 --> C{expected 匹配或无需 CAS?}
    C -- 否 --> E3[返回 RevisionConflictError]
    C -- 是 --> T[执行领域转换]
    T --> TV{领域有效?}
    TV -- 否 --> E4[返回领域错误]
    TV -- 是 --> P[适配器事务/CAS]
    P --> PS{提交成功?}
    PS -- 否 --> E5[返回 persist/commit 错误]
    PS -- 是 --> O[返回持久化结果]
```

## 相对源码链接

- [应用服务](../../../application/automation/folder_service.go)
- [端口](../../../application/automation/folder_repository.go)
- [领域模型](../../../domain/automation/)
- [测试](../../../application/automation/folder_service_test.go)
