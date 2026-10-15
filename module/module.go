// Package module is the registry of the optional features compiled into V2bX.
//
// A feature lives in its own package under module/ and is compiled in only with its build tag:
// module/imports/<name>.go imports it under that tag (as core/imports does for the cores), so a
// build without the tag carries none of its code. Removing a feature means deleting its package
// and that one import file.
//
// A feature registers itself from init() for the cores it works with. Every request to the panel
// names the features of the node's core (api/panel), so the panel never counts on one the node
// doesn't have.
package module

import (
	"slices"
	"sync"
)

var (
	mu       sync.RWMutex
	features = map[string]map[string]bool{} // feature -> core types
)

// Register declares that a feature works on the given core types ("sing", "xray", "hysteria2").
func Register(feature string, cores ...string) {
	mu.Lock()
	defer mu.Unlock()
	if features[feature] == nil {
		features[feature] = map[string]bool{}
	}
	for _, c := range cores {
		features[feature][c] = true
	}
}

// For lists, sorted, the features a node served by the given core type has.
func For(core string) []string {
	mu.RLock()
	defer mu.RUnlock()
	var list []string
	for name, cores := range features {
		if cores[core] {
			list = append(list, name)
		}
	}
	slices.Sort(list)
	return list
}
