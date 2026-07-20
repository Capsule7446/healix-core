# 浏览器执行模型优化设计

## 1. 背景与目标

当前浏览器核心已经支持单页面导航、元素定位、交互、等待、验证和确定性自愈，但执行模型中仍存在重复的定位、读取、轮询、重试和观测逻辑。不同节点各自处理这些机制，导致错误语义不一致、操作事实不完整，自愈安全边界也难以集中保证。

本次重构采用以下原则：

- **外部语义分离，内部机制统一**；
- **直接重构现有模型，不保留旧模型兼容层**；
- 继续只支持单页面、单并发；
- 浏览器库仍由宿主适配器实现，领域层只定义稳定端口；
- 自愈安全检查必须先于 selector overlay；
- 敏感输入不得进入操作观测；
- 取消和超时的细粒度语义本轮只保留当前 `context.Context` 能力，后续单独设计。

## 2. 目标外部语义

执行树保留四类清晰语义：

```text
Node
├── Action
│   ├── ElementAction
│   │   ├── click
│   │   ├── input
│   │   ├── select
│   │   ├── hover
│   │   ├── extract
│   │   └── noop
│   └── PageAction
│       ├── navigate
│       └── press
├── WaitControl
├── Assertion
└── Composite
```

- `Action`：修改或读取页面状态；
- `WaitControl`：等待页面状态满足条件；
- `Assertion`：表达业务质量断言并产出验证事实；
- `Composite`：组织顺序、重复和工作流调用结构。

本次不引入多页面、Frame 或浏览器上下文对象，避免把页面生命周期和执行节点混在一起。

## 3. 统一内部机制

### 3.1 Locator

`Locator` 统一当前单页面元素定位：

- 使用 `NodeSpec` 和 selector 优先级定位；
- 读取和写入本次运行的 selector overlay；
- 定位失败时调用确定性 healer；
- 在安全评估通过后才应用 healed selector；
- 返回短生命周期的 `Element` 句柄。

### 3.2 Reader

`Reader` 统一元素读取：

- 文本；
- 属性；
- 存在性；
- 可见性；
- 稳定性和控件状态。

Reader 不执行动作，也不写入业务事实；读取错误交由统一错误分类器处理。

### 3.3 Poller

`Poller` 统一条件轮询：

- timeout；
- poll interval；
- stability interval；
- 最后一次观察值；
- context 结束；
- transient driver error 的有限重试。

`WaitControl` 和 `Assertion` 都通过 Poller 复用轮询机制，而不是各自创建 ticker 或 timeout 循环。

### 3.4 OperationRunner

`OperationRunner` 统一操作执行：

- 调用操作；
- 只对显式 transient driver error 执行有限重试；
- 记录每次或最终尝试次数；
- 计时；
- 归类错误；
- 发出操作前后观测。

安全拒绝、断言失败、元素未找到、超时和 context 结束都不能通过自动重试绕过。

### 3.5 ErrorClassifier

统一错误类别：

- `not_found`；
- `not_visible`；
- `not_interactable`；
- `timeout`；
- `navigation`；
- `assertion`；
- `context_closed`；
- `transient_driver`；
- `unknown`。

分类结果必须保留原始错误链，并保证零值错误不会在日志或包装时 panic。

### 3.6 OperationObserver

统一记录：

- run ID；
- node ID；
- operation；
- 原始或有效 selector；
- 是否使用自愈；
- 尝试次数；
- duration；
- success；
- error kind。

不得记录完整输入值、密码、原始 HTML 或其他敏感内容。

## 4. 自愈模型

候选从单一总分扩展为结构化证据：

```go
type CandidateEvidence struct {
    Dimension string
    Score     float64
    Matched   bool
}
```

候选决策流程固定为：

```text
候选评分
  ↓
结构化证据生成
  ↓
安全边界检查
  ↓
通过才允许应用
  ↓
写入 SelectorOverlay
```

决策必须区分：

- `APPLIED`：达到应用阈值且安全边界通过；
- `BELOW_REVIEW_CAP`：达到复核阈值但需要人工复核；
- `NO_CANDIDATE`：没有候选达到最低阈值；
- `SAFETY_REJECTED`：候选与页面、语义、表单或其他安全边界冲突。

安全拒绝不得通过重试、降低阈值或直接写入 selector overlay 绕过。

## 5. 直接重构范围

本次不保留旧字符串映射、旧枚举包装或兼容适配层，直接调整：

- `domain/node` 的 `StepNode`、Action 模型、WaitControl、ValidationNode；
- `domain/heal` 的 Decision、Candidate、评分证据和安全策略；
- `domain/fingerprint` 需要的上下文和语义值对象；
- `application/engine` 编译映射；
- workspace 持久化模型及其验证；
- contract、unit、integration 和 race tests。

## 6. 明确延期能力

以下能力本轮不实现，保留在 [backlog](backlog.md)：

- 多页面、Tab、Popup；
- Frame；
- 文件上传和下载；
- Cookie、LocalStorage、SessionStorage；
- Browser Context；
- 拖拽、触摸和坐标鼠标；
- 任意 JavaScript；
- 网络拦截和请求 Mock；
- 浏览器进程管理；
- 并发执行和资源池；
- 跨页面事件；
- 取消和超时的细粒度语义。

## 7. 实施顺序

1. 实现 `Locator`、`Reader`、`Poller`；
2. 实现 `ErrorClassifier` 和 `OperationRunner`；
3. 重构 `StepNode`；
4. 重构 `WaitControl`；
5. 重构 `ValidationNode`；
6. 接入操作前后观测；
7. 增加候选结构化证据；
8. 增加自愈安全策略；
9. 接入 `SelectorOverlay` 安全门；
10. 调整 workspace、编译器和测试；
11. 删除重复逻辑；
12. 执行完整 Go 验证和代码审查。

## 8. 验证标准

```text
go test ./...
go test -race ./...
go test -cover ./...
go vet ./...
go build ./...
git diff --check
go test ./domain/heal -run '^$' -bench . -benchmem
```

完成标准：所有测试通过、覆盖率保持主要包 80% 以上、无 race/vet/build 错误、单页面和单并发边界未被扩张，并且安全拒绝无法进入 selector overlay。
