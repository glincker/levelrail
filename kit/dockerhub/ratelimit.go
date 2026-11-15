package dockerhub

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrRateLimited is matched by errors.Is for any 429 from Docker Hub or its
// registry, including the registry's "toomanyrequests" answer.
var ErrRateLimited = errors.New("dockerhub: rate limited")

// RateLimitedError carries the Retry-After wait when Docker Hub sent one.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("dockerhub: rate limited (toomanyrequests), retry after %s", e.RetryAfter)
	}
	return "dockerhub: rate limited (toomanyrequests)"
}

// Is makes errors.Is(err, ErrRateLimited) true.
func (e *RateLimitedError) Is(target error) bool { return target == ErrRateLimited }

// parseRetryAfter reads delta seconds or an HTTP date. Zero means absent.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

// PullLimits is the pull quota Docker reports in registry response headers.
type PullLimits struct {
	// Known is false when the registry sent no limit headers, as for plans
	// without a pull limit.
	Known         bool   `json:"known"`
	Limit         int    `json:"limit"`
	Remaining     int    `json:"remaining"`
	WindowSeconds int    `json:"window_seconds"`
	Source        string `json:"source"`
	// Authenticated is true when the preview request carried a token.
	Authenticated bool `json:"authenticated"`
}

// ParseRateLimit reads ratelimit-limit, ratelimit-remaining and
// docker-ratelimit-source, for example "100;w=21600".
func ParseRateLimit(h http.Header) PullLimits {
	limit, window, okL := parseQuota(h.Get("RateLimit-Limit"))
	remaining, _, okR := parseQuota(h.Get("RateLimit-Remaining"))
	return PullLimits{
		Known: okL && okR, Limit: limit, Remaining: remaining, WindowSeconds: window,
		Source: h.Get("Docker-RateLimit-Source"),
	}
}

func parseQuota(v string) (n, window int, ok bool) {
	head, params, _ := strings.Cut(strings.TrimSpace(v), ";")
	n, err := strconv.Atoi(strings.TrimSpace(head))
	if err != nil {
		return 0, 0, false
	}
	for _, p := range strings.Split(params, ";") {
		if w, found := strings.CutPrefix(strings.TrimSpace(p), "w="); found {
			window, _ = strconv.Atoi(w)
		}
	}
	return n, window, true
}

// PullLimits reads the current pull quota with a HEAD request on the
// documented preview repository, which does not count as a pull. A 429
// that still carries limit headers is reported as remaining zero.
func (c *Client) PullLimits(ctx context.Context, cred Credentials) (PullLimits, error) {
	tok, err := c.registryToken(ctx, cred, "repository:"+previewRepo+":pull")
	if err != nil {
		return PullLimits{}, err
	}
	hdr, err := c.headManifest(ctx, tok.Token, previewRepo, "latest")
	lim := ParseRateLimit(hdr)
	lim.Authenticated = cred.Present()
	if err != nil {
		if errors.Is(err, ErrRateLimited) && lim.Known {
			return lim, nil
		}
		return PullLimits{}, fmt.Errorf("dockerhub: pull limits: %w", err)
	}
	return lim, nil
}

// CachedPullLimits is PullLimits with an in-memory cache. checkedAt is when
// the answer was read from Docker.
func (c *Client) CachedPullLimits(ctx context.Context, cred Credentials, ttl time.Duration, refresh bool) (lim PullLimits, checkedAt time.Time, cached bool, err error) {
	key := "limits|" + cred.cacheKey()
	if !refresh {
		if v, at, ok := c.cacheLoadAt(key); ok {
			return v.(PullLimits), at, true, nil
		}
	}
	lim, err = c.PullLimits(ctx, cred)
	if err != nil {
		return PullLimits{}, time.Time{}, false, err
	}
	if ttl > 0 {
		c.cacheStore(key, lim, ttl)
	}
	return lim, time.Now(), false, nil
}

func (c *Client) cacheLoadAt(key string) (any, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || time.Now().After(e.expires) {
		return nil, time.Time{}, false
	}
	return e.value, e.stored, true
}
