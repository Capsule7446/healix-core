package execution

import (
	"strings"
	"testing"
)

// validateSealedNavigationURL is the gate between a plan author and the URL a
// browser will be told to open, so every rule it enforces is a security rule.
// The matrix below carries one row per rule and one row per way a rule has a
// plausible bypass — a scheme check that only looks at a prefix, a userinfo
// check defeated by an encoded `@`, an authority check defeated by a `?`
// appearing before the `//`.
func TestSealedNavigationURLSecurityMatrix(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantReject bool
	}{
		// Scheme. Anything that is not http(s) can reach a local handler, the
		// filesystem, or the JavaScript engine.
		{name: "javascript scheme", value: "javascript:alert(1)", wantReject: true},
		{name: "javascript scheme with slashes", value: "javascript://example.test/%0aalert(1)", wantReject: true},
		{name: "data scheme", value: "data:text/html;base64,PHNjcmlwdD4=", wantReject: true},
		{name: "file scheme", value: "file:///etc/passwd", wantReject: true},
		{name: "file scheme windows path", value: "file://C:/Windows/win.ini", wantReject: true},
		{name: "ftp scheme", value: "ftp://example.test/payload", wantReject: true},
		{name: "chrome scheme", value: "chrome://settings", wantReject: true},
		{name: "about scheme", value: "about:blank", wantReject: true},
		{name: "view-source scheme", value: "view-source:https://example.test", wantReject: true},
		{name: "scheme relative", value: "//example.test/path", wantReject: true},
		{name: "relative path", value: "/path/only", wantReject: true},
		{name: "bare host", value: "example.test/path", wantReject: true},
		{name: "empty", value: "", wantReject: true},
		{name: "uppercase http is a scheme match", value: "HTTPS://example.test/path"},

		// Userinfo. Credentials in a URL are both a leak and a spoofing tool:
		// `https://trusted.test@evil.test` reads as trusted.test to a human.
		{name: "userinfo", value: "https://user@example.test/path", wantReject: true},
		{name: "userinfo with password", value: "https://user:pass@example.test/path", wantReject: true},
		{name: "userinfo spoofing a trusted host", value: "https://trusted.test@evil.test/path", wantReject: true},
		{name: "empty userinfo", value: "https://@example.test/path", wantReject: true},

		// Host. The interpolated variants are the regression: the host check
		// used to be skipped whenever the URL contained any variable, so
		// `https:///${path}` was accepted with no host while the identical URL
		// without the variable was refused.
		{name: "empty host", value: "https:///path", wantReject: true},
		{name: "empty host with interpolated path", value: "https:///${path}", wantReject: true},
		{name: "empty host with interpolated query", value: "http:///?q=${query}", wantReject: true},
		{name: "host is present", value: "https://example.test/path"},

		// Control characters. A raw CR or LF splits a request; a NUL or a
		// tab can truncate a host comparison in a downstream parser.
		{name: "null byte", value: "https://example.test/\x00", wantReject: true},
		{name: "carriage return", value: "https://example.test/\rHost: evil.test", wantReject: true},
		{name: "line feed", value: "https://example.test/\npath", wantReject: true},
		{name: "tab inside host", value: "https://exa\tmple.test/path", wantReject: true},
		{name: "delete character", value: "https://example.test/\x7f", wantReject: true},
		{name: "escape character", value: "https://example.test/\x1b[0m", wantReject: true},

		// Interpolation. Scheme and authority are fixed at seal time; only
		// path, query, and fragment may vary per run.
		{name: "interpolated host", value: "https://${host}/path", wantReject: true},
		{name: "interpolated scheme", value: "${scheme}://example.test/path", wantReject: true},
		{name: "interpolated port", value: "https://example.test:${port}/path", wantReject: true},
		{name: "interpolated userinfo", value: "https://${user}@example.test/path", wantReject: true},
		{name: "interpolated whole url", value: "${url}", wantReject: true},
		{name: "interpolation before the authority ends", value: "https://example.test${suffix}/path", wantReject: true},
		{name: "malformed expression", value: "https://example.test/${unterminated", wantReject: true},
		{name: "empty variable name", value: "https://example.test/${}", wantReject: true},
		{name: "interpolated path", value: "https://example.test/${path}"},
		{name: "interpolated query", value: "https://example.test/search?q=${query}"},
		{name: "interpolated fragment", value: "https://example.test/page#${anchor}"},
		{name: "interpolated path and query", value: "https://example.test/${a}?b=${c}"},

		// Ordinary URLs must keep working; a validator that rejects
		// everything is indistinguishable from one that is never called.
		{name: "plain https", value: "https://example.test"},
		{name: "plain http", value: "http://example.test/path"},
		{name: "explicit port", value: "https://example.test:8443/path"},
		{name: "ipv4 host", value: "https://127.0.0.1:3000/path"},
		{name: "ipv6 host", value: "https://[::1]:3000/path"},
		{name: "query and fragment", value: "https://example.test/a?b=c&d=e#f"},
		{name: "percent encoded path", value: "https://example.test/a%20b"},
		{name: "internationalized host", value: "https://例え.test/path"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSealedNavigationURL(test.value)

			if test.wantReject && err == nil {
				t.Fatalf("validateSealedNavigationURL(%q) = nil, want a rejection", test.value)
			}
			if !test.wantReject && err != nil {
				t.Fatalf("validateSealedNavigationURL(%q) = %v, want it accepted", test.value, err)
			}
		})
	}
}

// TestSealedNavigationURLDefersLengthToTheAggregateBound records where the
// size ceiling actually lives. validateSealedNavigationURL deliberately has no
// length opinion of its own — a second, closer limit would be one more number
// to keep in sync — so the bound is the plan-wide MaxStringBytes. This test
// exists so that if the URL rule ever grows its own limit, the split is a
// decision rather than an accident.
func TestSealedNavigationURLDefersLengthToTheAggregateBound(t *testing.T) {
	oversize := "https://example.test/" + strings.Repeat("a", MaxStringBytes)

	if err := validateSealedNavigationURL(oversize); err != nil {
		t.Fatalf("the URL rule grew a length opinion: %v", err)
	}

	plan := navigationPlanWithURL(oversize)
	if err := validateAggregateInputBounds(plan); err == nil {
		t.Fatalf("a %d byte navigation value passed the aggregate bound", len(oversize))
	}
}

func navigationPlanWithURL(value string) PlanSnapshot {
	return PlanSnapshot{
		Workflows: []WorkflowSnapshot{{
			ID: "root", FlowFragmentID: "root", VersionID: "root-v1", DisplayName: "Root", VersionNumber: 1,
			Steps: []Step{{ID: "open", DisplayName: "Open", Kind: ActionStep, Action: "navigate", Value: value}},
		}},
	}
}

// TestSealedNavigationURLRejectionsStayValueFree pins the leak rule: the
// rejected URL is caller input that may hold a token in its query, so it must
// never come back in the error text.
func TestSealedNavigationURLRejectionsStayValueFree(t *testing.T) {
	secrets := []string{
		"https://user:hunter2@example.test/path",
		"https://example.test/callback?token=s3cr3t",
		"javascript:alert(document.cookie)",
	}
	for _, secret := range secrets {
		err := validateSealedNavigationURL(secret)
		if err == nil {
			continue
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("rejection echoed the URL: %v", err)
		}
		for _, fragment := range []string{"hunter2", "s3cr3t", "document.cookie"} {
			if strings.Contains(err.Error(), fragment) {
				t.Fatalf("rejection leaked %q: %v", fragment, err)
			}
		}
	}
}
