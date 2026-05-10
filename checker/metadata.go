package checker

// CheckerMeta holds display information printed by -list-checkers and explain.
type CheckerMeta struct {
	Severity    string // primary severity level this checker reports
	Group       string // theme group (memory | concurrency | io)
	Description string // one-line description of what it catches
	Link        string // goperf.dev reference URL
	BadExample  string // short code snippet that triggers the checker
	GoodExample string // preferred alternative
}

// Metadata maps each checker name to its display metadata.
var Metadata = map[string]CheckerMeta{
	"MemPrealloc": {
		Severity:    "WARN",
		Group:       "memory",
		Description: "append() in loops without capacity; make(map) without size hint",
		Link:        "https://goperf.dev/01-common-patterns/mem-prealloc/",
		BadExample: "var result []string\n" +
			"for _, v := range items {\n" +
			"    result = append(result, v) // reallocates repeatedly\n" +
			"}",
		GoodExample: "result := make([]string, 0, len(items))\n" +
			"for _, v := range items {\n" +
			"    result = append(result, v)\n" +
			"}",
	},
	"ObjectPool": {
		Severity:    "WARN",
		Group:       "memory",
		Description: "high-churn allocations in loops that could use sync.Pool",
		Link:        "https://goperf.dev/01-common-patterns/object-pooling/",
		BadExample: "for range requests {\n" +
			"    buf := &bytes.Buffer{} // new heap object each iteration\n" +
			"    process(buf)\n" +
			"}",
		GoodExample: "var pool = sync.Pool{New: func() any { return &bytes.Buffer{} }}\n" +
			"for range requests {\n" +
			"    buf := pool.Get().(*bytes.Buffer)\n" +
			"    buf.Reset()\n" +
			"    process(buf)\n" +
			"    pool.Put(buf)\n" +
			"}",
	},
	"StructAlign": {
		Severity:    "WARN",
		Group:       "memory",
		Description: "struct fields ordered small→large causing padding waste",
		Link:        "https://goperf.dev/01-common-patterns/fields-alignment/",
		BadExample: "type T struct {\n" +
			"    a bool  // 1 byte + 7 bytes padding\n" +
			"    b int64 // 8 bytes\n" +
			"    c bool  // 1 byte + 7 bytes padding\n" +
			"} // sizeof = 24",
		GoodExample: "type T struct {\n" +
			"    b int64 // 8 bytes\n" +
			"    a bool  // 1 byte\n" +
			"    c bool  // 1 byte + 6 bytes padding\n" +
			"} // sizeof = 16",
	},
	"InterfaceBoxing": {
		Severity:    "INFO",
		Group:       "memory",
		Description: "[]interface{} params and empty interfaces causing heap boxing",
		Link:        "https://goperf.dev/01-common-patterns/interface-boxing/",
		BadExample: "func logValues(args []interface{}) {\n" +
			"    // every concrete value is boxed onto the heap\n" +
			"}",
		GoodExample: "func logValues(args []string) {}\n" +
			"// or use a typed variadic: func log(format string, args ...any)",
	},
	"ZeroCopy": {
		Severity:    "INFO",
		Group:       "io",
		Description: "unnecessary buffer copies (append([]byte{}, src...))",
		Link:        "https://goperf.dev/01-common-patterns/zero-copy/",
		BadExample:  "dst := append([]byte{}, src...) // allocates + copies",
		GoodExample: "dst := bytes.Clone(src) // Go 1.20+, same semantics\n" +
			"// or: use src directly if mutation is not needed",
	},
	"GoroutinePool": {
		Severity:    "WARN",
		Group:       "concurrency",
		Description: "unbounded goroutine creation in loops",
		Link:        "https://goperf.dev/01-common-patterns/worker-pool/",
		BadExample: "for _, item := range items {\n" +
			"    go process(item) // could spawn thousands of goroutines\n" +
			"}",
		GoodExample: "sem := make(chan struct{}, runtime.NumCPU())\n" +
			"for _, item := range items {\n" +
			"    sem <- struct{}{}\n" +
			"    go func(v Item) {\n" +
			"        defer func() { <-sem }()\n" +
			"        process(v)\n" +
			"    }(item)\n" +
			"}",
	},
	"ContextMisuse": {
		Severity:    "ERROR",
		Group:       "concurrency",
		Description: "context.Context stored in struct fields",
		Link:        "https://goperf.dev/01-common-patterns/context/",
		BadExample: "type Server struct {\n" +
			"    ctx context.Context // wrong: context belongs to a call, not a type\n" +
			"}",
		GoodExample: "func (s *Server) Handle(ctx context.Context) error {\n" +
			"    // pass ctx as a parameter to each method that needs it\n" +
			"    return s.db.Query(ctx, query)\n" +
			"}",
	},
	"BufferedIO": {
		Severity:    "WARN",
		Group:       "io",
		Description: "unbuffered file writes in loops; missing Flush()",
		Link:        "https://goperf.dev/01-common-patterns/buffered-io/",
		BadExample: "for _, line := range lines {\n" +
			"    f.Write([]byte(line)) // one syscall per write\n" +
			"}",
		GoodExample: "w := bufio.NewWriter(f)\n" +
			"for _, line := range lines {\n" +
			"    w.WriteString(line)\n" +
			"}\n" +
			"w.Flush()",
	},
	"AtomicMutex": {
		Severity:    "INFO",
		Group:       "concurrency",
		Description: "simple counters/flags using mutexes — prefer sync/atomic",
		Link:        "https://goperf.dev/01-common-patterns/atomic-ops/",
		BadExample: "var mu sync.Mutex\n" +
			"var counter int\n" +
			"mu.Lock()\n" +
			"counter++\n" +
			"mu.Unlock()",
		GoodExample: "var counter atomic.Int64\n" +
			"counter.Add(1)",
	},
	"LazyInit": {
		Severity:    "INFO",
		Group:       "memory",
		Description: "expensive init() and package-level allocations that could be deferred",
		Link:        "https://goperf.dev/01-common-patterns/lazy-init/",
		BadExample:  "var cache = buildExpensiveCache() // runs at program start, always",
		GoodExample: "var cache = sync.OnceValue(buildExpensiveCache)\n" +
			"// initialized on first use, skipped entirely if never called",
	},
	"StackAlloc": {
		Severity:    "INFO",
		Group:       "memory",
		Description: "new(T) on primitives and &localVar forcing heap allocation",
		Link:        "https://goperf.dev/01-common-patterns/stack-alloc/",
		BadExample: "p := new(int) // always heap-allocates\n" +
			"*p = 42\n" +
			"use(p)",
		GoodExample: "v := 42 // lives on the stack unless it escapes\n" +
			"use(&v)",
	},
	"Batching": {
		Severity:    "WARN",
		Group:       "io",
		Description: "individual DB/Redis/HTTP calls in loops",
		Link:        "https://goperf.dev/01-common-patterns/batching-ops/",
		BadExample: "for _, id := range ids {\n" +
			"    row, _ := db.QueryRow(ctx, \"SELECT * FROM t WHERE id=?\", id) // N round-trips\n" +
			"}",
		GoodExample: "rows, _ := db.Query(ctx, \"SELECT * FROM t WHERE id IN (?)\", ids) // 1 round-trip",
	},
	"TimeNowLoop": {
		Severity:    "INFO",
		Group:       "concurrency",
		Description: "time.Now() inside loops — each call is a syscall",
		Link:        "https://goperf.dev/01-common-patterns/time/",
		BadExample: "for _, item := range items {\n" +
			"    ts := time.Now() // syscall on every iteration\n" +
			"    process(item, ts)\n" +
			"}",
		GoodExample: "now := time.Now() // one syscall before the loop\n" +
			"for _, item := range items {\n" +
			"    process(item, now)\n" +
			"}",
	},
	"WaitGroupMisuse": {
		Severity:    "ERROR",
		Group:       "concurrency",
		Description: "wg.Add() called inside goroutine literals — race condition",
		Link:        "https://goperf.dev/01-common-patterns/goroutines/",
		BadExample: "for range items {\n" +
			"    go func() {\n" +
			"        wg.Add(1) // race: goroutine may not run before wg.Wait()\n" +
			"        defer wg.Done()\n" +
			"        work()\n" +
			"    }()\n" +
			"}\n" +
			"wg.Wait()",
		GoodExample: "for range items {\n" +
			"    wg.Add(1) // Add before the go statement\n" +
			"    go func() {\n" +
			"        defer wg.Done()\n" +
			"        work()\n" +
			"    }()\n" +
			"}\n" +
			"wg.Wait()",
	},
	"DeferInLoop": {
		Severity:    "WARN",
		Group:       "concurrency",
		Description: "defer inside a loop allocates a closure per iter and fires at function return",
		Link:        "https://goperf.dev/01-common-patterns/defer/",
		BadExample: "for _, f := range files {\n" +
			"    defer f.Close() // defers pile up; Close() runs after the entire function returns\n" +
			"}",
		GoodExample: "for _, f := range files {\n" +
			"    func() {\n" +
			"        defer f.Close() // closes at the end of each iteration\n" +
			"        process(f)\n" +
			"    }()\n" +
			"}",
	},
	"StringConcatLoop": {
		Severity:    "WARN",
		Group:       "memory",
		Description: "string += in a loop causes O(n²) allocations — use strings.Builder",
		Link:        "https://goperf.dev/01-common-patterns/string-building/",
		BadExample: "s := \"\"\n" +
			"for _, w := range words {\n" +
			"    s += w // copies the entire string on every iteration\n" +
			"}",
		GoodExample: "var sb strings.Builder\n" +
			"for _, w := range words {\n" +
			"    sb.WriteString(w)\n" +
			"}\n" +
			"s := sb.String()",
	},
	"RegexpCompile": {
		Severity:    "WARN",
		Group:       "memory",
		Description: "regexp.Compile/MustCompile inside a function — compile once at package level",
		Link:        "https://goperf.dev/01-common-patterns/precompile-regexp/",
		BadExample: "func valid(s string) bool {\n" +
			"    re := regexp.MustCompile(`\\d+`) // compiled on every call\n" +
			"    return re.MatchString(s)\n" +
			"}",
		GoodExample: "var digitRe = regexp.MustCompile(`\\d+`) // compiled once at startup\n" +
			"func valid(s string) bool {\n" +
			"    return digitRe.MatchString(s)\n" +
			"}",
	},
	"HTTPClientReuse": {
		Severity:    "WARN",
		Group:       "io",
		Description: "http.Client{} created per call — share a package-level client for connection reuse",
		Link:        "https://goperf.dev/01-common-patterns/http-client/",
		BadExample: "func fetch(url string) (*http.Response, error) {\n" +
			"    c := &http.Client{Timeout: 5 * time.Second} // no connection reuse\n" +
			"    return c.Get(url)\n" +
			"}",
		GoodExample: "var httpClient = &http.Client{Timeout: 5 * time.Second}\n" +
			"func fetch(url string) (*http.Response, error) {\n" +
			"    return httpClient.Get(url) // reuses idle connections\n" +
			"}",
	},
}
