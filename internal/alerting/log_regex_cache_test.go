package alerting

import (
	"fmt"
	"sync"
	"testing"
)

func TestCompileLogRegex_InvalidPatternNotCached(t *testing.T) {
	const bad = `timeout (after`
	for i := 0; i < 2; i++ {
		if _, err := compileLogRegex(bad); err == nil {
			t.Fatalf("attempt %d: expected compile error for %q", i, bad)
		}
	}
	logRegexes.mu.Lock()
	_, cached := logRegexes.m[bad]
	logRegexes.mu.Unlock()
	if cached {
		t.Fatal("invalid pattern must not be cached")
	}
}

func TestCompileLogRegex_ReusesCompiledPattern(t *testing.T) {
	const pattern = `req=\d+`
	first, err := compileLogRegex(pattern)
	if err != nil {
		t.Fatal(err)
	}
	second, err := compileLogRegex(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("expected the cached *regexp.Regexp to be returned for the same pattern")
	}
}

func TestCompileLogRegex_EvictionKeepsCacheBounded(t *testing.T) {
	for i := 0; i < maxCachedLogRegexes+50; i++ {
		if _, err := compileLogRegex(fmt.Sprintf(`evict-%d\d`, i)); err != nil {
			t.Fatal(err)
		}
	}
	logRegexes.mu.Lock()
	size := len(logRegexes.m)
	logRegexes.mu.Unlock()
	if size > maxCachedLogRegexes {
		t.Fatalf("cache size %d exceeds bound %d", size, maxCachedLogRegexes)
	}
	if _, err := compileLogRegex(`evict-0\d`); err != nil {
		t.Fatalf("evicted pattern must still compile: %v", err)
	}
}

func TestCompileLogRegex_ConcurrentUse(t *testing.T) {
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				if _, err := compileLogRegex(fmt.Sprintf(`conc-%d\d`, (g+i)%32)); err != nil {
					t.Error(err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}
