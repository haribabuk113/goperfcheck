package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestInterfaceBoxingChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "slice of interface{} triggers",
			src: `package p
func f(args []interface{}) {}`,
			wantN: 1,
		},
		{
			name: "function parameter of interface{} triggers",
			src: `package p
func f(v interface{}) {}`,
			wantN: 1,
		},
		{
			name: "map with interface{} value triggers",
			src: `package p
func f() map[string]interface{} { return nil }`,
			wantN: 1,
		},
		{
			name: "typed slice does not trigger",
			src: `package p
func f(args []string) {}`,
			wantN: 0,
		},
		{
			name: "typed function parameter does not trigger",
			src: `package p
func f(v int) {}`,
			wantN: 0,
		},
	}

	c := &InterfaceBoxingChecker{}
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
		})
	}
}

// TestInterfaceBoxingFalsePositive_UnavoidableBoxing documents three known
// false positive categories where InterfaceBoxing fires but the interface{}
// usage is correct and unavoidable.
//
// Because AST-only analysis cannot resolve which package a type originates from
// or how a function is called, the checker fires on all []interface{} and
// interface{} parameter patterns regardless of context.
func TestInterfaceBoxingFalsePositive_UnavoidableBoxing(t *testing.T) {
	cases := []struct {
		name        string
		description string
		src         string
	}{
		{
			name: "fmt-style variadic wrapper",
			description: "A thin wrapper around fmt.Fprintf must accept ...interface{} " +
				"because the underlying function requires it. There is no way to " +
				"eliminate the boxing without changing the API contract.",
			src: `package p
func logf(format string, args []interface{}) {
	// wraps fmt.Fprintf — interface{} is required by the fmt package
}`,
		},
		{
			name: "json-marshaling helper",
			description: "json.Marshal already uses reflection internally. A helper that " +
				"accepts interface{} is not adding boxing overhead beyond what json.Marshal " +
				"already requires.",
			src: `package p
func marshal(v interface{}) ([]byte, error) {
	// passes v to json.Marshal — reflection already boxes the value
	return nil, nil
}`,
		},
		{
			name: "intentional heterogeneous container",
			description: "[]interface{} is the correct and idiomatic type when a container " +
				"must hold values of genuinely different types (e.g. a scripting engine's " +
				"value stack, an AST node list with mixed node kinds).",
			src: `package p
// Values holds a mixed-type runtime value stack.
type Values struct {
	stack []interface{}
}`,
		},
	}

	c := &InterfaceBoxingChecker{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tc.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)

			// The checker fires on all three — these are documented false positives.
			// AST-only analysis cannot distinguish these correct uses from genuinely
			// optimisable cases. Verify the context before acting on this suggestion.
			if len(issues) == 0 {
				t.Fatalf("%s: expected InterfaceBoxing to fire (known false positive: %s)",
					tc.name, tc.description)
			}
		})
	}
}
