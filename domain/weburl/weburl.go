// 包 weburl 拥有 Core 中"绝对 HTTP(S) URL"这一条共享规则。
//
// 该规则原本在四处各写一遍——automation 的 BaseURL、execution 的 environment
// snapshot、execution 的 navigation step、node 的运行期导航——并且已经漂移：
// 两个 BaseURL 校验点缺少控制字符检查，navigation 校验点则在存在插值时跳过了
// host 检查。规则相同而实现分散，漂移只是时间问题，所以规则收敛到这里，各上下文
// 只保留自己的错误码与字段名。
package weburl

import (
	"net/url"
	"strings"
)

// Rejection 说明一个 URL 违反了共享规则的哪一条。
//
// 返回原因而非 error 是刻意的：每个有界上下文用自己的 fault code、字段名和安全
// 文案报告失败，共享内核不应替它们决定这些。原因值本身是封闭词表，不含调用方
// 输入，可以安全地进入私有 cause。
type Rejection string

const (
	Accepted           Rejection = ""
	RejectControlChars Rejection = "control_characters"
	RejectNotAbsolute  Rejection = "not_absolute"
	RejectScheme       Rejection = "scheme_not_http"
	RejectHostMissing  Rejection = "host_missing"
	RejectUserinfo     Rejection = "userinfo_present"
)

// Check 按固定顺序应用共享规则，先判定的原因优先。
//
// 顺序是契约的一部分：控制字符最先判，因为一个含 CR 的值在被解析之前就已经
// 能拆分下游请求，先报告更具体的原因会掩盖这一点。
func Check(value string) Rejection {
	if strings.IndexFunc(value, isControl) >= 0 {
		return RejectControlChars
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme == "" {
		return RejectNotAbsolute
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return RejectScheme
	}
	// userinfo 先于 host 判定：https://trusted.test@evil.test 的 host 是合法的，
	// 真正的问题是它读起来像 trusted.test。
	if parsed.User != nil {
		return RejectUserinfo
	}
	if parsed.Host == "" {
		return RejectHostMissing
	}
	return Accepted
}

// Accept 是只关心通过与否的调用方的简写。
func Accept(value string) bool { return Check(value) == Accepted }

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }
