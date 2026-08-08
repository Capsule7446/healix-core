package scheduling

import "sort"

// sortedKeys 按固定顺序返回映射的键。
//
// 本包中遍历映射并在遇到第一个违规项时返回的每个校验器都需要它。Go 的映射遍历顺序
// 是随机的；没有它，同一个被拒绝的输入在不同运行中会指出不同字段，最坏时还会得到
// 不同种类的失败，因为这些循环有的分支返回未编码错误，有的分支返回已携带参数自身
// 错误码的错误。无论哪种情况顶层错误码都不变，但 fault.IsCode 会遍历整条错误链，
// 因此宿主若按链中更深处的错误码分支，对字节完全相同的输入也会得到不同结果。fault
// 包记录了这一确切风险。
//
// 3e56ba2 证明这类问题是缺陷而非表面问题：只有当底层原因是输入的函数时，稳定错误码
// 才有意义。之后两次清理逐个修复实例，却各自漏掉了本包中的同类位置，因此这里统一
// 命名该迭代策略并让所有调用点复用。
func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
