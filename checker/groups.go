package checker

import (
	"sort"
	"strings"
)

// Groups maps each theme name to the checker names it contains.
// Names are lower-cased keys; checker names match Checker.Name() exactly.
var Groups = map[string][]string{
	"memory":      {"MemPrealloc", "ObjectPool", "StructAlign", "InterfaceBoxing", "LazyInit", "StackAlloc", "StringConcatLoop", "RegexpCompile"},
	"concurrency": {"GoroutinePool", "ContextMisuse", "AtomicMutex", "TimeNowLoop", "WaitGroupMisuse", "DeferInLoop", "SyncMapMisuse"},
	"io":          {"ZeroCopy", "BufferedIO", "Batching", "HTTPClientReuse"},
}

// SortedGroupNames returns the group names in sorted order.
func SortedGroupNames() []string {
	names := make([]string, 0, len(Groups))
	for g := range Groups {
		names = append(names, g)
	}
	sort.Strings(names)
	return names
}

// CheckersForGroup returns the checkers belonging to the named group (case-insensitive).
// Returns nil, false when the group name is not recognised.
func CheckersForGroup(group string, all []Checker) ([]Checker, bool) {
	names, ok := Groups[strings.ToLower(group)]
	if !ok {
		return nil, false
	}
	inGroup := make(map[string]bool, len(names))
	for _, n := range names {
		inGroup[n] = true
	}
	var matched []Checker
	for _, c := range all {
		if inGroup[c.Name()] {
			matched = append(matched, c)
		}
	}
	return matched, true
}
