package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestHTTPClientReuseChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "http.Client{} inside function triggers",
			src: `package p
import "net/http"
func fetch(url string) {
	client := http.Client{}
	client.Get(url)
}`,
			wantN: 1,
		},
		{
			name: "http.Client{} with fields inside function triggers",
			src: `package p
import (
	"net/http"
	"time"
)
func fetch(url string) {
	client := http.Client{Timeout: 30 * time.Second}
	client.Get(url)
}`,
			wantN: 1,
		},
		{
			name: "&http.Client{} inside function triggers",
			src: `package p
import "net/http"
func fetch(url string) {
	client := &http.Client{}
	client.Get(url)
}`,
			wantN: 1,
		},
		{
			name: "http.Client{} returned directly from function triggers",
			src: `package p
import "net/http"
func newClient() *http.Client {
	return &http.Client{}
}`,
			wantN: 1,
		},
		{
			name: "http.Client{} inside func literal triggers",
			src: `package p
import "net/http"
func f() {
	fn := func() {
		client := http.Client{}
		_ = client
	}
	fn()
}`,
			wantN: 1,
		},
		{
			name: "http.Client{} inside goroutine literal triggers",
			src: `package p
import "net/http"
func f() {
	go func() {
		client := http.Client{}
		_ = client
	}()
}`,
			wantN: 1,
		},
		{
			name: "multiple http.Client{} in same function each trigger",
			src: `package p
import "net/http"
func f(urls []string) {
	for _, u := range urls {
		client := http.Client{}
		client.Get(u)
	}
}`,
			wantN: 1,
		},
		{
			name: "http.Client{} at package level does not trigger",
			src: `package p
import "net/http"
var client = http.Client{}`,
			wantN: 0,
		},
		{
			name: "&http.Client{} at package level does not trigger",
			src: `package p
import "net/http"
var client = &http.Client{Timeout: 0}`,
			wantN: 0,
		},
		{
			name: "other struct literal inside function does not trigger",
			src: `package p
type MyClient struct{}
func f() {
	c := MyClient{}
	_ = c
}`,
			wantN: 0,
		},
		{
			name: "http.Request{} inside function does not trigger",
			src: `package p
import "net/http"
func f() {
	req := http.Request{}
	_ = req
}`,
			wantN: 0,
		},
		{
			name: "two functions each with http.Client{} each trigger",
			src: `package p
import "net/http"
func a() { c := http.Client{}; _ = c }
func b() { c := http.Client{}; _ = c }`,
			wantN: 2,
		},
	}

	c := &HTTPClientReuseChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			if len(issues) != tt.wantN {
				t.Errorf("got %d issue(s), want %d", len(issues), tt.wantN)
				for _, iss := range issues {
					t.Logf("  issue: %s (line %d)", iss.Message, iss.Line)
				}
			}
		})
	}
}

func TestHTTPClientReuseChecker_IssueFields(t *testing.T) {
	src := `package p
import "net/http"
func fetch(url string) {
	client := http.Client{}
	client.Get(url)
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := &HTTPClientReuseChecker{}
	issues := c.Check(fset, f)
	if len(issues) != 1 {
		t.Fatalf("got %d issue(s), want 1", len(issues))
	}
	iss := issues[0]
	if iss.Checker != "HTTPClientReuse" {
		t.Errorf("Checker = %q, want HTTPClientReuse", iss.Checker)
	}
	if iss.Severity != SeverityWarning {
		t.Errorf("Severity = %v, want Warning", iss.Severity)
	}
	if iss.Line == 0 {
		t.Error("Line should be non-zero")
	}
	if !contains(iss.Rule, "goperf.dev") {
		t.Errorf("Rule %q missing goperf.dev link", iss.Rule)
	}
	if !contains(iss.Message, "http.Client{}") {
		t.Errorf("Message %q should mention http.Client{}", iss.Message)
	}
	if !contains(iss.Suggestion, "package-level") {
		t.Errorf("Suggestion %q should mention package-level", iss.Suggestion)
	}
	if iss.Benchmark == "" {
		t.Error("Benchmark should be non-empty")
	}
}
