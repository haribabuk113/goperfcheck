package benchmarks

import (
	"strings"
	"testing"
)

func buildStringConcat(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += "hello"
	}
	return s
}

func buildStringBuilder(n int) string {
	var b strings.Builder
	b.Grow(n * 5)
	for i := 0; i < n; i++ {
		b.WriteString("hello")
	}
	return b.String()
}

func BenchmarkStringConcatLoop(b *testing.B) {
	for b.Loop() {
		buildStringConcat(100)
	}
}

func BenchmarkStringsBuilder(b *testing.B) {
	for b.Loop() {
		buildStringBuilder(100)
	}
}
