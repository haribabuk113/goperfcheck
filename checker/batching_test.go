package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestBatchingChecker(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantN   int
		wantMsg string // substring to check in first issue message, optional
	}{
		{
			name: "db.Exec in range loop triggers",
			src: `package p
func f(db DB, ids []int) {
	for _, id := range ids {
		db.Exec("DELETE FROM t WHERE id = ?", id)
	}
}`,
			wantN:   1,
			wantMsg: "db.Exec()",
		},
		{
			name: "db.Query in for loop triggers",
			src: `package p
func f(db DB, n int) {
	for i := 0; i < n; i++ {
		db.Query("SELECT 1")
	}
}`,
			wantN:   1,
			wantMsg: "db.Query()",
		},
		{
			name: "db.Insert in range loop triggers",
			src: `package p
func f(db DB, rows []Row) {
	for _, r := range rows {
		db.Insert(r)
	}
}`,
			wantN: 1,
		},
		{
			name: "rdb.Set in range loop triggers",
			src: `package p
func f(rdb Redis, items []Item) {
	for _, item := range items {
		rdb.Set(item.Key, item.Val, 0)
	}
}`,
			wantN:   1,
			wantMsg: "rdb.Set()",
		},
		{
			name: "rdb.HSet in range loop triggers",
			src: `package p
func f(rdb Redis, items []Item) {
	for _, item := range items {
		rdb.HSet("hash", item.Key, item.Val)
	}
}`,
			wantN: 1,
		},
		{
			name: "httpClient.Do in range loop triggers",
			src: `package p
func f(httpClient Client, reqs []*Request) {
	for _, req := range reqs {
		httpClient.Do(req)
	}
}`,
			wantN:   1,
			wantMsg: "httpClient.Do()",
		},
		{
			name: "httpClient.Post in range loop triggers",
			src: `package p
func f(httpClient Client, urls []string) {
	for _, u := range urls {
		httpClient.Post(u, "application/json", nil)
	}
}`,
			wantN: 1,
		},
		{
			name: "producer.SendMessage in range loop triggers",
			src: `package p
func f(producer MQ, msgs []Msg) {
	for _, m := range msgs {
		producer.SendMessage(m)
	}
}`,
			wantN: 1,
		},
		{
			name: "kafka.Produce in range loop triggers",
			src: `package p
func f(kafka K, msgs []Msg) {
	for _, m := range msgs {
		kafka.Produce(m)
	}
}`,
			wantN: 1,
		},
		{
			name: "tx.Exec in range loop triggers",
			src: `package p
func f(tx TX, ids []int) {
	for _, id := range ids {
		tx.Exec("UPDATE t SET x=1 WHERE id=?", id)
	}
}`,
			wantN: 1,
		},
		{
			name: "client.Set in range loop triggers",
			src: `package p
func f(client C, items []Item) {
	for _, item := range items {
		client.Set(item.Key, item.Val, 0)
	}
}`,
			wantN: 1,
		},
		{
			name: "multiple db calls in same loop counted separately",
			src: `package p
func f(db DB, rows []Row) {
	for _, r := range rows {
		db.Query("SELECT id FROM t WHERE k = ?", r.K)
		db.Exec("INSERT INTO log VALUES (?)", r.K)
	}
}`,
			wantN: 2,
		},
		{
			name: "db.Exec outside loop does not trigger",
			src: `package p
func f(db DB) {
	db.Exec("DELETE FROM t")
}`,
			wantN: 0,
		},
		{
			name: "unknown receiver in loop does not trigger",
			src: `package p
func f(conn Conn, ids []int) {
	for _, id := range ids {
		conn.Execute("DELETE FROM t WHERE id = ?", id)
	}
}`,
			wantN: 0,
		},
		{
			name: "known receiver but unknown method does not trigger",
			src: `package p
func f(db DB, ids []int) {
	for _, id := range ids {
		db.Ping()
	}
}`,
			wantN: 0,
		},
		{
			name: "function call (not method) in loop does not trigger",
			src: `package p
func f(ids []int) {
	for _, id := range ids {
		doSomething(id)
	}
}`,
			wantN: 0,
		},
		{
			name: "gorm.Create in range loop triggers",
			src: `package p
func f(gorm ORM, users []User) {
	for _, u := range users {
		gorm.Create(&u)
	}
}`,
			wantN: 1,
		},
	}

	c := &BatchingChecker{}
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
					t.Logf("  issue: %s", iss.Message)
				}
			}
			if tt.wantMsg != "" && len(issues) > 0 {
				if !contains(issues[0].Message, tt.wantMsg) {
					t.Errorf("message %q does not contain %q", issues[0].Message, tt.wantMsg)
				}
			}
		})
	}
}

func TestBatchingChecker_IssueFields(t *testing.T) {
	src := `package p
func f(db DB, rows []Row) {
	for _, r := range rows {
		db.Exec("INSERT INTO t VALUES (?)", r.V)
	}
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := &BatchingChecker{}
	issues := c.Check(fset, f)
	if len(issues) != 1 {
		t.Fatalf("got %d issue(s), want 1", len(issues))
	}
	iss := issues[0]
	if iss.Checker != "Batching" {
		t.Errorf("Checker = %q, want %q", iss.Checker, "Batching")
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
	if iss.Suggestion == "" {
		t.Error("Suggestion should be non-empty")
	}
	if iss.Benchmark == "" {
		t.Error("Benchmark should be non-empty")
	}
}
