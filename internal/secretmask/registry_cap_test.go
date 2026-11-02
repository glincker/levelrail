package secretmask_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestCapCountsPrimariesOnlyAndKeepsEarlierOnes(t *testing.T) {
	t.Setenv("APP_SECRETMASK_MAX_VALUES", "50")
	r := secretmask.New()
	for i := 0; i < 80; i++ {
		r.Add(fmt.Sprintf("primary-secret-%04d", i))
	}
	if r.Len() != 50 {
		t.Fatalf("Len = %d, want 50 primaries (encoded forms must not count)", r.Len())
	}
	first := "primary-secret-0000"
	if strings.Contains(r.Mask("x "+first), first) {
		t.Fatal("an early primary was evicted or never masked")
	}
	if strings.Contains(r.Mask("x cHJpbWFyeS1zZWNyZXQtMDAwMA=="), "cHJpbWFyeS1zZWNyZXQtMDAwMA") {
		t.Fatal("encoded form of an early primary is not masked")
	}
}

func TestAddAndMaskConcurrently(t *testing.T) {
	r := secretmask.New()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				r.Add(fmt.Sprintf("goroutine-%d-secret-%04d", g, i))
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = r.Mask("line goroutine-1-secret-0001 end")
			}
		}()
	}
	wg.Wait()
	if strings.Contains(r.Mask("goroutine-3-secret-0199"), "goroutine-3") {
		t.Fatal("value added concurrently is not masked afterwards")
	}
}
