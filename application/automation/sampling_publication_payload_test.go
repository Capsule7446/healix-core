package automation

import (
	"reflect"
	"strings"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

// The publication digest walks the payload by reflection and reads three types
// through their accessors, because those three keep every field unexported and
// reflection cannot see inside them. Anything else with that shape falls through
// to the generic struct arm, which emits a field count and then nothing — the
// same silent {} that made two different publications hash alike under
// json.Marshal.
//
// Handling three types by name only works while three is the whole list. This
// walks the payload's type graph and fails when a fourth appears, which is the
// only way anyone finds out before the digest quietly stops distinguishing it.
func TestEveryOpaqueTypeInThePublicationPayloadIsHandled(t *testing.T) {
	handled := map[string]bool{
		"parameter.Value":         true,
		"parameter.OptionalValue": true,
		"parameter.Binding":       true,
	}

	visited := map[reflect.Type]bool{}
	var unhandled []string

	var walk func(reflect.Type, string, int)
	walk = func(typed reflect.Type, path string, depth int) {
		// The payload is a finite tree today; the bound is here so a future
		// self-referential type fails the test rather than hanging it.
		if depth > 32 || visited[typed] {
			return
		}
		visited[typed] = true
		switch typed.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(typed.Elem(), path+"[]", depth+1)
		case reflect.Map:
			walk(typed.Key(), path+".key", depth+1)
			walk(typed.Elem(), path+".value", depth+1)
		case reflect.Struct:
			exported := 0
			for index := 0; index < typed.NumField(); index++ {
				if typed.Field(index).IsExported() {
					exported++
				}
			}
			if typed.NumField() > 0 && exported == 0 && !handled[shortName(typed)] {
				unhandled = append(unhandled, shortName(typed)+" at "+path)
			}
			for index := 0; index < typed.NumField(); index++ {
				field := typed.Field(index)
				walk(field.Type, path+"."+field.Name, depth+1)
			}
		}
	}
	walk(reflect.TypeOf(domain.SamplingPublication{}), "SamplingPublication", 0)

	if len(unhandled) != 0 {
		t.Fatalf("%d type(s) in the publication payload have no exported fields and are not handled "+
			"by encodeCanonicalPayload, so the digest cannot tell two different values of them apart:\n  %s",
			len(unhandled), strings.Join(unhandled, "\n  "))
	}
}

func shortName(typed reflect.Type) string {
	path := typed.PkgPath()
	if index := strings.LastIndex(path, "/"); index >= 0 {
		path = path[index+1:]
	}
	if path == "" {
		return typed.Name()
	}
	return path + "." + typed.Name()
}
