package compose

import (
	"fmt"
	"testing"
)

func BenchmarkJoinErrors(b *testing.B) {
	errs := make([]error, 100)
	for i := 0; i < 100; i++ {
		errs[i] = fmt.Errorf("validation error %d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = joinErrors(errs)
	}
}
