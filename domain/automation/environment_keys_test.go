package automation

import (
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/interpolation"
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
	}{
		{name: "unterminated", value: "${env.host"},
		{name: "empty name", value: "${}"},
		{name: "leading whitespace", value: "${ env.host}"},
		{name: "trailing whitespace", value: "${env.host }"},
		{name: "nested expression marker", value: "${env.${host}}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := EnvironmentKeys(test.value)
			if !fault.IsCode(err, interpolation.CodeExpressionInvalid) {
				t.Fatalf("EnvironmentKeys() error = %v", err)
			}
		})
	}
}
