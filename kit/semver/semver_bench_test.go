package semver

import "testing"

func BenchmarkCompare(b *testing.B) {
	pairs := [][2]string{
		{"v1.2.3", "v1.10.0"},
		{"1.2.3-rc.1", "1.2.3"},
		{"v2.0.0", "v2.0.0"},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p := pairs[i%len(pairs)]
		_, _ = Compare(p[0], p[1])
	}
}
