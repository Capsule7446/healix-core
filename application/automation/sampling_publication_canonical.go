package automation

import (
	"hash"
	"math"
	"reflect"
	"sort"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

// 采样发布请求摘要使用稳定遍历编码完整负载。三个参数类型的字段未导出，编码器通过其公共
// 访问器读取值；application 层负责该编码，domain/execution 使用相同技术生成快照摘要。
// 编码契约包括：
//   - 每个变长值带长度前缀，确保不同负载不会因边界移动得到相同字节流。
//   - Map 键按序排列，使摘要不依赖 Go 的 map 遍历顺序。
//   - 结构体先写入字段数量，字段集合变化会反映到摘要中。
//   - 浮点数写入精确、可移植的 Float64 位模式。

// encodeCanonicalPayload 以稳定、无歧义的字节序列编码发布负载，用于生成请求摘要。
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
	// 三个不透明类型通过公共访问器读取内部值。
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
		// 先写入字段数量，使字段集合变化反映到摘要。
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
		// 浮点值按 Float64bits 提取精确、可移植的位模式，本模块另外两个摘要编码器亦采用此表示。
		writeDigestUint64(h, math.Float64bits(value.Float()))
	}
}

// encodeCanonicalParameterValue 通过参数公共访问器按类型编码参数值。
func encodeCanonicalParameterValue(h hash.Hash, value parameter.Value) {
	writeDigestString(h, string(value.Type()))
	switch value.Type() {
	case parameter.Text, parameter.Number, parameter.SingleSelect:
		writeDigestString(h, value.Text())
	case parameter.Boolean:
		writeDigestBool(h, value.Boolean())
	case parameter.MultiSelect:
		// 多选项目按其值内顺序编码。
		items := value.MultiSelect()
		writeDigestUint64(h, uint64(len(items)))
		for _, item := range items {
			writeDigestString(h, item)
		}
	}
}

// writeDigestString 以长度前缀写入字符串，防止变长值之间发生边界歧义。
func writeDigestString(h hash.Hash, value string) {
	writeDigestUint64(h, uint64(len(value)))
	_, _ = h.Write([]byte(value))
}

// writeDigestUint64 以固定的大端字节序写入无符号整数。
func writeDigestUint64(h hash.Hash, value uint64) {
	var buffer [8]byte
	for index := 7; index >= 0; index-- {
		buffer[index] = byte(value)
		value >>= 8
	}
	_, _ = h.Write(buffer[:])
}

// writeDigestBool 写入单字节布尔值表示。
func writeDigestBool(h hash.Hash, value bool) {
	if value {
		_, _ = h.Write([]byte{1})
		return
	}
	_, _ = h.Write([]byte{0})
}
