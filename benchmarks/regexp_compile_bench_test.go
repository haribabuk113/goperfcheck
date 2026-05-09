package benchmarks

import (
	"regexp"
	"testing"
)

var precompiledRe = regexp.MustCompile(`\d+`)

func matchWithCompile(s string) bool {
	re := regexp.MustCompile(`\d+`)
	return re.MatchString(s)
}

func matchPrecompiled(s string) bool {
	return precompiledRe.MatchString(s)
}

func BenchmarkRegexpCompilePerCall(b *testing.B) {
	for b.Loop() {
		matchWithCompile("hello123world")
	}
}

func BenchmarkRegexpPrecompiled(b *testing.B) {
	for b.Loop() {
		matchPrecompiled("hello123world")
	}
}
