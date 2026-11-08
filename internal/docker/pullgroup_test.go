package docker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPullGroupDedupesSameRef(t *testing.T) {
	g := newPullGroup()
	var calls atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = g.do(context.Background(), "img:1", func() error {
				calls.Add(1)
				<-release
				return nil
			})
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Errorf("pull ran %d times, want 1", n)
	}
}

func TestPullGroupCapsConcurrency(t *testing.T) {
	SetPullConcurrency(2)
	t.Cleanup(func() { SetPullConcurrency(0) })
	g := newPullGroup()
	var cur, peak atomic.Int32
	var wg sync.WaitGroup
	for _, k := range []string{"a", "b", "c", "d", "e"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = g.do(context.Background(), k, func() error {
				n := cur.Add(1)
				for {
					p := peak.Load()
					if n <= p || peak.CompareAndSwap(p, n) {
						break
					}
				}
				time.Sleep(30 * time.Millisecond)
				cur.Add(-1)
				return nil
			})
		}()
	}
	wg.Wait()
	if p := peak.Load(); p > 2 {
		t.Errorf("peak concurrent pulls = %d, want <= 2", p)
	}
}

func TestPullGroupKeySeparatesCredentials(t *testing.T) {
	if pullKey("r/i:1", nil) == pullKey("r/i:1", &RegistryAuth{Username: "u", Password: "p"}) {
		t.Error("anonymous and authenticated pulls share a key")
	}
}
