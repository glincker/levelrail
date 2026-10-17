package sourcecache

import (
	"context"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
)

// HostStatus is one git host that is currently failing.
type HostStatus struct {
	Host        string
	RepoURL     string
	Reason      string
	Since       time.Time
	LastFailure time.Time
	Failures    int
	Blocked     []string
}

type hostEntry struct {
	HostStatus
	blocked map[string]struct{}
}

type health struct {
	mu    sync.Mutex
	hosts map[string]*hostEntry
}

func newHealth() *health { return &health{hosts: map[string]*hostEntry{}} }

func hostOf(repoURL string) string {
	if u, err := url.Parse(repoURL); err == nil && u.Host != "" {
		return u.Hostname()
	}
	return repoURL
}

func (h *health) markFailed(repoURL string, at time.Time, reason string) {
	host := hostOf(repoURL)
	h.mu.Lock()
	defer h.mu.Unlock()
	e := h.hosts[host]
	if e == nil {
		e = &hostEntry{HostStatus: HostStatus{Host: host, Since: at}, blocked: map[string]struct{}{}}
		h.hosts[host] = e
	}
	e.RepoURL, e.Reason, e.LastFailure = cleanURL(repoURL), oneLine(reason), at
	e.Failures++
}

func (h *health) markOK(repoURL string) {
	host := hostOf(repoURL)
	h.mu.Lock()
	delete(h.hosts, host)
	h.mu.Unlock()
}

// Block records that key (an app name) could not deploy because host is down,
// so it can be retried when the host recovers. No-op for a healthy host.
func (c *Cache) Block(repoURL, key string) {
	c.health.mu.Lock()
	defer c.health.mu.Unlock()
	if e := c.health.hosts[hostOf(repoURL)]; e != nil {
		e.blocked[key] = struct{}{}
	}
}

// Unreachable lists the hosts whose last fetch failed, sorted by host.
func (c *Cache) Unreachable() []HostStatus {
	c.health.mu.Lock()
	defer c.health.mu.Unlock()
	out := make([]HostStatus, 0, len(c.health.hosts))
	for _, e := range c.health.hosts {
		st := e.HostStatus
		for k := range e.blocked {
			st.Blocked = append(st.Blocked, k)
		}
		sort.Strings(st.Blocked)
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}

func defaultProbe(ctx context.Context, repoURL string) error {
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{Name: remoteName, URLs: []string{cleanURL(repoURL)}})
	_, err := rem.ListContext(ctx, &git.ListOptions{})
	return err
}

// Reachable asks the host for its ref list. An auth or not-found answer
// still means the host is up.
func (c *Cache) Reachable(ctx context.Context, repoURL string) bool {
	pctx, cancel := context.WithTimeout(ctx, c.cfg.FetchTimeout)
	defer cancel()
	err := c.probeFn(pctx, repoURL)
	return err == nil || !IsUnreachable(err)
}

// Watch probes every failing host each interval until ctx ends. A host that
// answers is cleared and onRecover gets the keys that were blocked on it.
func (c *Cache) Watch(ctx context.Context, interval time.Duration, onRecover func(ctx context.Context, host string, blocked []string)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		c.RecoverOnce(ctx, onRecover)
	}
}

// RecoverOnce is one Watch pass.
func (c *Cache) RecoverOnce(ctx context.Context, onRecover func(ctx context.Context, host string, blocked []string)) {
	for _, st := range c.Unreachable() {
		if !c.Reachable(ctx, st.RepoURL) {
			continue
		}
		c.health.markOK(st.RepoURL)
		c.log.Info("sourcecache: git host reachable again", "host", st.Host, "blocked", len(st.Blocked))
		if onRecover != nil {
			onRecover(ctx, st.Host, st.Blocked)
		}
	}
}
