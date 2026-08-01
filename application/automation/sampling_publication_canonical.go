package automation

import (
	"hash"
	"math"
	"reflect"
	"sort"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

// The publication request digest used to hash json.Marshal of the whole
// payload. That worked for every field except the three parameter types, whose
// fields are all unexported: encoding/json emits {} for them, silently. Two
// publications differing only in a parameter default therefore hashed the same,
// so republishing an edit under one command id was mistaken for a replay and
// the edit was returned as already applied without ever being written.
//
// This walks the payload instead and reads those three through their public
// accessors. It is the same technique domain/execution uses for its snapshot
// digest, kept here rather than in domain because encoding is an application
// concern and the architecture guard enforces that.
//
// Two properties the walk has to keep, both of which json.Marshal gave for free
// and are easy to lose by hand:
//   - Every variable-length value is length-prefixed, so no two different
//     payloads can produce the same byte stream by shifting a boundary.
//   - Map keys are sorted, so an unordered container cannot make the digest
//     depend on Go's map iteration order.

func encodeCanonicalPayload(h hash.Hash, value reflect.Value) {
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			writeDigestBool(h, false)
			return
		}
		writeDigestBool(h, true)
		encodeCanonicalPayload(h, value.Elem())
		return
	}
	// The three opaque types, read through the accessors that are the only way
	// to see inside them at all.
	if value.CanInterface() {
		switch typed := value.Interface().(type) {
		case parameter.Value:
			encodeCanonicalParameterValue(h, typed)
			return
		case parameter.OptionalValue:
			literal, present := typed.Value()
			writeDigestBool(h, present)
			if present {
				encodeCanonicalParameterValue(h, literal)
			}
			return
		case parameter.Binding:
			writeDigestString(h, string(typed.Kind()))
			if literal, ok := typed.Literal(); ok {
				writeDigestBool(h, true)
				encodeCanonicalParameterValue(h, literal)
				return
			}
			writeDigestBool(h, false)
			if name, ok := typed.ParentName(); ok {
				writeDigestString(h, name)
			}
			return
		}
	}
	switch value.Kind() {
	case reflect.Pointer:
		writeDigestBool(h, !value.IsNil())
		if !value.IsNil() {
			encodeCanonicalPayload(h, value.Elem())
		}
	case reflect.Struct:
		// The field count leads, so adding a field changes the digest rather than
		// letting the new value slide into the previous field's bytes.
		writeDigestUint64(h, uint64(value.NumField()))
		for index := 0; index < value.NumField(); index++ {
			encodeCanonicalPayload(h, value.Field(index))
		}
	case reflect.Slice, reflect.Array:
		writeDigestUint64(h, uint64(value.Len()))
		for index := 0; index < value.Len(); index++ {
			encodeCanonicalPayload(h, value.Index(index))
		}
	case reflect.Map:
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		writeDigestUint64(h, uint64(len(keys)))
		for _, key := range keys {
			writeDigestString(h, key.String())
			encodeCanonicalPayload(h, value.MapIndex(key))
		}
	case reflect.String:
		writeDigestString(h, value.String())
	case reflect.Bool:
		writeDigestBool(h, value.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		writeDigestUint64(h, uint64(value.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		writeDigestUint64(h, value.Uint())
	case reflect.Float32, reflect.Float64:
		// Float64bits, not a scaled integer conversion. Converting a float to an
		// integer is implementation-dependent for NaN and for anything outside the
		// integer range, which would make the digest differ by architecture rather
		// than by payload. The bit pattern is exact and portable, and it is what
		// the other two digest encoders in this module already use.
		writeDigestUint64(h, math.Float64bits(value.Float()))
	}
}

func encodeCanonicalParameterValue(h hash.Hash, value parameter.Value) {
	writeDigestString(h, string(value.Type()))
	switch value.Type() {
	case parameter.Text, parameter.Number, parameter.SingleSelect:
		writeDigestString(h, value.Text())
	case parameter.Boolean:
		writeDigestBool(h, value.Boolean())
	case parameter.MultiSelect:
		// Order is part of a multi-select value, not an artefact of storage.
		items := value.MultiSelect()
		writeDigestUint64(h, uint64(len(items)))
		for _, item := range items {
			writeDigestString(h, item)
		}
	}
}

func writeDigestString(h hash.Hash, value string) {
	writeDigestUint64(h, uint64(len(value)))
	_, _ = h.Write([]byte(value))
}

func writeDigestUint64(h hash.Hash, value uint64) {
	var buffer [8]byte
	for index := 7; index >= 0; index-- {
		buffer[index] = byte(value)
		value >>= 8
	}
	_, _ = h.Write(buffer[:])
}

func writeDigestBool(h hash.Hash, value bool) {
	if value {
		_, _ = h.Write([]byte{1})
		return
	}
	_, _ = h.Write([]byte{0})
}
