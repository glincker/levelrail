package preview

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// publicNetwork is the Docker network a browser joins to reach a public URL.
const publicNetwork = "bridge"

type warmBrowser struct {
	b     Browser
	busy  bool
	timer *time.Timer
}

// browserPool keeps a browser warm for a short idle window so a burst of
// renders pays one container start, then stops it so nothing sits in memory.
type browserPool struct {
	mu    sync.Mutex
	items map[string]*warmBrowser
}

func newBrowserPool() *browserPool { return &browserPool{items: map[string]*warmBrowser{}} }

func (p *browserPool) closeAll() {
	p.mu.Lock()
	var victims []Browser
	for k, w := range p.items {
		if w.timer != nil {
			w.timer.Stop()
		}
		victims = append(victims, w.b)
		delete(p.items, k)
	}
	p.mu.Unlock()
	closeBrowsers(victims)
}

// closeBrowsers stops containers outside the pool lock, since a stop can be slow.
func closeBrowsers(bs []Browser) {
	for _, b := range bs {
		b.Close()
	}
}

// count is the number of live pooled browsers, for tests.
func (p *browserPool) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.items)
}

// poolKey separates browsers by network and launch args, because a public
// render pins a host with an arg that must never leak to another app.
func poolKey(spec docker.BrowserSpec) string {
	return spec.Network + "\x00" + strings.Join(spec.Command, "\x00")
}

// acquireBrowser returns a browser on spec.Network and a release func. With
// idle <= 0 the browser is stopped on release. release(false) always stops it,
// because a browser that just failed is not worth reusing.
func (m *Manager) acquireBrowser(ctx context.Context, spec docker.BrowserSpec, idle time.Duration) (Browser, func(bool), error) {
	key := poolKey(spec)
	p := m.pool
	p.mu.Lock()
	if w, ok := p.items[key]; ok && !w.busy && idle > 0 {
		w.busy = true
		if w.timer != nil {
			w.timer.Stop()
		}
		p.mu.Unlock()
		return w.b, m.releaser(key, w, idle), nil
	}
	// Starting a new browser: stop idle ones of other networks first so warm
	// browsers never add up in memory.
	var victims []Browser
	for k, w := range p.items {
		if !w.busy {
			if w.timer != nil {
				w.timer.Stop()
			}
			victims = append(victims, w.b)
			delete(p.items, k)
		}
	}
	p.mu.Unlock()
	closeBrowsers(victims)

	b, err := m.deps.Runner.StartBrowser(ctx, spec)
	if err != nil {
		return nil, nil, err
	}
	w := &warmBrowser{b: b, busy: true}
	if idle <= 0 {
		return b, func(bool) { b.Close() }, nil
	}
	p.mu.Lock()
	var stale Browser
	if old, ok := p.items[key]; ok {
		// Another worker pooled a browser for this network meanwhile.
		if old.busy {
			p.mu.Unlock()
			return b, func(bool) { b.Close() }, nil
		}
		if old.timer != nil {
			old.timer.Stop()
		}
		stale = old.b
	}
	p.items[key] = w
	p.mu.Unlock()
	if stale != nil {
		stale.Close()
	}
	return b, m.releaser(key, w, idle), nil
}

func (m *Manager) releaser(key string, w *warmBrowser, idle time.Duration) func(bool) {
	p := m.pool
	return func(reuse bool) {
		p.mu.Lock()
		if !reuse {
			if p.items[key] == w {
				delete(p.items, key)
			}
			p.mu.Unlock()
			w.b.Close()
			return
		}
		w.busy = false
		w.timer = time.AfterFunc(idle, func() {
			p.mu.Lock()
			expired := !w.busy && p.items[key] == w
			if expired {
				delete(p.items, key)
			}
			p.mu.Unlock()
			if expired {
				w.b.Close()
			}
		})
		p.mu.Unlock()
	}
}

// publicTarget validates a public render URL and pins its hostname to one
// vetted address, so the browser cannot be steered to an internal address by
// a second DNS answer. Internal destinations stay refused unless the operator
// opted in through the shared network guard.
func (m *Manager) publicTarget(ctx context.Context, base, path string) (string, []string, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", nil, netguard.ErrInvalidURL
	}
	host := strings.ToLower(u.Hostname())
	full := strings.TrimRight(u.Scheme+"://"+u.Host, "/") + path
	if ip, perr := netip.ParseAddr(host); perr == nil {
		if netguard.IsBlocked(ip) && !netguard.AllowPrivate() {
			return "", nil, fmt.Errorf("%w: %s", netguard.ErrBlockedAddress, host)
		}
		return full, nil, nil
	}
	lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addrs, err := m.lookup(lctx, host)
	if err != nil || len(addrs) == 0 {
		return "", nil, fmt.Errorf("preview: %s does not resolve", host)
	}
	var pick netip.Addr
	for _, a := range addrs {
		if netguard.IsBlocked(a) && !netguard.AllowPrivate() {
			return "", nil, fmt.Errorf("%w: %s", netguard.ErrBlockedAddress, host)
		}
		if !pick.IsValid() || (a.Is4() && !pick.Is4()) {
			pick = a
		}
	}
	ip := pick.String()
	if pick.Is6() {
		ip = "[" + ip + "]"
	}
	return full, []string{"--host-resolver-rules=MAP " + host + " " + ip}, nil
}

func sameHostURL(a, b string) bool {
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	return err1 == nil && err2 == nil && strings.EqualFold(ua.Host, ub.Host)
}

const failKeyPrefix = "render_fail:"

func (m *Manager) setFail(ctx context.Context, app, reason string) {
	switch reason {
	case "", "app_gone", "superseded", "not_serving":
		return
	}
	if err := m.deps.Store.SetPreviewState(ctx, failKeyPrefix+app, reason); err != nil {
		m.log.Warn("preview: remember render failure failed", slog.String("app", app), slog.String("error", err.Error()))
	}
}

func (m *Manager) clearFail(ctx context.Context, app string) {
	if v, err := m.deps.Store.GetPreviewState(ctx, failKeyPrefix+app); err == nil && v == "" {
		return
	}
	if err := m.deps.Store.SetPreviewState(ctx, failKeyPrefix+app, ""); err != nil {
		m.log.Warn("preview: clear render failure failed", slog.String("app", app), slog.String("error", err.Error()))
	}
}
