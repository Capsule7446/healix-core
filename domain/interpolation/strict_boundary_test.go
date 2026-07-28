package interpolation

import (
	"strings"
	"testing"
)

type mapResolver map[string]string

func (r mapResolver) Variable(name string) (string, bool) { value, ok := r[name]; return value, ok }

type pointerResolver struct{}

func (*pointerResolver) Variable(string) (string, bool) { return "", false }

func TestExpandStrictInputs(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		resolver  Resolver
		want      string
		wantError bool
	}{
		{name: "empty", value: "", want: ""},
		{name: "unicode", value: "你好 ${name}", resolver: mapResolver{"name": "世界"}, want: "你好 世界"},
		{name: "control in name", value: "${a\nb}", resolver: mapResolver{}, wantError: true},
		{name: "empty name", value: "${}", resolver: mapResolver{}, wantError: true},
		{name: "unterminated", value: "${name", resolver: mapResolver{}, wantError: true},
		{name: "nested marker", value: "${a${b}}", resolver: mapResolver{}, wantError: true},
		{name: "missing", value: "${missing}", resolver: mapResolver{}, wantError: true},
		{name: "nil resolver", value: "${name}", wantError: true},
		{name: "typed nil resolver", value: "${name}", resolver: (*pointerResolver)(nil), wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Expand(tt.value, tt.resolver)
			if (err != nil) != tt.wantError {
				t.Fatalf("Expand() error = %v, wantError %v", err, tt.wantError)
			}
			if !tt.wantError && got != tt.want {
				t.Fatalf("Expand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNamesStrictSyntaxAndDuplicates(t *testing.T) {
	got, err := Names("${a}-${a}-${变量}")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "a", "变量"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Names() = %v, want %v", got, want)
	}

	for _, malformed := range []string{"${", "${}", "${ a}", "${a b}", "${a${b}}"} {
		if _, err := Names(malformed); err == nil {
			t.Errorf("Names(%q) expected error", malformed)
		}
	}
}
