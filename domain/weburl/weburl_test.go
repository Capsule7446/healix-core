package weburl

import (
	"strings"
	"testing"
)

// The whole point of this package is that one table decides the rule for every
// context. If a case belongs in only one caller, it belongs in that caller's
// tests, not here.
func TestCheckMatrix(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  Rejection
	}{
		{name: "https", value: "https://example.test/path", want: Accepted},
		{name: "http", value: "http://example.test", want: Accepted},
		{name: "uppercase scheme", value: "HTTPS://example.test", want: Accepted},
		{name: "explicit port", value: "https://example.test:8443/p", want: Accepted},
		{name: "ipv4", value: "https://127.0.0.1:3000/p", want: Accepted},
		{name: "ipv6", value: "https://[::1]:3000/p", want: Accepted},
		{name: "query and fragment", value: "https://example.test/a?b=c#d", want: Accepted},
		{name: "internationalized host", value: "https://例え.test/p", want: Accepted},
		{name: "trailing slash", value: "https://example.test/", want: Accepted},

		{name: "null byte", value: "https://example.test/\x00", want: RejectControlChars},
		{name: "carriage return", value: "https://example.test/\r", want: RejectControlChars},
		{name: "line feed", value: "https://example.test/\n", want: RejectControlChars},
		{name: "tab", value: "https://exa\tmple.test", want: RejectControlChars},
		{name: "delete", value: "https://example.test/\x7f", want: RejectControlChars},
		// Control characters outrank every other reason, because a value that
		// can split a request downstream is dangerous before it is malformed.
		{name: "control character in an otherwise bad url", value: "javascript:\x00alert(1)", want: RejectControlChars},

		{name: "empty", value: "", want: RejectNotAbsolute},
		{name: "relative", value: "/path", want: RejectNotAbsolute},
		{name: "scheme relative", value: "//example.test/p", want: RejectNotAbsolute},
		{name: "bare host", value: "example.test/p", want: RejectNotAbsolute},
		{name: "whitespace", value: "   ", want: RejectNotAbsolute},

		{name: "javascript", value: "javascript:alert(1)", want: RejectScheme},
		{name: "data", value: "data:text/html,x", want: RejectScheme},
		{name: "file", value: "file:///etc/passwd", want: RejectScheme},
		{name: "ftp", value: "ftp://example.test/p", want: RejectScheme},
		{name: "about", value: "about:blank", want: RejectScheme},
		{name: "chrome", value: "chrome://settings", want: RejectScheme},

		{name: "userinfo", value: "https://user@example.test/p", want: RejectUserinfo},
		{name: "userinfo with password", value: "https://user:pass@example.test/p", want: RejectUserinfo},
		{name: "empty userinfo", value: "https://@example.test/p", want: RejectUserinfo},
		// The host is valid here; the reason it is refused is that a reader
		// sees trusted.test.
		{name: "userinfo spoofing a host", value: "https://trusted.test@evil.test/p", want: RejectUserinfo},

		{name: "empty host", value: "https:///path", want: RejectHostMissing},
		// "https://" parses cleanly with an empty host rather than failing,
		// so host_missing is the precise reason, not not_absolute.
		{name: "empty host no path", value: "https://", want: RejectHostMissing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Check(test.value); got != test.want {
				t.Fatalf("Check(%q) = %q, want %q", test.value, got, test.want)
			}
			if accepted := Accept(test.value); accepted != (test.want == Accepted) {
				t.Fatalf("Accept(%q) = %v, disagrees with Check", test.value, accepted)
			}
		})
	}
}

// TestRejectionsCarryNoCallerInput lets every context put a Rejection into a
// private cause without re-auditing it. The closed vocabulary is the reason
// that is safe.
func TestRejectionsCarryNoCallerInput(t *testing.T) {
	secret := "https://user:hunter2@evil.test/?token=s3cr3t"

	rejection := Check(secret)

	if rejection == Accepted {
		t.Fatal("the probe URL was accepted")
	}
	for _, fragment := range []string{"hunter2", "s3cr3t", "evil.test", secret} {
		if strings.Contains(string(rejection), fragment) {
			t.Fatalf("rejection %q carries caller input %q", rejection, fragment)
		}
	}
}

func FuzzCheckNeverPanicsAndIsTotal(f *testing.F) {
	for _, seed := range []string{"", "https://example.test", "javascript:x", "https:///p", "https://u@h/p", "\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		got := Check(value)
		switch got {
		case Accepted, RejectControlChars, RejectNotAbsolute, RejectScheme, RejectHostMissing, RejectUserinfo:
		default:
			t.Fatalf("Check(%q) returned the undeclared rejection %q", value, got)
		}
		// An accepted value must satisfy every rule, not just the one that
		// happened to be checked last.
		if got == Accepted && !Accept(value) {
			t.Fatalf("Check and Accept disagree on %q", value)
		}
	})
}
