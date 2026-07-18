package interpolation

import (
	"slices"
	"strings"
	"testing"
)

type variableMap map[string]string

func (values variableMap) Variable(name string) (string, bool) {
	value, ok := values[name]
	return value, ok
}

func TestNamesAndExpandExpressionMatrix(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		values     variableMap
		wantNames  []string
		want       string
		wantErr    string
	}{
		{name: "plain", expression: "plain", want: "plain"},
		{name: "adjacent", expression: "${a}${b}", values: variableMap{"a": "A", "b": "B"}, wantNames: []string{"a", "b"}, want: "AB"},
		{name: "duplicates preserve order", expression: "${a}/${a}", values: variableMap{"a": "A"}, wantNames: []string{"a", "a"}, want: "A/A"},
		{name: "unicode", expression: "${租户}", values: variableMap{"租户": "北区"}, wantNames: []string{"租户"}, want: "北区"},
		{name: "empty resolved value", expression: "x${a}y", values: variableMap{"a": ""}, wantNames: []string{"a"}, want: "xy"},
		{name: "non recursive", expression: "${a}", values: variableMap{"a": "${b}", "b": "B"}, wantNames: []string{"a"}, want: "${b}"},
		{name: "undefined", expression: "${missing}", values: variableMap{}, wantNames: []string{"missing"}, wantErr: "undefined variable"},
		{name: "unterminated", expression: "${a", wantErr: "unterminated"},
		{name: "empty name", expression: "${}", wantErr: "empty variable"},
		{name: "whitespace", expression: "${ a }", wantErr: "invalid variable name"},
		{name: "nested dollar", expression: "${a$b}", wantErr: "invalid variable name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			names, namesErr := Names(test.expression)
			got, expandErr := Expand(test.expression, test.values)
			if test.wantErr != "" {
				if namesErr == nil && !strings.Contains(test.wantErr, "undefined") {
					t.Fatalf("Names accepted invalid expression: %v", names)
				}
				if expandErr == nil || !strings.Contains(expandErr.Error(), test.wantErr) {
					t.Fatalf("Expand error=%v want containing %q", expandErr, test.wantErr)
				}
				return
			}
			if namesErr != nil || expandErr != nil || !slices.Equal(names, test.wantNames) || got != test.want {
				t.Fatalf("Names/Expand = (%v,%v,%q,%v), want (%v,nil,%q,nil)", names, namesErr, got, expandErr, test.wantNames, test.want)
			}
		})
	}
}

func TestExpandRejectsNilResolverOnlyWhenNeeded(t *testing.T) {
	if got, err := Expand("plain", nil); err != nil || got != "plain" {
		t.Fatalf("plain value with nil resolver = %q, %v", got, err)
	}
	if _, err := Expand("${name}", nil); err == nil || !strings.Contains(err.Error(), "resolver is required") {
		t.Fatalf("nil resolver error = %v", err)
	}
}

func FuzzNamesAndExpandShareGrammar(f *testing.F) {
	for _, seed := range []string{"plain", "${a}", "${a}/${b}", "${}", "${unterminated", "${ a }"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, expression string) {
		names, namesErr := Names(expression)
		resolver := variableMap{}
		for _, name := range names {
			resolver[name] = name
		}
		_, expandErr := Expand(expression, resolver)
		if (namesErr == nil) != (expandErr == nil) {
			t.Fatalf("grammar disagreement for %q: Names=%v Expand=%v", expression, namesErr, expandErr)
		}
	})
}

func TestNamesAndExpandShareOneCaseSensitiveGrammar(t *testing.T) {
	expression := "${env.base_url}/users/${params.User}/${env.Tenant}"
	names, err := Names(expression)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names, []string{"env.base_url", "params.User", "env.Tenant"}) {
		t.Fatalf("names = %#v", names)
	}
	expanded, err := Expand(expression, variableMap{
		"env.base_url": "https://example.test", "params.User": "alice", "env.Tenant": "North",
	})
	if err != nil || expanded != "https://example.test/users/alice/North" {
		t.Fatalf("expanded=%q err=%v", expanded, err)
	}
	if _, err := Expand("${env.tenant}", variableMap{"env.Tenant": "North"}); err == nil {
		t.Fatal("case-insensitive environment lookup was accepted")
	}
	for _, malformed := range []string{"${}", "${env.key", "${ env.key }"} {
		if _, err := Names(malformed); err == nil {
			t.Fatalf("malformed expression %q accepted", malformed)
		}
	}
}
