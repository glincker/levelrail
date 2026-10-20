package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeOllama struct {
	mu       sync.Mutex
	tags     []string
	running  []string
	pulls    int
	loads    int
	pullFail bool
}

func (f *fakeOllama) list(names []string) string {
	var items []string
	for _, n := range names {
		items = append(items, fmt.Sprintf(`{"name":%q,"model":%q}`, n, n))
	}
	return `{"models":[` + strings.Join(items, ",") + `]}`
}

func (f *fakeOllama) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/api/tags":
		_, _ = fmt.Fprint(w, f.list(f.tags))
	case "/api/ps":
		_, _ = fmt.Fprint(w, f.list(f.running))
	case "/api/pull":
		f.pulls++
		var req struct{ Model string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		if f.pullFail {
			_, _ = fmt.Fprintln(w, `{"error":"manifest unknown"}`)
			return
		}
		_, _ = fmt.Fprintln(w, `{"status":"pulling manifest"}`)
		_, _ = fmt.Fprintln(w, `{"status":"pulling abc","total":4000000000,"completed":1000000000}`)
		f.tags = append(f.tags, req.Model)
	case "/api/generate":
		f.loads++
		f.running = append(f.running, "llama3.1:8b")
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestHTTPProber_OllamaLifecycle(t *testing.T) {
	f := &fakeOllama{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	dial := strings.TrimPrefix(srv.URL, "http://")
	p := NewHTTPProber()
	ctx := context.Background()

	st := p.Probe(ctx, EngineOllama, dial, "llama3.1:8b")
	if st.Phase != PhaseDownloading {
		t.Fatalf("first probe = %+v, want downloading", st)
	}
	waitFor(t, "pull to finish", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.pulls == 1 && len(f.tags) == 1
	})

	st = p.Probe(ctx, EngineOllama, dial, "llama3.1:8b")
	if st.Phase != PhaseLoading {
		t.Fatalf("after pull probe = %+v, want loading", st)
	}
	waitFor(t, "load to finish", func() bool {
		return p.Probe(ctx, EngineOllama, dial, "llama3.1:8b").Phase == PhaseReady
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pulls != 1 {
		t.Errorf("pulls = %d, want exactly 1", f.pulls)
	}
}

func TestHTTPProber_OllamaDownloadProgressAndFailure(t *testing.T) {
	p := NewHTTPProber()
	got := pullDetail("m", pullLine{Total: 4 << 30, Completed: 1 << 30})
	if !strings.Contains(got, "25%") || !strings.Contains(got, "1.0 GB of 4.0 GB") {
		t.Errorf("pullDetail = %q", got)
	}

	f := &fakeOllama{pullFail: true}
	srv := httptest.NewServer(f)
	defer srv.Close()
	dial := strings.TrimPrefix(srv.URL, "http://")
	p.Probe(context.Background(), EngineOllama, dial, "nope:1")
	waitFor(t, "failed pull", func() bool {
		st := p.Probe(context.Background(), EngineOllama, dial, "nope:1")
		return st.Phase == PhaseFailed && strings.Contains(st.Detail, "manifest unknown")
	})
	f.mu.Lock()
	pulls := f.pulls
	f.mu.Unlock()
	if pulls != 1 {
		t.Errorf("a failed pull must not be retried within the backoff window, pulls = %d", pulls)
	}
}

func TestHTTPProber_OllamaUnreachable(t *testing.T) {
	st := NewHTTPProber().Probe(context.Background(), EngineOllama, "127.0.0.1:1", "m")
	if st.Phase != PhaseUnreachable {
		t.Errorf("phase = %s, want unreachable", st.Phase)
	}
}

func TestHTTPProber_VLLMAndLlamaCpp(t *testing.T) {
	code := http.StatusServiceUnavailable
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.WriteHeader(code)
	}))
	defer srv.Close()
	dial := strings.TrimPrefix(srv.URL, "http://")
	p := NewHTTPProber()
	ctx := context.Background()

	for _, engine := range []string{EngineVLLM, EngineLlamaCpp} {
		if st := p.Probe(ctx, engine, dial, "o/m"); st.Phase != PhaseLoading {
			t.Errorf("%s while 503: %+v, want loading", engine, st)
		}
		if st := p.Probe(ctx, engine, "127.0.0.1:1", "o/m"); st.Phase != PhaseLoading {
			t.Errorf("%s unreachable: %+v, want loading", engine, st)
		}
	}
	mu.Lock()
	code = http.StatusOK
	mu.Unlock()
	for _, engine := range []string{EngineVLLM, EngineLlamaCpp} {
		if st := p.Probe(ctx, engine, dial, "o/m"); st.Phase != PhaseReady {
			t.Errorf("%s while 200: %+v, want ready", engine, st)
		}
	}
	if st := p.Probe(ctx, "tgi", dial, "x"); st.Phase != PhaseFailed {
		t.Errorf("unknown engine: %+v", st)
	}
}

func TestHostResolverAndLister(t *testing.T) {
	h := NewHostResolver("203-0-113-5", fallbackFn)
	if got := h.Hosts(modelWith("a", "x.example.com")); len(got) != 1 || got[0] != "x.example.com" {
		t.Errorf("explicit domain hosts = %v", got)
	}
	if got := h.Hosts(modelWith("a", "")); len(got) != 1 || got[0] != "model-a.203-0-113-5.sslip.io" {
		t.Errorf("fallback hosts = %v", got)
	}
	if got := NewHostResolver("", nil).Hosts(modelWith("a", "")); got != nil {
		t.Errorf("no fallback hosts = %v", got)
	}
	if got := NewHostResolver("", nil).BaseURL(modelWith("a", "")); got != "" {
		t.Errorf("BaseURL without host = %q", got)
	}

	live, gone := modelWith("live", "live.example.com"), modelWith("gone", "gone.example.com")
	gone.Deleting = true
	hosts, err := HostLister{Store: listStore{[]store.Model{live, gone}}, Hosts: h}.ModelHosts(context.Background())
	if err != nil || len(hosts) != 1 || hosts[0] != "live.example.com" {
		t.Errorf("ModelHosts = %v, %v", hosts, err)
	}
}

func modelWith(name, domain string) store.Model {
	return store.Model{Name: name, Domain: domain}
}
