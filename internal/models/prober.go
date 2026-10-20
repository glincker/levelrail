package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Phase is how far a model engine has got.
type Phase string

// Model engine phases.
const (
	PhaseUnreachable Phase = "unreachable"
	PhaseDownloading Phase = "downloading"
	PhaseLoading     Phase = "loading"
	PhaseReady       Phase = "ready"
	PhaseFailed      Phase = "failed"
)

// Status is one probe result.
type Status struct {
	Phase  Phase
	Detail string
}

// Prober reports whether a running engine has loaded its model.
type Prober interface {
	Probe(ctx context.Context, engine, dial, modelRef string) Status
}

const (
	probeTimeout   = 5 * time.Second
	pullRetryAfter = 30 * time.Second
)

// HTTPProber probes engines over their HTTP API and drives Ollama model
// pulls in the background so a reconcile pass never blocks on a download.
type HTTPProber struct {
	client *http.Client

	mu    sync.Mutex
	pulls map[string]*pullState
	loads map[string]bool
}

type pullState struct {
	detail   string
	done     bool
	err      string
	finished time.Time
}

// NewHTTPProber builds an HTTPProber.
func NewHTTPProber() *HTTPProber {
	return &HTTPProber{client: &http.Client{}, pulls: map[string]*pullState{}, loads: map[string]bool{}}
}

// Probe implements Prober.
func (p *HTTPProber) Probe(ctx context.Context, engine, dial, modelRef string) Status {
	switch engine {
	case EngineOllama:
		return p.probeOllama(ctx, dial, modelRef)
	case EngineVLLM:
		return p.probeVLLM(ctx, dial)
	case EngineLlamaCpp:
		return p.probeLlamaCpp(ctx, dial)
	}
	return Status{Phase: PhaseFailed, Detail: fmt.Sprintf("unknown engine %q", engine)}
}

func (p *HTTPProber) do(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("models: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("models: %s %s: %w", method, url, err)
	}
	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	defer c.cancel()
	return c.ReadCloser.Close()
}

func (p *HTTPProber) probeVLLM(ctx context.Context, dial string) Status {
	resp, err := p.do(ctx, http.MethodGet, "http://"+dial+"/v1/models", nil)
	if err != nil {
		return Status{PhaseLoading, "vLLM is starting; it downloads and loads weights on first start, which can take many minutes"}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Status{PhaseLoading, fmt.Sprintf("vLLM answered HTTP %d while loading", resp.StatusCode)}
	}
	return Status{Phase: PhaseReady}
}

func (p *HTTPProber) probeLlamaCpp(ctx context.Context, dial string) Status {
	resp, err := p.do(ctx, http.MethodGet, "http://"+dial+"/health", nil)
	if err != nil {
		return Status{PhaseLoading, "llama.cpp is starting; it downloads the GGUF on first start"}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Status{PhaseLoading, "llama.cpp is loading the model into VRAM"}
	}
	return Status{Phase: PhaseReady}
}

type ollamaTags struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}

func ollamaHas(list ollamaTags, ref string) bool {
	want := ref
	if !strings.Contains(ref, ":") {
		want = ref + ":latest"
	}
	for _, m := range list.Models {
		if m.Name == want || m.Model == want {
			return true
		}
	}
	return false
}

func (p *HTTPProber) getOllamaList(ctx context.Context, dial, path string) (ollamaTags, error) {
	var out ollamaTags
	resp, err := p.do(ctx, http.MethodGet, "http://"+dial+path, nil)
	if err != nil {
		return out, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("models: ollama %s returned HTTP %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("models: decode ollama %s: %w", path, err)
	}
	return out, nil
}

func (p *HTTPProber) probeOllama(ctx context.Context, dial, ref string) Status {
	tags, err := p.getOllamaList(ctx, dial, "/api/tags")
	if err != nil {
		return Status{PhaseUnreachable, "Ollama is starting"}
	}
	key := dial + "/" + ref
	if !ollamaHas(tags, ref) {
		return p.ensurePull(dial, ref, key)
	}
	p.clearPull(key)
	running, err := p.getOllamaList(ctx, dial, "/api/ps")
	if err == nil && ollamaHas(running, ref) {
		return Status{Phase: PhaseReady}
	}
	p.startLoad(dial, ref, key)
	return Status{PhaseLoading, "loading " + ref + " into VRAM"}
}

func (p *HTTPProber) clearPull(key string) {
	p.mu.Lock()
	delete(p.pulls, key)
	p.mu.Unlock()
}

func (p *HTTPProber) startLoad(dial, ref, key string) {
	p.mu.Lock()
	if p.loads[key] {
		p.mu.Unlock()
		return
	}
	p.loads[key] = true
	p.mu.Unlock()
	go func() {
		defer func() { p.mu.Lock(); delete(p.loads, key); p.mu.Unlock() }()
		body, _ := json.Marshal(map[string]any{"model": ref, "prompt": "", "keep_alive": -1, "stream": false})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+dial+"/api/generate", bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if resp, err := p.client.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
}

func (p *HTTPProber) ensurePull(dial, ref, key string) Status {
	p.mu.Lock()
	st, exists := p.pulls[key]
	if exists && st.done && time.Since(st.finished) > pullRetryAfter {
		delete(p.pulls, key)
		exists = false
	}
	if !exists {
		st = &pullState{detail: "starting download of " + ref}
		p.pulls[key] = st
		go p.runPull(dial, ref, st)
	}
	detail, errMsg := st.detail, st.err
	p.mu.Unlock()
	if errMsg != "" {
		return Status{PhaseFailed, "download failed: " + errMsg}
	}
	return Status{PhaseDownloading, detail}
}

type pullLine struct {
	Status    string `json:"status"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
	Error     string `json:"error"`
}

func (p *HTTPProber) setPull(st *pullState, fn func(*pullState)) {
	p.mu.Lock()
	fn(st)
	p.mu.Unlock()
}

func (p *HTTPProber) runPull(dial, ref string, st *pullState) {
	fail := func(msg string) {
		p.setPull(st, func(s *pullState) { s.done, s.err, s.finished = true, msg, time.Now() })
	}
	body, _ := json.Marshal(map[string]any{"model": ref, "stream": true})
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+dial+"/api/pull", bytes.NewReader(body))
	if err != nil {
		fail(err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		fail(err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var l pullLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		if l.Error != "" {
			fail(l.Error)
			return
		}
		p.setPull(st, func(s *pullState) { s.detail = pullDetail(ref, l) })
	}
	if err := sc.Err(); err != nil {
		fail(err.Error())
		return
	}
	p.setPull(st, func(s *pullState) { s.done, s.finished = true, time.Now() })
}

func pullDetail(ref string, l pullLine) string {
	if l.Total > 0 {
		return fmt.Sprintf("downloading %s: %d%% (%s of %s)", ref, l.Completed*100/l.Total, humanBytes(l.Completed), humanBytes(l.Total))
	}
	return fmt.Sprintf("downloading %s: %s", ref, l.Status)
}

func humanBytes(n int64) string {
	const gb, mb = 1 << 30, 1 << 20
	if n >= gb {
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	}
	return fmt.Sprintf("%d MB", n/mb)
}
