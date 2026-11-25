package reconcile

import (
	"testing"
	"time"
)

func TestControllerCosts_TopOrdersByTotal(t *testing.T) {
	var c costTable
	c.add("a", time.Millisecond, false)
	c.add("b", 5*time.Millisecond, true)
	c.add("b", 2*time.Millisecond, false)
	top := c.top(1)
	if len(top) != 1 || top[0].Name != "b" || top[0].Runs != 2 || top[0].Errors != 1 || top[0].Max != 5*time.Millisecond {
		t.Fatalf("top = %+v", top)
	}
}

func TestRefreshResync_AppliesLiveValue(t *testing.T) {
	e := &Engine{}
	tk := time.NewTicker(time.Hour)
	defer tk.Stop()
	if got := e.refreshResync(tk, 30*time.Second); got != 30*time.Second {
		t.Fatalf("no source = %v", got)
	}
	next := 10 * time.Second
	e.SetResyncSource(func() time.Duration { return next })
	if got := e.refreshResync(tk, 30*time.Second); got != 10*time.Second {
		t.Fatalf("changed = %v", got)
	}
	next = 0
	if got := e.refreshResync(tk, 10*time.Second); got != 10*time.Second {
		t.Fatalf("zero keeps = %v", got)
	}
}

func TestRefreshResync_ResetsTickerToLiveInterval(t *testing.T) {
	e := &Engine{}
	e.SetResyncSource(func() time.Duration { return 10 * time.Millisecond })
	tk := time.NewTicker(time.Hour)
	defer tk.Stop()
	e.refreshResync(tk, time.Hour)
	select {
	case <-tk.C:
	case <-time.After(time.Second):
		t.Fatal("ticker did not fire at the new interval")
	}
}
