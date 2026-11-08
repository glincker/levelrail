package docker

import (
	"context"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/singleflight"
)

// EnvPullConcurrency seeds the images.pull_concurrency runtime setting.
const EnvPullConcurrency = "APP_IMAGES_PULL_CONCURRENCY"

// defaultPullConcurrency is the seed when neither a setting nor the env names a cap.
const defaultPullConcurrency = 3

var pullConcurrency atomic.Int64

// SetPullConcurrency applies the live images.pull_concurrency value; n < 1 falls back to the seed.
func SetPullConcurrency(n int) { pullConcurrency.Store(int64(n)) }

func currentPullConcurrency() int {
	if n := int(pullConcurrency.Load()); n > 0 {
		return n
	}
	if v, err := strconv.Atoi(os.Getenv(EnvPullConcurrency)); err == nil && v > 0 {
		return v
	}
	return defaultPullConcurrency
}

// pullGroup collapses concurrent pulls of the same ref and credentials into
// one and caps how many distinct pulls run at once.
type pullGroup struct {
	flight singleflight.Group
	mu     sync.Mutex
	active int
	cond   *sync.Cond
}

func newPullGroup() *pullGroup {
	g := &pullGroup{}
	g.cond = sync.NewCond(&g.mu)
	return g
}

// do runs fn once per key among overlapping callers. Callers that join an
// in-flight pull share its result and do not receive its progress events.
func (g *pullGroup) do(ctx context.Context, key string, fn func() error) error {
	ch := g.flight.DoChan(key, func() (any, error) {
		g.acquire()
		defer g.release()
		return nil, fn()
	})
	select {
	case res := <-ch:
		return res.Err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *pullGroup) acquire() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for g.active >= currentPullConcurrency() {
		g.cond.Wait()
	}
	g.active++
}

func (g *pullGroup) release() {
	g.mu.Lock()
	g.active--
	g.mu.Unlock()
	g.cond.Broadcast()
}

// pullKey separates credentials so a private pull is never served to a caller without access.
func pullKey(ref string, auth *RegistryAuth) string {
	if auth == nil {
		return ref + "\x00"
	}
	return ref + "\x00" + auth.Username + "\x00" + auth.Password
}
