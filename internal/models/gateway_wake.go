package models

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	wakePollInterval  = 500 * time.Millisecond
	wakeNudgeInterval = 2 * time.Second
	touchWriteTimeout = 5 * time.Second
)

// WakeStore is the optional store surface that lets the gateway wake and
// keep alive on-demand models. *store.DB satisfies it; a store without it
// leaves on-demand models unreachable while asleep.
type WakeStore interface {
	GetModel(ctx context.Context, name string) (*store.Model, error)
	TouchModel(ctx context.Context, name string, at time.Time) error
}

// SetWakeHook registers a function called (repeatedly, cheaply) while a
// request waits for a model to wake, typically the reconciler's Nudge.
func (g *Gateway) SetWakeHook(fn func()) { g.wakeHook = fn }

func (g *Gateway) touch(name string, force bool) {
	if g.wakeStore == nil {
		return
	}
	now := time.Now()
	g.touchMu.Lock()
	every := g.limits.TouchInterval
	if force {
		every = time.Second
	}
	if last, ok := g.lastTouch[name]; ok && now.Sub(last) < every {
		g.touchMu.Unlock()
		return
	}
	g.lastTouch[name] = now
	g.touchMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), touchWriteTimeout)
	defer cancel()
	if err := g.wakeStore.TouchModel(ctx, name, now.UTC()); err != nil {
		g.logger.Warn("models: gateway touch failed", slog.String("model", name), slog.String("error", err.Error()))
	}
}

// holdActive marks the model in use until the returned function runs, and
// keeps refreshing it while a long request streams so the engine is not
// stopped under it.
func (g *Gateway) holdActive(name string) func() {
	g.touch(name, false)
	if g.wakeStore == nil {
		return func() {}
	}
	done := make(chan struct{})
	every := max(g.limits.TouchInterval, time.Second)
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				g.touch(name, false)
			}
		}
	}()
	return func() {
		close(done)
		g.touch(name, false)
	}
}

func awakeAndReachable(m store.Model) bool {
	return m.Residency != store.ResidencyOnDemand || (m.ResidencyState == store.ResidencyAwake && m.EndpointDial != "")
}

// ensureAwake returns the model ready to proxy to, holding the request
// while a sleeping engine loads. It writes the error response itself and
// returns false when the model is not ready within the wait.
func (g *Gateway) ensureAwake(ctx context.Context, w *statusWriter, m store.Model) (store.Model, bool) {
	if g.wakeStore == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "model_not_ready", "model is not running yet")
		return m, false
	}
	if fresh, err := g.wakeStore.GetModel(ctx, m.Name); err == nil {
		m = *fresh
	}
	if awakeAndReachable(m) {
		return m, true
	}
	if fresh, ok := g.wake(ctx, m); ok {
		return fresh, true
	}
	if ctx.Err() != nil {
		return m, false
	}
	w.Header().Set("Retry-After", strconv.Itoa(max(int(math.Ceil(g.limits.WakeRetryAfter.Seconds())), 1)))
	writeOpenAIError(w, http.StatusServiceUnavailable, "model_waking", "the model is starting; retry shortly")
	return m, false
}

func (g *Gateway) nudge() {
	if g.wakeHook != nil {
		g.wakeHook()
	}
}

// wake asks for the engine to start, then polls until it is serving or the
// bounded wait ends.
func (g *Gateway) wake(ctx context.Context, m store.Model) (store.Model, bool) {
	g.touch(m.Name, true)
	g.nudge()
	if g.limits.WakeWait <= 0 {
		return m, false
	}
	deadline := time.NewTimer(g.limits.WakeWait)
	defer deadline.Stop()
	poll := time.NewTicker(wakePollInterval)
	defer poll.Stop()
	lastNudge := time.Now()
	for {
		select {
		case <-ctx.Done():
			return m, false
		case <-deadline.C:
			return m, false
		case <-poll.C:
		}
		fresh, err := g.wakeStore.GetModel(ctx, m.Name)
		if err != nil || fresh.Deleting {
			continue
		}
		if awakeAndReachable(*fresh) {
			return *fresh, true
		}
		if time.Since(lastNudge) >= wakeNudgeInterval {
			lastNudge = time.Now()
			g.nudge()
		}
	}
}
