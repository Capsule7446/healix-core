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
	// The canonical walk reads these three through their accessors.
	handled := map[string]bool{
		"parameter.Value":         true,
		"parameter.OptionalValue": true,
		"parameter.Binding":       true,
	}
	assertNoUnhandledOpaqueType(t, reflect.TypeOf(domain.SamplingPublication{}), "SamplingPublication", handled)
}

// The other two digests in this package still hash json.Marshal of their
// payload, which is correct only while nothing in that payload hides its state.
// Both are all-exported today; so was the step transition commit until a value
// object landed in it, and so was the publication payload. Nothing marked the
// difference, so nothing objected when it changed.
//
// json.Marshal has no handled set at all: a type it cannot see becomes {} with
// no error, so anything opaque reaching these is a defect by arrival.
func TestNoOpaqueTypeReachesAJSONMarshalledDigest(t *testing.T) {
	for _, payload := range []struct {
		name string
		typ  reflect.Type
	}{
		{"HealReviewRequest", reflect.TypeOf(HealReviewRequest{})},
		{"HealStreak", reflect.TypeOf(domain.HealStreak{})},
	} {
		t.Run(payload.name, func(t *testing.T) {
			assertNoUnhandledOpaqueType(t, payload.typ, payload.name, nil)
		})
	}
}

func assertNoUnhandledOpaqueType(t *testing.T, root reflect.Type, rootName string, handled map[string]bool) {
	t.Helper()

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
	walk(root, rootName, 0)

	if len(unhandled) != 0 {
		t.Fatalf("%d type(s) reachable from %s have no exported fields and are not handled, so the "+
			"digest cannot tell two different values of them apart:\n  %s",
			len(unhandled), rootName, strings.Join(unhandled, "\n  "))
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
