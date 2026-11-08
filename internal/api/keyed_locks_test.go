package api

import (
	"sync"
	"testing"
)

func TestKeyedLocksSerializeAndRelease(t *testing.T) {
	var k keyedLocks
	var wg sync.WaitGroup
	counter := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer k.lock("a-pr-1")()
			counter++
		}()
	}
	wg.Wait()
	if counter != 20 {
		t.Fatalf("counter = %d, want 20", counter)
	}
	if len(k.m) != 0 {
		t.Fatalf("entries left = %d, want 0", len(k.m))
	}
}
