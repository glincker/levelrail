package appsleep

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestWakeConcurrentStartsOnce(t *testing.T) {
	sleep := &fakeSleep{rows: map[string]*store.AppSleep{"web": {ServiceName: "web", IdleMinutes: 30, Sleeping: true}}}
	apps := &fakeApps{suspended: map[string]bool{"web": true}}
	var woken atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, err := Wake(context.Background(), sleep, apps, nil, "web", time.Now()); err == nil && ok {
				woken.Add(1)
			}
		}()
	}
	wg.Wait()
	if woken.Load() != 1 {
		t.Fatalf("wake started %d times, want 1", woken.Load())
	}
}

func TestWaiterGateBounds(t *testing.T) {
	var g WaiterGate
	r1, ok1 := g.Acquire("web", 2)
	_, ok2 := g.Acquire("web", 2)
	_, ok3 := g.Acquire("web", 2)
	if !ok1 || !ok2 || ok3 {
		t.Fatalf("got %v %v %v, want true true false", ok1, ok2, ok3)
	}
	r1()
	if _, ok := g.Acquire("web", 2); !ok {
		t.Fatal("a released slot must be reusable")
	}
	if _, ok := g.Acquire("other", 2); !ok {
		t.Fatal("limits are per app")
	}
}
