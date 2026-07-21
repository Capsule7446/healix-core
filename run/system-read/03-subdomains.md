# 03 子域

| 子域 | 职责 | 分类 |
|---|---|---|
| Workflow authoring/planning | Stores workflow, versions, test tasks, dependencies, run plans | Core business support |
| Browser execution | Runs compiled workflow nodes against a host Driver | Core |
| Selector fingerprinting | Represents stable target identity and normalized selectors | Core |
| Safe healing | Scores candidates, applies safety/review policy, preserves replay facts | Core/differentiating |
| Execution evidence | Captures phases, validation, operation and healing facts | Supporting but governance-critical |
| Workspace persistence | Reads/writes workflow and task aggregates through ports | Generic/supporting boundary |
| Framework detection | Classifies sanitized browser observations | Supporting capability |
| Recording/lifecycle | Starts/stops optional session capture | Supporting infrastructure port |
