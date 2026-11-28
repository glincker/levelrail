package ingress

import (
	"context"
	"sync"
	"testing"
)

// A reset racing an Apply must never leave Caddy on a stale config while
// lastApplied claims the newer payload.
func TestDriver_ConcurrentApplyAndResetKeepServedConfigCurrent(t *testing.T) {
	orig := loadConfig
	t.Cleanup(func() { loadConfig = orig })
	var mu sync.Mutex
	var served []byte
	loadConfig = func(b []byte, _ bool) error {
		mu.Lock()
		served = append([]byte(nil), b...)
		mu.Unlock()
		return nil
	}
	d := New(nil)
	ctx := context.Background()
	if err := d.Apply(ctx, acmeTLSConfig("")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		cfg := acmeTLSConfig("https://ca.example.org/dir" + string(rune('a'+i%26)))
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = d.Apply(ctx, cfg) }()
		go func() { defer wg.Done(); _ = d.ResetCertCache(ctx) }()
		wg.Wait()
		d.mu.Lock()
		last := d.lastApplied
		d.mu.Unlock()
		mu.Lock()
		got := append([]byte(nil), served...)
		mu.Unlock()
		if last != nil && string(last) != string(got) {
			t.Fatalf("iteration %d: lastApplied differs from the served config", i)
		}
	}
}

func TestDriver_ResetRetriesReloadOnce(t *testing.T) {
	orig := loadConfig
	t.Cleanup(func() { loadConfig = orig })
	loadConfig = func([]byte, bool) error { return nil }
	d := New(nil)
	if err := d.Apply(context.Background(), acmeTLSConfig("")); err != nil {
		t.Fatal(err)
	}
	calls := 0
	loadConfig = func([]byte, bool) error {
		calls++
		if calls == 2 {
			return context.DeadlineExceeded
		}
		return nil
	}
	if err := d.ResetCertCache(context.Background()); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if calls != 3 {
		t.Fatalf("loads = %d, want empty plus failed plus retried", calls)
	}
}
