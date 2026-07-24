package execution

import "testing"

func TestSealedNavigationURLRejectsAuthorityInterpolation(t *testing.T) {
	for _, value := range []string{"https://${host}/path", "https://${host}/path?next=127.0.0.1", "${scheme}://example.test/path"} {
		if err := validateSealedNavigationURL(value); err == nil {
			t.Fatalf("authority interpolation accepted: %q", value)
		}
	}
}

func TestSealedNavigationURLAllowsPathAndQueryInterpolation(t *testing.T) {
	for _, value := range []string{"https://example.test/${path}", "https://example.test/search?q=${query}"} {
		if err := validateSealedNavigationURL(value); err != nil {
			t.Fatalf("safe interpolation rejected: %q: %v", value, err)
		}
	}
}
