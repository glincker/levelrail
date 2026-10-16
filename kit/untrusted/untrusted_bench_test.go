package untrusted

import (
	"strings"
	"testing"
)

var benchPlainLine = strings.Repeat("2026-10-16T12:00:00Z INFO request served path=/api/v1/apps status=200 duration=12ms\n", 4)

func BenchmarkClean(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Clean(benchPlainLine)
	}
}

func BenchmarkSanitize(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Sanitize(benchPlainLine, 4096)
	}
}
