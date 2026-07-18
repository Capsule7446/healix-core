package workspace

import (
	"reflect"
	"strings"
	"testing"
)

func TestEnvironmentKeysReturnsSortedUniqueCaseSensitiveKeys(t *testing.T) {
	got, err := EnvironmentKeys(
		"https://${env.host}/${env.tenant}",
		"${env.tenant}-${plain}-${Env.host}",
		"${env.a.b}/${env.host}/${env.}",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.b", "host", "tenant"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EnvironmentKeys() = %v, want %v", got, want)
	}
	got, err = EnvironmentKeys("no variables")
	if err != nil || len(got) != 0 {
		t.Fatalf("plain value keys = %v, err=%v", got, err)
	}
}

func TestEnvironmentKeysPropagatesExpressionGrammarErrors(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "unterminated", value: "${env.host", want: "unterminated"},
		{name: "empty name", value: "${}", want: "empty variable"},
		{name: "leading whitespace", value: "${ env.host}", want: "invalid variable name"},
		{name: "trailing whitespace", value: "${env.host }", want: "invalid variable name"},
		{name: "nested expression marker", value: "${env.${host}}", want: "invalid variable name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := EnvironmentKeys(test.value)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("EnvironmentKeys() error = %v, want %q", err, test.want)
			}
		})
	}
}
