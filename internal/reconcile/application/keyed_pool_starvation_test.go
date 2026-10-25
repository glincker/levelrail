package application

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func probedService(name string, readyTimeout time.Duration) *store.DesiredService {
	return &store.DesiredService{
		Name: name, Image: "img:v1", Port: 80,
		Health: &store.ServiceHealth{
			Readiness:    &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond},
			ReadyTimeout: readyTimeout,
		},
	}
}

// One app whose readiness never passes must not keep 20 healthy apps, including
// ones created while it waits, from converging once more than one worker runs.
func TestKeyedPool_NeverReadyAppDoesNotStarveHealthyApps(t *testing.T) {
	ok := alwaysHealthy()
	defer ok.Close()
	bad := neverHealthy()
	defer bad.Close()

	for _, workers := range []int{2, 4} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			ctrls := []reconcile.Controller{
				New("bad", &fakeStore{svc: probedService("bad", time.Minute)}, newFakeRuntime(serverPort(t, bad)), WithReadyBudget(time.Minute)),
			}
			names := make([]string, 20)
			for i := range names {
				names[i] = fmt.Sprintf("h%02d", i)
				ctrls = append(ctrls, New(names[i], &fakeStore{svc: probedService(names[i], 0)}, newFakeRuntime(serverPort(t, ok)), WithReadyBudget(5*time.Second)))
			}
			var visible atomic.Int32
			visible.Store(11)

			e := reconcile.NewEngine(nil)
			e.SetWorkers(workers, time.Hour)
			e.SetKeyed(IsServiceControllerName)
			e.SetSource(func(context.Context) ([]reconcile.Controller, error) {
				return ctrls[:visible.Load()], nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- e.Run(ctx, nil, 50*time.Millisecond) }()
			defer func() {
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("Run did not return after cancel")
				}
			}()

			converged := func(name string) bool {
				res, err := e.LastResult(ControllerName(name))
				return err == nil && len(res.Conditions) > 0 && res.Conditions[0].Status == reconcile.ConditionTrue
			}
			waitUntil := func(bound time.Duration, what string, cond func() bool) {
				t.Helper()
				deadline := time.Now().Add(bound)
				for !cond() {
					if time.Now().After(deadline) {
						t.Fatalf("not within %v: %s", bound, what)
					}
					if s := e.Stats().Queue; s != nil && s.InFlight > workers {
						t.Fatalf("in flight %d exceeds %d workers", s.InFlight, workers)
					}
					time.Sleep(5 * time.Millisecond)
				}
			}

			waitUntil(10*time.Second, "first ten healthy apps converged", func() bool {
				for _, n := range names[:10] {
					if !converged(n) {
						return false
					}
				}
				return true
			})
			visible.Store(21)
			e.Nudge()
			waitUntil(10*time.Second, "apps created while bad waits converged", func() bool {
				for _, n := range names {
					if !converged(n) {
						return false
					}
				}
				return true
			})
			if converged("bad") {
				t.Fatal("never-ready app reported converged")
			}
		})
	}
}
