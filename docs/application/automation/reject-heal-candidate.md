# 拒绝修复候选

与 [批准修复候选](approve-heal-candidate.md) 是同一个服务、同一套端口、同一个 `prepare`，因此构造约束、七个端口、幂等键、三重前置校验和治理专用错误码全部相同——那些内容只在批准文件里写一次。本文件只记录拒绝路径与批准路径的差异。

## 方法、输入与输出

- 方法：`HealReviewService.Reject(context.Context, domain.HealCandidateReviewCommand)`
- 输出：`error`（成功为 `nil`）。批准返回节点聚合，拒绝什么都不返回。
- 领域转换：`HealCandidate.Review(HealCandidateRejected)` + `HealStreak.Reject(sequence)`。提交的是候选状态与 streak 转移，**不发布节点版本**。

## 与批准路径的差异

- 命中回放时直接返回 `nil`，没有可复核的节点副本可返回。
- 共用 `prepare`，因此仍然会读取节点并比对节点 Revision 与基线版本——即使这条路径不写节点。`prepare` 返回的 intent 里 `NextNode` 是有值的，拒绝路径随后显式把它置为 `nil`。
- `prepare` 之后额外做三步：`HealReviewSource.LoadStreak`（包装为 `load heal streak`）、`HealReviewIdentityProvider.NextRejectionSequence`（包装为 `allocate heal rejection sequence`）、`HealStreak.Reject(sequence)`。
- intent 里因此多带 `ExpectedStreak`、`ExpectedStreakDigest`（`HealReviewStreakDigest`）与 `NextStreak`；请求摘要在这些字段填好之后才计算。
- 不调用 `HealReviewIdentityProvider.NewNodeVersionID`，所以没有 `allocate heal node version identity` 这一层包装。
- 结果形状检查方向相反：回放或提交结果里 `Result.ElementTarget` 必须为 nil、`Result.Streak` 必须非 nil、候选状态必须是 `HealCandidateRejected`，否则 `AUTOMATION_HEAL_REVIEW_CONTRACT_VIOLATION`。

## 时序

```mermaid
sequenceDiagram
    actor A as 入站适配器
    participant S as HealReviewService.Reject
    participant Auth as ReviewerAuthorizer
    participant T as HealReviewTransaction
    participant Src as HealReviewSource / NodeRepository
    participant D as 领域模型
    A->>S: HealCandidateReviewCommand
    S->>D: command.Validate(Rejected)
    S->>Auth: AuthorizeReviewer
    S->>T: LookupHealReview(CommandID, identityDigest)
    alt 命中回放
        T-->>S: HealReviewOutcome
        S->>S: validateHealReviewReplay
        S-->>A: nil
    else 无记录
        S->>Src: prepare（读候选、验证、读节点、三重比对）
        S->>Src: LoadStreak + NextRejectionSequence
        S->>D: candidate.Review(Rejected) + streak.Reject(sequence)
        S->>T: CommitHealReview(intent，NextNode 为 nil)
        alt 提交失败
            T-->>S: transaction error
            S-->>A: 包装为 commit heal review
        else 提交成功
            T-->>S: HealReviewOutcome
            S->>S: validateHealReviewOutcome
            S-->>A: nil
        end
    end
```

## 失败流

```mermaid
flowchart TD
    I[接收命令] --> V{command.Validate 通过?}
    V -- 否 --> E1[返回领域校验错误]
    V -- 是 --> AU{审核者已授权且非空?}
    AU -- 否 --> E2[包装为 authorize heal reviewer]
    AU -- 是 --> L[LookupHealReview]
    L --> LR{查询成功?}
    LR -- 否 --> E3[包装为 lookup heal review]
    LR -- 是 --> F{已有记录?}
    F -- 是 --> C1{回放与请求一致?}
    C1 -- 否 --> E4[AUTOMATION_HEAL_REVIEW_CONTRACT_VIOLATION]
    C1 -- 是 --> O[返回 nil]
    F -- 否 --> P[prepare 三重前置校验]
    P --> PE{通过?}
    PE -- 否 --> E5[authority / revision / stale base 冲突]
    PE -- 是 --> ST[LoadStreak]
    ST --> SE{读取成功?}
    SE -- 否 --> E6[包装为 load heal streak]
    SE -- 是 --> SQ[NextRejectionSequence]
    SQ --> QE{分配成功?}
    QE -- 否 --> E7[包装为 allocate heal rejection sequence]
    QE -- 是 --> T[Review + streak.Reject]
    T --> TV{领域有效?}
    TV -- 否 --> E8[返回领域错误]
    TV -- 是 --> CM[CommitHealReview]
    CM --> CS{提交成功?}
    CS -- 否 --> E9[包装为 commit heal review]
    CS -- 是 --> C2{outcome 与 intent 一致且节点为 nil?}
    C2 -- 否 --> E4
    C2 -- 是 --> O
```

## 源码

- [应用服务](../../../application/automation/heal_review_service.go)
- [端口与摘要](../../../application/automation/heal_candidate_repository.go)
- [领域模型](../../../domain/automation/healing.go)
- [测试](../../../application/automation/heal_review_service_test.go)、[用例矩阵](../../../application/automation/publication_review_usecase_matrix_test.go)
