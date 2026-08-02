# domain/fault Test Case Matrix

## 范围与口径

本表记录 `domain/fault` 的公开业务入口和全部顶层 Go testcase。Go 测试源码是唯一可执行事实；表驱动测试的全部子案例由其对应的测试函数统一引用。

`domain/fault` 是错误内核：每个有界上下文的公开失败都经由它构造，因此它自身的契约不能只靠调用方间接覆盖。本表的边界描述逐条写明真实输入与预期，不使用「表驱动子案例（如存在）」这类占位文字——错误内核是唯一无法用别处的矩阵兜底的包。

内核守护三条不变量，下表每一行都归属其中之一：

1. **公开文本安全**：`Error()`、`Message()`、`Format` 的任何动词都不得泄露 private cause、调用方输入或非可打印 ASCII。
2. **分类稳定**：`Code`/`Kind` 是宿主分支依据，必须能穿透 `fmt.Errorf` 包装与 `errors.Join`，且不得被二次包装改写。
3. **所有权独立**：构造与读取都深拷贝 `params`/`violations`，调用方持有的切片与内核持有的互不别名。

## Public API / Use-case Inventory

| 公开入口 | 定义文件 | 测试证据状态 |
|---|---|---|
| `Kind`（11 个封闭常量） | [`domain/fault/fault.go`](../../domain/fault/fault.go) | 由 `TestNewRejectsInvalidPublicValues` 覆盖封闭集之外的取值被拒。 |
| `Code` / `Code.Error` | [`domain/fault/fault.go`](../../domain/fault/fault.go) | `TestCodeSatisfiesErrorWithItsOwnString` 直接覆盖；`Code` 实现 `error` 是 `errors.Is(err, someCode)` 得以成立的前提。 |
| `NewParam` / `Param.Key` / `Param.Value` | [`domain/fault/fault.go`](../../domain/fault/fault.go) | `TestParamAccessorsReturnWhatWasConstructed`、`TestNewParamRejectsOversizeValue` 直接覆盖。 |
| `NewViolation` / `Violation.Code` / `Field` / `Message` / `Params` | [`domain/fault/fault.go`](../../domain/fault/fault.go) | `TestNewViolationRejectsUnusableParams`、`TestFaultRejectsInvalidOptionsAndViolationShapes` 直接覆盖。 |
| `WithParams` / `WithViolations` | [`domain/fault/fault.go`](../../domain/fault/fault.go) | `TestPopulatedFaultReadsBackThroughItsOwnAccessors`、`TestFaultDefensivelyCopiesParamsAndViolations` 直接覆盖。 |
| `New` / `Wrap` | [`domain/fault/fault.go`](../../domain/fault/fault.go) | 全表主入口；构造期校验、截断、option 失败均有独立行。 |
| `Error.Error` / `Unwrap` / `Format` / `Is` | [`domain/fault/fault.go`](../../domain/fault/fault.go) | 不变量 1 与 2 的核心，见泄露与遍历各行。 |
| `Error.Kind` / `Code` / `Message` / `Params` / `Violations` | [`domain/fault/fault.go`](../../domain/fault/fault.go) | 非 nil 与 nil 两条路径分别由 `TestPopulatedFaultReadsBackThroughItsOwnAccessors` 和 `TestNilFaultCollectionAccessorsReturnNil` / `TestFaultHandlesNilAndUnknownErrors` 覆盖。 |
| `CodeOf` / `KindOf` / `IsCode` / `Describe` | [`domain/fault/fault.go`](../../domain/fault/fault.go) | `TestFaultQueriesTraverseWrappingAndJoin`、`TestIsCodeRejectsAnUnusableCode` 直接覆盖。 |
| `Descriptor.Kind` / `Code` / `Message` / `Params` / `Violations` | [`domain/fault/fault.go`](../../domain/fault/fault.go) | `Describe` 的只读投影，随 `TestFaultDefensivelyCopiesParamsAndViolations` 一并验证所有权。 |
| `MaxViolations` | [`domain/fault/fault.go`](../../domain/fault/fault.go) | `TestOverCapViolationsTruncateInsteadOfFailingConstruction` 覆盖截断而非失败的语义。 |
| `CodeFieldRequired` / `CodeFieldInvalid` / `CodeFieldDuplicate` / `CodeFieldMismatch` | [`domain/fault/violation_codes.go`](../../domain/fault/violation_codes.go) | 封闭 reason 词表；「不得作为顶层 Error 的 code」由 [`architecture/fault_contract_guard_test.go`](../../architecture/fault_contract_guard_test.go) 强制。 |

## Test Case Evidence Matrix

| Test case | 输入、边界或业务前置状态 | 预期契约 | 可执行证据 |
|---|---|---|---|
| `TestNewRejectsInvalidPublicValues` | 封闭集之外的 `Kind`；不匹配 `^[A-Z][A-Z0-9_]{2,62}$` 的 `Code`；空白、超 512 字节、含控制字符的 message。 | 构造失败并返回普通 Go error；无部分构造的 `*Error` 逃逸。 | [`domain/fault/fault_test.go`](../../domain/fault/fault_test.go) · `TestNewRejectsInvalidPublicValues` |
| `TestFaultPreservesCauseWithoutDisclosingIt` | `Wrap` 一个文本含 token/URL 的 cause。 | `Error()` 与 `Message()` 只输出 `CODE: message`；cause 仅经 `Unwrap` 可达。 | [`domain/fault/fault_test.go`](../../domain/fault/fault_test.go) · `TestFaultPreservesCauseWithoutDisclosingIt` |
| `TestFaultQueriesTraverseWrappingAndJoin` | fault 被 `fmt.Errorf("%w")` 包装、被 `errors.Join` 与其他 error 合并、多层嵌套。 | `CodeOf`/`KindOf`/`IsCode`/`Describe` 均能穿透定位；宿主分支不依赖链的形状。 | [`domain/fault/fault_test.go`](../../domain/fault/fault_test.go) · `TestFaultQueriesTraverseWrappingAndJoin` |
| `TestFaultDefensivelyCopiesParamsAndViolations` | 构造后修改调用方持有的 params/violations 切片；再修改 `Params()`/`Violations()`/`Describe()` 返回的切片。 | 两个方向都不影响内核持有的副本；不变量 3。 | [`domain/fault/fault_test.go`](../../domain/fault/fault_test.go) · `TestFaultDefensivelyCopiesParamsAndViolations` |
| `TestFaultHandlesNilAndUnknownErrors` | nil `*Error` 接收者；typed-nil 装进 `error`；完全无关的 `errors.New`。 | `Error()` 返回 `<nil>`，`Unwrap`/`Code`/`Kind`/`Message` 返回零值；`CodeOf`/`KindOf`/`IsCode` 一律报告未找到。 | [`domain/fault/fault_test.go`](../../domain/fault/fault_test.go) · `TestFaultHandlesNilAndUnknownErrors` |
| `TestFaultRejectsInvalidOptionsAndViolationShapes` | violation 的 code 不合法、field 不匹配 `^[a-z][A-Za-z0-9.]{0,126}$`、message 为空或不安全。 | 构造失败；违规形状不进入信封。 | [`domain/fault/fault_test.go`](../../domain/fault/fault_test.go) · `TestFaultRejectsInvalidOptionsAndViolationShapes` |
| `TestFaultValidationErrorsDoNotReflectRejectedValues` | 用被拒绝的值本身触发校验失败。 | 返回的 error 文案不回显该值——被拒值按定义是任意调用方输入。 | [`domain/fault/fault_test.go`](../../domain/fault/fault_test.go) · `TestFaultValidationErrorsDoNotReflectRejectedValues` |
| `TestFaultRejectsMaliciousTextAtPublicBoundaries` | 8 类恶意文本注入 message 与 param value。 | 构造期一律拒绝，不在渲染期过滤。 | [`domain/fault/fault_test.go`](../../domain/fault/fault_test.go) · `TestFaultRejectsMaliciousTextAtPublicBoundaries` |
| `TestPublicTextRejectsInvisibleAndReorderingCharacters` | 12 种不可见/重排字符（no-break space、ogham space、en quad、bidi override、bidi isolate、零宽连接符、组合标记、变体选择符、U+3164 HANGUL FILLER 等）。 | 公开文本被限定为可打印 ASCII（0x20–0x7E）；「渲染结果与字面不一致」的字符全部拒绝。 | [`domain/fault/fault_test.go`](../../domain/fault/fault_test.go) · `TestPublicTextRejectsInvisibleAndReorderingCharacters` |
| `TestFormattingNeverReachesThePrivateCause` | 对携带敏感 cause 的 fault 依次施加 `%v`、`%s`、`%q`、`%+v`、`%#v`。 | 五个动词全部只渲染安全表面；`%#v`/`%+v` 的默认结构体反射不得打印未导出的 cause。 | [`domain/fault/fault_test.go`](../../domain/fault/fault_test.go) · `TestFormattingNeverReachesThePrivateCause` |
| `TestOverCapViolationsTruncateInsteadOfFailingConstruction` | `MaxViolations + 10` 条 violation。 | 截断到确定性前 32 条并构造成功，而非构造失败——敌意输入不得把校验变成拒绝服务。 | [`domain/fault/fault_test.go`](../../domain/fault/fault_test.go) · `TestOverCapViolationsTruncateInsteadOfFailingConstruction` |
| `TestCodeSatisfiesErrorWithItsOwnString` | 把 `Code` 装进 `error` 接口后调用 `Error()` 并用 `%v` 渲染。 | 输出等于 code 字符串本身；这是 `errors.Is(err, someCode)` 成立的前提，因此任何匹配过 sentinel 的宿主都可能触达它。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestCodeSatisfiesErrorWithItsOwnString` |
| `TestParamAccessorsReturnWhatWasConstructed` | 正常构造的 `Param`，以及零值 `Param`。 | 访问器回读构造值；零值返回空串而非 panic。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestParamAccessorsReturnWhatWasConstructed` |
| `TestPopulatedFaultReadsBackThroughItsOwnAccessors` | 带 param 与 violation 的非 nil fault。 | `Kind`/`Code`/`Message`/`Params`/`Violations` 的**非 nil 分支**回读构造值。此前包内测试只经 `KindOf`/`Describe` 读取，非 nil 分支从未执行。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestPopulatedFaultReadsBackThroughItsOwnAccessors` |
| `TestNilFaultCollectionAccessorsReturnNil` | nil `*Error` 上的 `Params()` 与 `Violations()`。 | 返回 nil 切片而非 panic 或空切片。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestNilFaultCollectionAccessorsReturnNil` |
| `TestFormatOfANilFaultStaysPrintable` | nil `*Error` 经 5 个 fmt 动词渲染。 | 全部渲染出 `<nil>`；日志语句中的 panic 会让诊断路径打垮它本要诊断的东西。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestFormatOfANilFaultStaysPrintable` |
| `TestIsCodeRejectsAnUnusableCode` | 空串、小写、2 字符、含空格、下划线结尾的 `Code`；目标 error 为真 fault 与 nil。 | 一律返回 false，且必须由前置守卫而非巧合决定；合法 code 仍正常匹配。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestIsCodeRejectsAnUnusableCode` |
| `TestNewParamRejectsOversizeValue` | param value 恰好 256 字节与 257 字节。 | 边界值接受，超一字节拒绝。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestNewParamRejectsOversizeValue` |
| `TestNewViolationRejectsUnusableParams` | 传入零值 `Param`——即调用方忽略 `NewParam` 返回的 error 后持有的东西。 | violation 构造器复检而不信任传入值。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestNewViolationRejectsUnusableParams` |
| `TestValidateMessageRejectsOversizeMessage` | message 恰好 512 字节与 513 字节。 | 边界值接受，超一字节拒绝。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestValidateMessageRejectsOversizeMessage` |
| `TestValidateParamsMatrix` | 13 个子案例：空集、恰好 16 个、17 个、零值 param、大写 key、含下划线 key、value 恰好 256/257 字节、含控制字符 value、非 ASCII value、重复 key、不同 key。 | 每条防御分支单独判定，避免一个笼统拒绝掩盖其余分支。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestValidateParamsMatrix` |
| `TestValidateViolationsMatrix` | 10 个子案例：空集、恰好 32 条、33 条、零值 violation、非法 code、非法 field、空 message、不安全 message、含零值 param 的 violation。 | 同上；33 条一行是纵深防御——`construct` 会先截断，但未来的其他调用方未必。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestValidateViolationsMatrix` |
| `TestConstructRejectsAFailingOption` | 一个返回 error 的 `Option`。 | 该 error 原样透出。`Option` 已导出而 `faultOptions` 未导出，因此只有本包能构造失败 option——这正是包外无法覆盖此分支的原因。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestConstructRejectsAFailingOption` |
| `TestConstructRejectsUnusableParams` | 经 `New` 与 `Wrap` 两条路径传入零值 `Param`。 | 两者都拒绝；`Wrap` 不因为多了 cause 就放宽校验。 | [`domain/fault/fault_surface_test.go`](../../domain/fault/fault_surface_test.go) · `TestConstructRejectsUnusableParams` |

## Cross-cutting / Conformance Cases

错误码注册表与实现的一致性不在本表，由 [`architecture/fault_contract_guard_test.go`](../../architecture/fault_contract_guard_test.go) 强制：(Kind, Code) 配对、跨上下文前缀所有权、未注册码、未导出码常量、导出哨兵 error、以及 violation reason code 不得作为顶层 `Error` 的 code。外部消费者视角的兼容性证据见 [`contract/fault_public_api_test.go`](../../contract/fault_public_api_test.go)。

## 维护规则

1. 新增或删除 `Test…` 函数时，必须同步更新本表；表驱动新增子案例要更新相应行的边界描述。
2. 新增公开 domain API 或 application use case 时，必须先添加公开入口清单行和至少一条可执行测试证据。
3. 文档不替代测试；冲突时以 Go 测试断言和领域契约为准。
4. 本表的边界列写真实输入与预期。新增行不得复用占位文字——错误内核没有第二份矩阵可以兜底。
