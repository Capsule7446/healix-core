package automation

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

func TestProbeGraph(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var opaque, mapkeys, unhandled, ifaces []string
	var walk func(reflect.Type, string)
	walk = func(tp reflect.Type, path string) {
		if seen[tp] {
			return
		}
		seen[tp] = true
		switch tp.Kind() {
		case reflect.Struct:
			exported := 0
			for i := 0; i < tp.NumField(); i++ {
				if tp.Field(i).IsExported() {
					exported++
				}
			}
			if exported == 0 {
				opaque = append(opaque, fmt.Sprintf("%s (%s)", tp.String(), path))
			}
			for i := 0; i < tp.NumField(); i++ {
				walk(tp.Field(i).Type, path+"."+tp.Field(i).Name)
			}
		case reflect.Slice, reflect.Array, reflect.Pointer:
			walk(tp.Elem(), path+"[]")
		case reflect.Map:
			mapkeys = append(mapkeys, fmt.Sprintf("%s key=%s (%s)", tp.String(), tp.Key().Kind(), path))
			walk(tp.Key(), path+"<key>")
			walk(tp.Elem(), path+"[k]")
		case reflect.Interface:
			ifaces = append(ifaces, fmt.Sprintf("%s (%s)", tp.String(), path))
		case reflect.String, reflect.Bool,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
		default:
			unhandled = append(unhandled, fmt.Sprintf("%s kind=%s (%s)", tp.String(), tp.Kind(), path))
		}
	}
	walk(reflect.TypeOf(domain.SamplingPublication{}), "SamplingPublication")
	sort.Strings(opaque)
	sort.Strings(mapkeys)
	fmt.Println("=== ZERO-EXPORTED-FIELD STRUCTS ===")
	for _, s := range opaque {
		fmt.Println(" ", s)
	}
	fmt.Println("=== MAPS ===")
	for _, s := range mapkeys {
		fmt.Println(" ", s)
	}
	fmt.Println("=== INTERFACES ===")
	for _, s := range ifaces {
		fmt.Println(" ", s)
	}
	fmt.Println("=== UNHANDLED KINDS ===")
	for _, s := range unhandled {
		fmt.Println(" ", s)
	}
}
