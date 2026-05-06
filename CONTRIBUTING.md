# Contributing to goperfcheck

Thank you for taking the time to contribute!

## Getting started

```bash
git clone https://github.com/haribabuk113/goperfcheck.git
cd goperfcheck
go build ./...
go test ./...
```

## Reporting bugs

Open a [GitHub issue](https://github.com/haribabuk113/goperfcheck/issues) and include:
- Go version (`go version`)
- The file or snippet that triggered the problem
- The actual output vs what you expected

## Adding a new checker

Each checker lives in `checker/` as its own file.

1. Create `checker/my_rule.go` and implement the `Checker` interface:

```go
type MyRuleChecker struct{}

func (c *MyRuleChecker) Name() string { return "MyRule" }

func (c *MyRuleChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
    var issues []Issue
    // AST inspection logic here
    return issues
}
```

2. Register it in `checker/registry.go` inside `AllCheckers()`.

3. Add table-driven tests in `checker/my_rule_test.go` (see existing `*_test.go` files for the pattern).

4. Document the rule in `README.md` under the relevant category.

## Code style

- Run `go vet ./...` and `go fmt ./...` before submitting.
- No external dependencies — the tool must rely only on the Go standard library.
- Checkers must be purely AST-based (no `go/types` or full type-checking).
- Keep each checker focused on a single rule.

## Pull requests

- One logical change per PR.
- Include a test that fails before your change and passes after.
- Update `README.md` if you add or change a checker.
- Squash fixup commits before merging.
