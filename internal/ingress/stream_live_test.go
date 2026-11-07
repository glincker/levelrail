package ingress

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

const streamGap = 600 * time.Millisecond

// streamBackend writes one chunk, flushes, waits, then writes a second chunk.
func streamBackend(t *testing.T, contentType string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		fl := w.(http.Flusher)
		_, _ = fmt.Fprint(w, "first\n\n")
		fl.Flush()
		time.Sleep(streamGap)
		_, _ = fmt.Fprint(w, "second\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv
}

func streamRoute(backend string, mutate func(*ProxyRoute)) ProxyRoute {
	r := ProxyRoute{Hosts: []string{"app.test"}, BackendDial: backend}
	if mutate != nil {
		mutate(&r)
	}
	return r
}

// firstChunkDelay returns how long the first line took to reach the client.
func firstChunkDelay(t *testing.T, base string) time.Duration {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "app.test"
	start := time.Now()
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || line != "first\n" {
		t.Fatalf("first chunk = %q, err %v", line, err)
	}
	return time.Since(start)
}

// TestStream_Live proves streamed bodies reach the client before the backend finishes,
// through each handler chain that wraps the proxy.
func TestStream_Live(t *testing.T) {
	skipUnderLoad(t)
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer auth.Close()

	cases := map[string]func(*ProxyRoute){
		"plain": nil,
		"auth gate and header rules": func(r *ProxyRoute) {
			r.AuthGate = &AuthGate{URL: auth.URL, CheckPath: "/check", Timeout: time.Second}
			r.Policy = &RoutePolicy{Policy: trafficpolicy.Policy{Headers: &trafficpolicy.Headers{
				HideServer: true,
				Security:   &trafficpolicy.SecurityPreset{ContentTypeOptions: true, FrameOptions: "SAMEORIGIN"},
				Rules:      []trafficpolicy.HeaderRule{{Side: "response", Op: trafficpolicy.OpSet, Name: "X-Test", Value: "1"}},
			}}}
		},
		"error pages": func(r *ProxyRoute) { r.ErrorPages = []ErrorPage{{StatusCode: 502, Body: "down"}} },
		"waf rate limit": func(r *ProxyRoute) {
			r.WAF = &WAFConfig{RateLimitRPS: 1000, RateLimitBurst: 1000}
		},
	}
	for _, ct := range []string{"text/event-stream", "text/html; charset=utf-8"} {
		for name, mutate := range cases {
			t.Run(ct+"/"+name, func(t *testing.T) {
				backend := streamBackend(t, ct)
				addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
				cfg, err := BuildRoutesConfig(RoutesOptions{
					ServerName: "stream", ListenAddr: addr,
					Routes:   []ProxyRoute{streamRoute(backend.Listener.Addr().String(), mutate)},
					HTTPPort: freePort(t), StorageDir: t.TempDir(),
				})
				if err != nil {
					t.Fatal(err)
				}
				cfg.Apps.HTTP.Servers["stream"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}
				d := New(testLogger(t))
				if err := d.Apply(context.Background(), cfg); err != nil {
					t.Fatalf("apply: %v", err)
				}
				t.Cleanup(func() { _ = d.Stop(context.Background()) })
				if got := firstChunkDelay(t, "http://"+addr); got > streamGap/2 {
					t.Errorf("first chunk took %v, buffered until the backend finished (gap %v)", got, streamGap)
				}
			})
		}
	}
}
