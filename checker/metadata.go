package checker

// CheckerMeta holds display information printed by -list-checkers.
type CheckerMeta struct {
	Severity    string // primary severity level this checker reports
	Group       string // theme group (memory | concurrency | io)
	Description string // one-line description of what it catches
}

// Metadata maps each checker name to its display metadata.
var Metadata = map[string]CheckerMeta{
	"MemPrealloc":     {"WARN", "memory", "append() in loops without capacity; make(map) without size hint"},
	"ObjectPool":      {"WARN", "memory", "high-churn allocations in loops that could use sync.Pool"},
	"StructAlign":     {"WARN", "memory", "struct fields ordered small→large causing padding waste"},
	"InterfaceBoxing": {"INFO", "memory", "[]interface{} params and empty interfaces causing heap boxing"},
	"ZeroCopy":        {"INFO", "io", "unnecessary buffer copies (append([]byte{}, src...))"},
	"GoroutinePool":   {"WARN", "concurrency", "unbounded goroutine creation in loops"},
	"ContextMisuse":   {"ERROR", "concurrency", "context.Context stored in struct fields"},
	"BufferedIO":      {"WARN", "io", "unbuffered file writes in loops; missing Flush()"},
	"AtomicMutex":     {"INFO", "concurrency", "simple counters/flags using mutexes — prefer sync/atomic"},
	"LazyInit":        {"INFO", "memory", "expensive init() and package-level allocations that could be deferred"},
	"StackAlloc":      {"INFO", "memory", "new(T) on primitives and &localVar forcing heap allocation"},
	"Batching":        {"WARN", "io", "individual DB/Redis/HTTP calls in loops"},
	"TimeNowLoop":     {"INFO", "concurrency", "time.Now() inside loops — each call is a syscall"},
	"WaitGroupMisuse": {"ERROR", "concurrency", "wg.Add() called inside goroutine literals — race condition"},
}
