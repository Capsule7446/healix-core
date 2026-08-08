// Package weburl 提供 Core 共享的绝对 HTTP(S) URL 校验规则。
// 各上下文可将封闭的拒绝原因映射为自己的错误码、字段名和安全文案。
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
	// Accepted 表示 URL 通过共享校验规则。
	Accepted Rejection = ""
	// RejectControlChars 表示 URL 含有 ASCII 控制字符。
	RejectControlChars Rejection = "control_characters"
	// RejectNotAbsolute 表示 URL 不是绝对 URL 或无法解析。
	RejectNotAbsolute Rejection = "not_absolute"
	// RejectScheme 表示 URL scheme 不是 http 或 https。
	RejectScheme Rejection = "scheme_not_http"
	// RejectHostMissing 表示 URL 缺少主机。
	RejectHostMissing Rejection = "host_missing"
	// RejectUserinfo 表示 URL 含有用户信息。
	RejectUserinfo Rejection = "userinfo_present"
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
	// userinfo 先于 host 判定，以确保含凭据的 URL 始终得到明确拒绝原因。
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

// isControl 判断 rune 是否为 ASCII 控制字符。
func isControl(r rune) bool { return r < 0x20 || r == 0x7f }
