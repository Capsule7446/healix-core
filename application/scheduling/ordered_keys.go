package scheduling

import "sort"

// sortedKeys returns a map's keys in a fixed order.
//
// Every validator in this package that ranges a map and returns on its first
// offender needs this. Go randomises map iteration, so without it the same
// rejected input names a different field on a different run — and in the worst
// case a different KIND of failure, because these loops have branches that
// return an uncoded error and branches that return one already carrying a
// parameter's own code. The top-level code stays put either way, but
// fault.IsCode walks the whole chain, so a host that branches on a code from
// deeper in it gets a different answer for byte-identical input. The fault
// package documents that exact hazard.
//
// 3e56ba2 established that this class is a defect rather than a cosmetic
// concern: a stable code is only worth having when the cause underneath it is a
// function of the input. Two later sweeps fixed instances one at a time and
// each missed siblings in this package, so the iteration is named once here and
// every site uses it.
func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
