package benchmarks_test

import (
	"bufio"
	"os"
	"testing"
)

const lineCount = 200
const lineData = "hello world this is a benchmark line of text\n"

func BenchmarkUnbufferedWrite(b *testing.B) {
	b.ReportAllocs()
	f, err := os.OpenFile("/dev/null", os.O_WRONLY, 0)
	if err != nil {
		b.Skip("cannot open /dev/null:", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			b.Error(err)
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < lineCount; j++ {
			if _, err := f.WriteString(lineData); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkBufferedWrite(b *testing.B) {
	b.ReportAllocs()
	f, err := os.OpenFile("/dev/null", os.O_WRONLY, 0)
	if err != nil {
		b.Skip("cannot open /dev/null:", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			b.Error(err)
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bw := bufio.NewWriter(f)
		for j := 0; j < lineCount; j++ {
			if _, err := bw.WriteString(lineData); err != nil {
				b.Fatal(err)
			}
		}
		if err := bw.Flush(); err != nil {
			b.Fatal(err)
		}
	}
}
