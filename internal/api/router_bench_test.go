package api

import "testing"

// BenchmarkNewTestRouter tracks the per-test fixture cost of building a router.
func BenchmarkNewTestRouter(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		newTestRouter(b)
	}
}
