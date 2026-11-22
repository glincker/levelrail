package build

import (
	"context"
	"testing"
	"time"
)

func TestFairShareAdmitsTeamWithFewestRunningFirst(t *testing.T) {
	l := NewLimiter(nil, fixed(Limits{}))
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := l.acquire(ctx, "n", 2, "a"); err != nil {
			t.Fatal(err)
		}
	}
	order := make(chan string, 2)
	wait := func(key string) {
		go func() {
			if err := l.acquire(ctx, "n", 2, key); err == nil {
				order <- key
			}
		}()
	}
	wait("a")
	waitForWaiters(t, l, 1)
	wait("b")
	waitForWaiters(t, l, 2)

	l.release("n", "a")
	select {
	case got := <-order:
		if got != "b" {
			t.Fatalf("admitted %q first, want b (team a already runs a build)", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no waiter admitted")
	}
}

func waitForWaiters(t *testing.T, l *Limiter, n int) {
	t.Helper()
	for i := 0; i < 200; i++ {
		l.mu.Lock()
		got := len(l.slotsFor("n").waiters)
		l.mu.Unlock()
		if got == n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("waiters never reached %d", n)
}
