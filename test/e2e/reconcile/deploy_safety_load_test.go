package reconcile

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// safetyAppScript answers ok-VERSION (bad-VERSION with a 500 in
// MODE=fail500), exits soon after start in MODE=crash, and drops in-flight
// requests on SIGTERM, so only a real drain keeps them alive.
const safetyAppScript = `
const http = require('http');
const v = process.env.VERSION, mode = process.env.MODE;
if (mode === 'crash') setTimeout(() => process.exit(3), Number(process.env.CRASH_AFTER_MS || 1500));
const server = http.createServer((req, res) => {
  if (req.url === '/healthz') { res.writeHead(200); res.end('ok'); return; }
  setTimeout(() => {
    if (mode === 'fail500') { res.writeHead(500); res.end('bad-' + v); return; }
    res.writeHead(200); res.end('ok-' + v);
  }, 25);
});
server.listen(8080);
process.on('SIGTERM', () => process.exit(0));
`

// loadResult tallies what a continuous client saw through the ingress.
type loadResult struct {
	mu        sync.Mutex
	ok        map[string]int
	appErrors int
	failures  []string
	total     atomic.Int64
	last      atomic.Value
}

func newLoadResult() *loadResult { return &loadResult{ok: map[string]int{}} }

func (r *loadResult) record(body string, status int, err error) {
	r.total.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case err != nil:
		r.failures = append(r.failures, fmt.Sprintf("%s transport error: %v", time.Now().Format("15:04:05.000"), err))
	case status == http.StatusOK && strings.HasPrefix(body, "ok-"):
		r.ok[strings.TrimPrefix(body, "ok-")]++
		r.last.Store(strings.TrimPrefix(body, "ok-"))
	case status == http.StatusInternalServerError && strings.HasPrefix(body, "bad-"):
		r.appErrors++
		r.last.Store(body)
	default:
		r.failures = append(r.failures, fmt.Sprintf("%s status %d body %.60q", time.Now().Format("15:04:05.000"), status, body))
	}
}

func (r *loadResult) snapshot() (ok map[string]int, appErrors int, failures []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ok = make(map[string]int, len(r.ok))
	for k, v := range r.ok {
		ok[k] = v
	}
	return ok, r.appErrors, append([]string(nil), r.failures...)
}

func (r *loadResult) lastVersion() string {
	v, _ := r.last.Load().(string)
	return v
}

// ingressClient dials every request to the local ingress listener and
// skips verification of Caddy's locally generated certificate.
func ingressClient(addr string) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // Caddy's internal issuer is never trusted by this process
			MaxIdleConnsPerHost: 16,
		},
	}
}

// hammer sends requests from workers goroutines until ctx is done.
func hammer(ctx context.Context, client *http.Client, url string, workers int, res *loadResult) *sync.WaitGroup {
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
				if err != nil {
					return
				}
				resp, err := client.Do(req)
				if ctx.Err() != nil {
					return
				}
				if err != nil {
					res.record("", 0, err)
					continue
				}
				b, readErr := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if ctx.Err() != nil {
					return
				}
				res.record(string(b), resp.StatusCode, readErr)
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}
	return &wg
}

// waitFor polls cond every 200ms until it holds or timeout passes.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}
