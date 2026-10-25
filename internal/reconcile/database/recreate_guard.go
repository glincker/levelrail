package database

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

// Env knobs for the recreate loop guard. A replace beyond the limit inside the
// window is delayed with an exponential backoff instead of repeating forever.
const (
	EnvRecreateLimit       = "APP_DB_RECREATE_LIMIT"
	EnvRecreateWindow      = "APP_DB_RECREATE_WINDOW"
	EnvRecreateBackoffBase = "APP_DB_RECREATE_BACKOFF_BASE"
	EnvRecreateBackoffMax  = "APP_DB_RECREATE_BACKOFF_MAX"

	defaultRecreateLimit       = 5
	defaultRecreateWindow      = 10 * time.Minute
	defaultRecreateBackoffBase = 30 * time.Second
	defaultRecreateBackoffMax  = 15 * time.Minute

	reasonRecreateLoop = "RecreateLoop"
)

// recreateLoopError means a replace was held back by the loop guard.
type recreateLoopError struct {
	count int
	wait  time.Duration
}

func (e *recreateLoopError) Error() string {
	return fmt.Sprintf("this database was recreated %d times in the last window, so further recreates are delayed (next attempt in %s); "+
		"the container keeps differing from desired state after every create, check `podman inspect` against the spec", e.count, e.wait.Round(time.Second))
}

// recreateGuard remembers recent replaces of one database container.
type recreateGuard struct {
	mu    sync.Mutex
	times []time.Time
}

func intFromEnv(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}

// allow records a replace at now, or returns the loop error when the guard
// says to wait. Each replace past the limit doubles the wait, up to the max.
func (g *recreateGuard) allow(now time.Time) error {
	limit := intFromEnv(EnvRecreateLimit, defaultRecreateLimit)
	window := durationFromEnv(EnvRecreateWindow, defaultRecreateWindow)
	base := durationFromEnv(EnvRecreateBackoffBase, defaultRecreateBackoffBase)
	maxWait := durationFromEnv(EnvRecreateBackoffMax, defaultRecreateBackoffMax)

	g.mu.Lock()
	defer g.mu.Unlock()
	kept := g.times[:0]
	for _, t := range g.times {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	g.times = kept
	if len(g.times) >= limit {
		wait := base
		for i := limit; i < len(g.times) && wait < maxWait; i++ {
			wait *= 2
		}
		if wait > maxWait {
			wait = maxWait
		}
		if left := wait - now.Sub(g.times[len(g.times)-1]); left > 0 {
			return &recreateLoopError{count: len(g.times), wait: left}
		}
	}
	g.times = append(g.times, now)
	return nil
}

// sameImageRef compares an image reference as the runtime reports it with the
// one the spec asked for. Podman reports docker.io/library/postgres:16 for a
// container created from postgres:16.
func sameImageRef(observed, desired string) bool {
	return canonicalImageRef(observed) == canonicalImageRef(desired)
}

func canonicalImageRef(ref string) string {
	ref = strings.TrimSpace(ref)
	for _, host := range []string{"docker.io/", "index.docker.io/", "registry-1.docker.io/"} {
		if rest, ok := strings.CutPrefix(ref, host); ok {
			ref = rest
			break
		}
	}
	if rest, ok := strings.CutPrefix(ref, "library/"); ok && !strings.Contains(rest, "/") {
		ref = rest
	}
	return ref
}

// replaceFailure maps a replace error to the condition result; a loop guard
// hold is a distinct reason with a requeue instead of a generic failure.
func replaceFailure(dbName string, err error) (reconcile.Result, error) {
	var loop *recreateLoopError
	if errors.As(err, &loop) {
		res := notReady(reasonRecreateLoop, loop)
		res.RequeueAfter = loop.wait
		return res, nil
	}
	return notReady("ReplaceFailed", err), fmt.Errorf("database/%s: %w", dbName, err)
}

func (c *Controller) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}
