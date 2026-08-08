# domain/weburl 测试用例矩阵

## 范围与口径

本表记录 `domain/weburl` 的公开业务入口和全部顶层 Go testcase。Go 测试源码是唯一可执行事实；表驱动测试的全部子案例由其对应的测试函数统一引用。

`domain/weburl` 统一维护 Core 的「绝对 HTTP(S) URL」共享规则：覆盖 base URL、采样起始 URL 与 navigation URL，并检查控制字符、userinfo、host 以及插值场景。规则的唯一判定处是本包；属于具体调用方的用例留在对应矩阵。

`Check` 返回封闭的 `Rejection` 词表而非 `error`：每个有界上下文用自己的 fault code、字段名和安全文案报告失败，共享内核不替它们决定这些。

## 公开 API 与领域入口

| 公开入口 | 定义文件 | 测试证据状态 |
|---|---|---|
| `Rejection`（6 个封闭常量） | [`domain/weburl/weburl.go`](../../domain/weburl/weburl.go) | `TestCheckMatrix` 逐个覆盖；`FuzzCheckNeverPanicsAndIsTotal` 断言不会返回词表之外的值。 |
| `Check` | [`domain/weburl/weburl.go`](../../domain/weburl/weburl.go) | 全表主入口。 |
| `Accept` | [`domain/weburl/weburl.go`](../../domain/weburl/weburl.go) | `TestCheckMatrix` 每行同时断言 `Accept` 与 `Check` 一致。 |

## 测试用例证据矩阵

| 测试用例 | 输入、边界或业务前置状态 | 预期契约 | 可执行证据 |
|---|---|---|---|
| `TestCheckMatrix` | 31 个用例分五组：接受组（https/http、大写 scheme、显式端口、IPv4、IPv6、query+fragment、国际化域名、结尾斜杠）；控制字符组（NUL/CR/LF/TAB/DEL，以及一个同时含控制字符和非法 scheme 的 URL）；非绝对组（空串、相对路径、协议相对、裸主机、纯空白）；scheme 组（`javascript:`/`data:`/`file:`/`ftp:`/`about:`/`chrome:`）；userinfo 组（`user@`、`user:pass@`、空 userinfo、`trusted.test@evil.test`）；空 host 组（`https:///path`、`https://`）。 | 每个用例的 `Rejection` 必须与该行声明完全一致，`Accept` 与之不得分歧。判定顺序是契约的一部分：控制字符优先处理——含 CR 的值在解析阶段即可拆分下游请求；userinfo 优先于 host——`trusted.test@evil.test` 的 host 合法，问题在于它读起来像 trusted.test。 | [`domain/weburl/weburl_test.go`](../../domain/weburl/weburl_test.go) · `TestCheckMatrix` |
| `TestRejectionsCarryNoCallerInput` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 返回的 `Rejection` 不含 `hunter2`、`s3cr3t`、`evil.test` 或整条 URL——封闭词表是各上下文可以直接把它放进私有 cause 而无需复审的原因。 | [`domain/weburl/weburl_test.go`](../../domain/weburl/weburl_test.go) · `TestRejectionsCarryNoCallerInput` |

## 跨入口与一致性用例

各调用方如何把 `Rejection` 映射到自己的 fault code 与字段，属于该调用方的契约，证据在其所属矩阵：`domain/execution`（sealed navigation URL）、`domain/automation`（environment base URL）、`domain/sampling`（session start URL）、`domain/node`（运行期 navigate）。`FuzzCheckNeverPanicsAndIsTotal` 是模糊测试目标，不作为具名用例列行。

## 维护规则

1. 新增或删除 `Test…` 函数时，必须同步更新本表；表驱动新增子案例要更新相应行的边界描述。
2. 新增公开 domain API 或 application use case 时，必须先添加公开入口清单行和至少一条可执行测试证据。
3. 文档不替代测试；冲突时以 Go 测试断言和领域契约为准。
4. 新增 URL 规则只能加在 `Check` 里。调用方不得重新手写 scheme、host 或 userinfo 判断；统一入口保证所有消费者使用同一规则。
