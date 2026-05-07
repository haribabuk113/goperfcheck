# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.1.x   | ✅        |

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report security issues by emailing **haribabuk113@gmail.com** with the subject line:

```
[goperfcheck] Security Vulnerability Report
```

Include in your report:
- A description of the vulnerability and its potential impact
- Steps to reproduce or a proof-of-concept
- Any suggested fix, if you have one

You can expect an acknowledgement within **48 hours** and a resolution or status update within **7 days**.

## Scope

`goperfcheck` is a static analysis CLI tool that reads Go source files from the local filesystem. It has no network access, no server component, and no persistent state beyond writing an optional Markdown report.

The primary security concern is **path traversal or unexpected file access** when running the tool against untrusted codebases. If you discover a way to make `goperfcheck` read, write, or execute files outside the scanned directory, please report it.

## Out of Scope

- Vulnerabilities in Go's standard library (report to the [Go security team](https://go.dev/security))
- Issues in static analysis output (false positives / false negatives are bugs, not security issues — open a regular GitHub issue)
