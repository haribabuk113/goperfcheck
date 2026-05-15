package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestLazyInitChecker_InitFunc(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantN   int
		wantMsg string // optional substring in first issue message
	}{
		{
			name: "sql.Open in init triggers",
			src: `package p
import "database/sql"
var db *sql.DB
func init() {
	db, _ = sql.Open("postgres", "dsn")
}`,
			wantN:   1,
			wantMsg: "sql.Open()",
		},
		{
			name: "os.Open in init triggers",
			src: `package p
import "os"
func init() {
	f, _ := os.Open("config.json")
	_ = f
}`,
			wantN:   1,
			wantMsg: "os.Open()",
		},
		{
			name: "os.Create in init triggers",
			src: `package p
import "os"
func init() {
	f, _ := os.Create("out.log")
	_ = f
}`,
			wantN: 1,
		},
		{
			name: "redis.NewClient in init triggers",
			src: `package p
import "github.com/go-redis/redis"
func init() {
	rdb := redis.NewClient(nil)
	_ = rdb
}`,
			wantN: 1,
		},
		{
			name: "grpc.Dial in init triggers",
			src: `package p
import "google.golang.org/grpc"
func init() {
	conn, _ := grpc.Dial("localhost:50051")
	_ = conn
}`,
			wantN: 1,
		},
		{
			name: "net.Listen in init triggers",
			src: `package p
import "net"
func init() {
	ln, _ := net.Listen("tcp", ":8080")
	_ = ln
}`,
			wantN: 1,
		},
		{
			name: "mongo.Connect in init triggers",
			src: `package p
import "go.mongodb.org/mongo"
func init() {
	c, _ := mongo.Connect(nil, nil)
	_ = c
}`,
			wantN: 1,
		},
		{
			name: "multiple expensive calls in init each trigger",
			src: `package p
import (
	"database/sql"
	"os"
)
func init() {
	db, _ := sql.Open("postgres", "dsn")
	_ = db
	f, _ := os.Open("config.json")
	_ = f
}`,
			wantN: 2,
		},
		{
			name: "non-expensive call in init does not trigger",
			src: `package p
import "fmt"
func init() {
	fmt.Println("started")
}`,
			wantN: 0,
		},
		{
			name: "expensive call in regular function does not trigger",
			src: `package p
import "database/sql"
func setup() {
	db, _ := sql.Open("postgres", "dsn")
	_ = db
}`,
			wantN: 0,
		},
		{
			name: "function named initDB does not trigger (only init() counts)",
			src: `package p
import "database/sql"
func initDB() {
	db, _ := sql.Open("postgres", "dsn")
	_ = db
}`,
			wantN: 0,
		},
	}

	c := &LazyInitChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			// Only count init()-triggered issues (message contains "in init()")
			var initIssues []Issue
			for _, iss := range issues {
				if contains(iss.Message, "in init()") {
					initIssues = append(initIssues, iss)
				}
			}
			if len(initIssues) != tt.wantN {
				t.Errorf("got %d init() issue(s), want %d", len(initIssues), tt.wantN)
				for _, iss := range initIssues {
					t.Logf("  issue: %s", iss.Message)
				}
			}
			if tt.wantMsg != "" && len(initIssues) > 0 {
				if !contains(initIssues[0].Message, tt.wantMsg) {
					t.Errorf("message %q does not contain %q", initIssues[0].Message, tt.wantMsg)
				}
			}
		})
	}
}

func TestLazyInitChecker_PackageLevel(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantN   int
		wantMsg string
	}{
		{
			name: "package-level sql.Open triggers",
			src: `package p
import "database/sql"
var db, _ = sql.Open("postgres", "dsn")`,
			wantN:   1,
			wantMsg: "package-level sql.Open()",
		},
		{
			name: "package-level os.Open triggers",
			src: `package p
import "os"
var configFile, _ = os.Open("config.json")`,
			wantN: 1,
		},
		{
			name: "package-level os.Create triggers",
			src: `package p
import "os"
var logFile, _ = os.Create("app.log")`,
			wantN: 1,
		},
		{
			name: "package-level redis.NewClient triggers",
			src: `package p
import "github.com/go-redis/redis"
var rdb = redis.NewClient(nil)`,
			wantN: 1,
		},
		{
			name: "package-level grpc.NewServer triggers",
			src: `package p
import "google.golang.org/grpc"
var srv = grpc.NewServer()`,
			wantN: 1,
		},
		{
			name: "package-level net.Dial triggers",
			src: `package p
import "net"
var conn, _ = net.Dial("tcp", "localhost:80")`,
			wantN: 1,
		},
		{
			name: "multiple package-level expensive vars each trigger",
			src: `package p
import (
	"database/sql"
	"os"
)
var db, _ = sql.Open("postgres", "dsn")
var f, _ = os.Open("config.json")`,
			wantN: 2,
		},
		{
			name: "package-level simple literal does not trigger",
			src: `package p
var maxRetries = 3`,
			wantN: 0,
		},
		{
			name: "package-level unknown function call does not trigger",
			src: `package p
var cfg = loadConfig()`,
			wantN: 0,
		},
		{
			name: "package-level http.ListenAndServe triggers",
			src: `package p
import "net/http"
var _ = http.ListenAndServe(":8080", nil)`,
			wantN: 1,
		},
	}

	c := &LazyInitChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			// Only count package-level issues (message contains "package-level")
			var pkgIssues []Issue
			for _, iss := range issues {
				if contains(iss.Message, "package-level") {
					pkgIssues = append(pkgIssues, iss)
				}
			}
			if len(pkgIssues) != tt.wantN {
				t.Errorf("got %d package-level issue(s), want %d", len(pkgIssues), tt.wantN)
				for _, iss := range pkgIssues {
					t.Logf("  issue: %s", iss.Message)
				}
			}
			if tt.wantMsg != "" && len(pkgIssues) > 0 {
				if !contains(pkgIssues[0].Message, tt.wantMsg) {
					t.Errorf("message %q does not contain %q", pkgIssues[0].Message, tt.wantMsg)
				}
			}
		})
	}
}

func TestLazyInitChecker_IssueFields(t *testing.T) {
	t.Run("init() issue fields", func(t *testing.T) {
		src := `package p
import "database/sql"
func init() {
	db, _ := sql.Open("postgres", "dsn")
	_ = db
}`
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "test.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		c := &LazyInitChecker{}
		issues := c.Check(fset, f)
		if len(issues) != 1 {
			t.Fatalf("got %d issue(s), want 1", len(issues))
		}
		iss := issues[0]
		if iss.Checker != "LazyInit" {
			t.Errorf("Checker = %q, want LazyInit", iss.Checker)
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
		if !contains(iss.Suggestion, "sync.OnceValue") {
			t.Errorf("Suggestion %q should mention sync.OnceValue", iss.Suggestion)
		}
		if iss.Benchmark == "" {
			t.Error("Benchmark should be non-empty")
		}
	})

	t.Run("package-level issue fields", func(t *testing.T) {
		src := `package p
import "os"
var f, _ = os.Open("config.json")`
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "test.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		c := &LazyInitChecker{}
		issues := c.Check(fset, f)
		if len(issues) != 1 {
			t.Fatalf("got %d issue(s), want 1", len(issues))
		}
		iss := issues[0]
		if iss.Checker != "LazyInit" {
			t.Errorf("Checker = %q, want LazyInit", iss.Checker)
		}
		if iss.Severity != SeverityWarning {
			t.Errorf("Severity = %v, want Warning", iss.Severity)
		}
		if !contains(iss.Suggestion, "sync.OnceValue") {
			t.Errorf("Suggestion %q should mention sync.OnceValue", iss.Suggestion)
		}
	})
}
