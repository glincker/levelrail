package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/store"
)

const gib = int64(1) << 30

type hubFile struct {
	Name string `json:"rfilename"`
	Size int64  `json:"size"`
}

// fakeHub serves the Hub endpoints Preflight uses.
type fakeHub struct {
	t        *testing.T
	repos    map[string]fakeRepo
	requests atomic.Int64
	lastAuth atomic.Value
}

type fakeRepo struct {
	gated      any
	license    string
	files      []hubFile
	rateLimit  bool
	serverErr  bool
	tokenOK    string
	needsToken bool
}

func (h *fakeHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.requests.Add(1)
	auth := r.Header.Get("Authorization")
	h.lastAuth.Store(auth)
	path := strings.TrimPrefix(r.URL.Path, "/")
	if strings.HasPrefix(path, "api/models/") {
		id := strings.TrimPrefix(path, "api/models/")
		repo, ok := h.repos[id]
		switch {
		case !ok:
			http.Error(w, `{"error":"Repository not found"}`, http.StatusUnauthorized)
		case repo.rateLimit:
			w.Header().Set("Retry-After", "42")
			http.Error(w, "slow down", http.StatusTooManyRequests)
		case repo.serverErr:
			http.Error(w, "boom", http.StatusBadGateway)
		case repo.needsToken && auth == "":
			http.Error(w, `{"error":"Invalid credentials"}`, http.StatusUnauthorized)
		default:
			body := map[string]any{"id": id, "gated": repo.gated, "cardData": map[string]any{"license": repo.license}, "siblings": repo.files}
			_ = json.NewEncoder(w).Encode(body)
		}
		return
	}
	// resolve probe: <owner>/<name>/resolve/main/<file>
	parts := strings.SplitN(path, "/resolve/", 2)
	if len(parts) == 2 {
		repo := h.repos[parts[0]]
		if repo.tokenOK != "" && auth == "Bearer "+repo.tokenOK {
			w.Header().Set("Location", "https://cdn.example.invalid/blob")
			w.WriteHeader(http.StatusFound)
			return
		}
		http.Error(w, "gated", http.StatusForbidden)
		return
	}
	http.NotFound(w, r)
}

func newHub(t *testing.T, repos map[string]fakeRepo) (*fakeHub, HFConfig) {
	t.Helper()
	hub := &fakeHub{t: t, repos: repos}
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)
	cfg := HFConfig{BaseURL: srv.URL, Timeout: 2 * time.Second, CacheTTL: time.Minute, MaxResponseBytes: 1 << 20, CacheEntries: 8}
	return hub, cfg
}

func llamaFiles() []hubFile {
	return []hubFile{
		{"README.md", 5_000},
		{"Llama-3.2-3B-Instruct-Q2_K.gguf", 2 * gib},
		{"Llama-3.2-3B-Instruct-Q4_K_M.gguf", 2*gib + gib/2},
		{"Llama-3.2-3B-Instruct-Q8_0.gguf", 4 * gib},
		{"Llama-3.2-3B-Instruct-f16.gguf", 7 * gib},
		{"mmproj-model-f16.gguf", gib / 4},
	}
}

func newPreflightSvc(t *testing.T, cfg HFConfig, freeDisk *int64) *Service {
	t.Helper()
	svc, _ := newSvc(t, nil)
	svc.SetHuggingFace(NewHFClient(cfg, http.DefaultClient, slog.New(slog.NewTextHandler(io.Discard, nil))), FitConfig{OverheadPercent: 20, FitPercent: 90, DiskHeadroomPercent: 10})
	if freeDisk != nil {
		svc.SetDiskFacts(func(context.Context, string) (int64, int64, bool) { return *freeDisk, 100 * gib, true })
	}
	return svc
}

func setGPU(t *testing.T, svc *Service, totalMiB, usedMiB int64) {
	t.Helper()
	db := svc.store.(*store.DB)
	info := gpu.Info{Present: true, RuntimeInstalled: true, Devices: []gpu.Device{{Index: 0, VRAMTotalMiB: totalMiB, VRAMUsedMiB: usedMiB}}}
	if err := db.SetNodeGPU(context.Background(), store.LocalNodeGPUKey, info); err != nil {
		t.Fatalf("SetNodeGPU: %v", err)
	}
}

func TestPreflight_HubOutcomes(t *testing.T) {
	repos := map[string]fakeRepo{
		"acme/open-GGUF":  {license: "apache-2.0", files: llamaFiles()},
		"acme/gated-open": {gated: "manual", license: "llama3.2", files: []hubFile{{"config.json", 1000}, {"model.safetensors", 6 * gib}}, tokenOK: "hf_good"},
		"acme/limited":    {rateLimit: true},
		"acme/broken":     {serverErr: true},
		"acme/private":    {needsToken: true, files: []hubFile{{"model.safetensors", gib}}},
	}
	_, cfg := newHub(t, repos)
	svc := newPreflightSvc(t, cfg, nil)
	ctx := context.Background()

	tests := []struct {
		name       string
		in         PreflightInput
		wantStatus string
		wantAccess string
		wantMsg    string
	}{
		{name: "found", in: PreflightInput{Repo: "acme/open-GGUF"}, wantStatus: HFStatusOK, wantAccess: "not_required"},
		{name: "found via url", in: PreflightInput{Repo: "https://huggingface.co/acme/open-GGUF"}, wantStatus: HFStatusOK, wantAccess: "not_required"},
		{name: "missing", in: PreflightInput{Repo: "acme/nope"}, wantStatus: HFStatusNotFound, wantMsg: "private"},
		{name: "gated without token", in: PreflightInput{Repo: "acme/gated-open"}, wantStatus: HFStatusGated, wantAccess: "denied", wantMsg: "gated"},
		{name: "gated with wrong token", in: PreflightInput{Repo: "acme/gated-open", Token: "hf_bad"}, wantStatus: HFStatusGated, wantAccess: "denied", wantMsg: "does not have access"},
		{name: "gated with good token", in: PreflightInput{Repo: "acme/gated-open", Token: "hf_good"}, wantStatus: HFStatusOK, wantAccess: "granted"},
		{name: "rate limited", in: PreflightInput{Repo: "acme/limited"}, wantStatus: HFStatusRateLimited, wantMsg: "rate limiting"},
		{name: "upstream error", in: PreflightInput{Repo: "acme/broken"}, wantStatus: HFStatusUnavailable, wantMsg: "502"},
		{name: "private needs token", in: PreflightInput{Repo: "acme/private"}, wantStatus: HFStatusNotFound},
		{name: "private with token", in: PreflightInput{Repo: "acme/private", Token: "hf_any"}, wantStatus: HFStatusOK, wantAccess: "not_required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.Preflight(ctx, tt.in)
			if err != nil {
				t.Fatalf("Preflight: %v", err)
			}
			if got.Status != tt.wantStatus || (tt.wantAccess != "" && got.Access != tt.wantAccess) {
				t.Errorf("status=%q access=%q, want %q %q (msg %q)", got.Status, got.Access, tt.wantStatus, tt.wantAccess, got.Message)
			}
			if !strings.Contains(got.Message, tt.wantMsg) {
				t.Errorf("message %q lacks %q", got.Message, tt.wantMsg)
			}
			if got.Status != HFStatusOK && got.NextStep == "" {
				t.Errorf("status %q has no next step", got.Status)
			}
		})
	}
}

func TestPreflight_RateLimitCarriesRetryAfterAndIsNotCached(t *testing.T) {
	hub, cfg := newHub(t, map[string]fakeRepo{"acme/limited": {rateLimit: true}})
	svc := newPreflightSvc(t, cfg, nil)
	for i := 0; i < 2; i++ {
		got, err := svc.Preflight(context.Background(), PreflightInput{Repo: "acme/limited"})
		if err != nil || got.RetryAfterSeconds != 42 {
			t.Fatalf("got %+v err %v", got, err)
		}
	}
	if n := hub.requests.Load(); n != 2 {
		t.Errorf("requests = %d, want 2 (rate limit must not be cached)", n)
	}
}

func TestPreflight_CacheTTL(t *testing.T) {
	hub, cfg := newHub(t, map[string]fakeRepo{"acme/open": {files: llamaFiles()}})
	svc := newPreflightSvc(t, cfg, nil)
	now := time.Now()
	svc.preflight.hf.now = func() time.Time { return now }
	ctx := context.Background()

	first, _ := svc.Preflight(ctx, PreflightInput{Repo: "acme/open"})
	second, _ := svc.Preflight(ctx, PreflightInput{Repo: "acme/open"})
	if first.Cached || !second.Cached || hub.requests.Load() != 1 {
		t.Fatalf("cached first=%v second=%v requests=%d", first.Cached, second.Cached, hub.requests.Load())
	}
	now = now.Add(2 * time.Minute)
	third, _ := svc.Preflight(ctx, PreflightInput{Repo: "acme/open"})
	if third.Cached || hub.requests.Load() != 2 {
		t.Errorf("after TTL: cached=%v requests=%d", third.Cached, hub.requests.Load())
	}
}

func TestPreflight_TokenNeverInResultOrLogs(t *testing.T) {
	const secret = "hf_supersecret_token_value"
	var logs strings.Builder
	hub, cfg := newHub(t, map[string]fakeRepo{"acme/g": {gated: true, files: []hubFile{{"config.json", 1}}, tokenOK: secret}, "acme/limited": {rateLimit: true}})
	svc := newPreflightSvc(t, cfg, nil)
	svc.preflight.hf.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ok, _ := svc.Preflight(context.Background(), PreflightInput{Repo: "acme/g", Token: secret})
	limited, _ := svc.Preflight(context.Background(), PreflightInput{Repo: "acme/limited", Token: secret})
	for name, v := range map[string]any{"ok": ok, "limited": limited} {
		b, _ := json.Marshal(v)
		if strings.Contains(string(b), secret) {
			t.Errorf("%s result leaks the token", name)
		}
	}
	if strings.Contains(logs.String(), secret) {
		t.Errorf("logs leak the token: %s", logs.String())
	}
	if got, _ := hub.lastAuth.Load().(string); got != "Bearer "+secret {
		t.Errorf("token not sent to the hub, auth = %q", got)
	}
}

func TestPreflight_HugeRepoIsCappedAndSummed(t *testing.T) {
	files := make([]hubFile, 0, 5000)
	for i := 0; i < 5000; i++ {
		files = append(files, hubFile{fmt.Sprintf("shards/model-%05d-of-05000.safetensors", i), gib / 10})
	}
	_, cfg := newHub(t, map[string]fakeRepo{"acme/huge": {files: files}})
	cfg.MaxResponseBytes = 4 << 20
	svc := newPreflightSvc(t, cfg, nil)
	svc.preflight.maxFiles = 50

	got, err := svc.Preflight(context.Background(), PreflightInput{Repo: "acme/huge", Engine: EngineVLLM})
	if err != nil {
		t.Fatal(err)
	}
	want := int64(5000) * (gib / 10)
	if got.FileCount != 5000 || len(got.Files) != 50 || !got.FilesTruncated || got.TotalBytes != want {
		t.Errorf("count=%d listed=%d truncated=%v total=%d want %d", got.FileCount, len(got.Files), got.FilesTruncated, got.TotalBytes, want)
	}
	if got.Selected == nil || got.Selected.Bytes != want {
		t.Errorf("selected = %+v", got.Selected)
	}
}

func TestPreflight_ResponseOverCapIsUnavailable(t *testing.T) {
	files := make([]hubFile, 0, 2000)
	for i := 0; i < 2000; i++ {
		files = append(files, hubFile{fmt.Sprintf("f-%05d.safetensors", i), 1})
	}
	_, cfg := newHub(t, map[string]fakeRepo{"acme/huge": {files: files}})
	cfg.MaxResponseBytes = 4096
	svc := newPreflightSvc(t, cfg, nil)
	got, err := svc.Preflight(context.Background(), PreflightInput{Repo: "acme/huge"})
	if err != nil || got.Status != HFStatusUnavailable || !strings.Contains(got.Message, "size cap") {
		t.Errorf("got %+v err %v", got, err)
	}
}

func TestPreflight_QuantFitAndDisk(t *testing.T) {
	_, cfg := newHub(t, map[string]fakeRepo{"acme/open-GGUF": {license: "mit", files: llamaFiles()}})
	tests := []struct {
		name         string
		vramMiB      int64
		usedMiB      int64
		diskFree     *int64
		quant        string
		wantRec      string
		wantDisk     string
		wantSelected string
	}{
		{name: "plenty of vram picks q8", vramMiB: 24 * 1024, wantRec: "Q8_0", diskFree: ptr(50 * gib), wantDisk: "ok", wantSelected: "Q8_0"},
		{name: "small gpu picks q4", vramMiB: 4 * 1024, wantRec: "Q4_K_M", diskFree: ptr(50 * gib), wantDisk: "ok", wantSelected: "Q4_K_M"},
		{name: "used vram counts", vramMiB: 24 * 1024, usedMiB: 21 * 1024, wantRec: "Q2_K", diskFree: ptr(50 * gib), wantDisk: "ok", wantSelected: "Q2_K"},
		{name: "explicit quant beats recommendation", vramMiB: 24 * 1024, quant: "q2_k", diskFree: ptr(50 * gib), wantRec: "Q8_0", wantDisk: "ok", wantSelected: "Q2_K"},
		{name: "disk too small", vramMiB: 24 * 1024, diskFree: ptr(gib), wantRec: "", wantDisk: "insufficient"},
		{name: "disk unknown", vramMiB: 24 * 1024, wantRec: "Q8_0", wantDisk: "unknown", wantSelected: "Q8_0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newPreflightSvc(t, cfg, tt.diskFree)
			setGPU(t, svc, tt.vramMiB, tt.usedMiB)
			got, err := svc.Preflight(context.Background(), PreflightInput{Repo: "acme/open-GGUF", Engine: EngineLlamaCpp, Quant: tt.quant})
			if err != nil {
				t.Fatal(err)
			}
			if got.RecommendedQuant != tt.wantRec || got.Disk.Status != tt.wantDisk {
				t.Errorf("rec=%q disk=%q, want %q %q (%+v)", got.RecommendedQuant, got.Disk.Status, tt.wantRec, tt.wantDisk, got.Quants)
			}
			if tt.wantSelected != "" && (got.Selected == nil || got.Selected.Label != tt.wantSelected) {
				t.Errorf("selected = %+v, want %q", got.Selected, tt.wantSelected)
			}
			if !strings.Contains(got.EstimateNote, "estimates") {
				t.Errorf("estimate note missing: %q", got.EstimateNote)
			}
			if got.License != "mit" || !got.HasGGUF || got.HasSafetensors {
				t.Errorf("metadata = %+v", got)
			}
		})
	}
}

func TestPreflight_NoNodeFactsIsUnknownNotFits(t *testing.T) {
	_, cfg := newHub(t, map[string]fakeRepo{"acme/open-GGUF": {files: llamaFiles()}})
	svc := newPreflightSvc(t, cfg, nil)
	got, err := svc.Preflight(context.Background(), PreflightInput{Repo: "acme/open-GGUF", Engine: EngineLlamaCpp})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range got.Quants {
		if q.Fit != FitUnknown {
			t.Errorf("%s fit = %q, want unknown without node facts", q.Name, q.Fit)
		}
	}
	if got.Node.VRAMFreeBytes != nil || got.Node.DiskFreeBytes != nil || got.Disk.Status != "unknown" {
		t.Errorf("node facts should be unknown: %+v %+v", got.Node, got.Disk)
	}
	if got.RecommendedQuant != "Q4_K_M" || !strings.Contains(got.RecommendationNote, "unknown") {
		t.Errorf("rec=%q note=%q", got.RecommendedQuant, got.RecommendationNote)
	}
}

func TestPreflight_EngineCompatibility(t *testing.T) {
	_, cfg := newHub(t, map[string]fakeRepo{
		"acme/gguf-only": {files: llamaFiles()},
		"acme/st-only":   {files: []hubFile{{"model.safetensors", gib}}},
		"acme/both":      {files: append(llamaFiles(), hubFile{"model.safetensors", gib})},
		"acme/neither":   {files: []hubFile{{"README.md", 1}}},
	})
	svc := newPreflightSvc(t, cfg, nil)
	tests := []struct {
		repo, engine string
		want         []string
		wantWarn     bool
	}{
		{"acme/gguf-only", EngineLlamaCpp, []string{EngineLlamaCpp, EngineOllama}, false},
		{"acme/gguf-only", EngineVLLM, []string{EngineLlamaCpp, EngineOllama}, true},
		{"acme/st-only", EngineVLLM, []string{EngineVLLM}, false},
		{"acme/st-only", EngineLlamaCpp, []string{EngineVLLM}, true},
		{"acme/both", EngineOllama, []string{EngineLlamaCpp, EngineOllama, EngineVLLM}, false},
		{"acme/neither", "", []string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.repo+"/"+tt.engine, func(t *testing.T) {
			got, err := svc.Preflight(context.Background(), PreflightInput{Repo: tt.repo, Engine: tt.engine})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got.CompatibleEngines, ",") != strings.Join(tt.want, ",") || (len(got.Warnings) > 0) != tt.wantWarn {
				t.Errorf("engines=%v warnings=%v, want %v warn=%v", got.CompatibleEngines, got.Warnings, tt.want, tt.wantWarn)
			}
		})
	}
}

func TestPreflight_InvalidInput(t *testing.T) {
	_, cfg := newHub(t, nil)
	svc := newPreflightSvc(t, cfg, nil)
	for _, repo := range []string{"", "noslash", "a/b/c", "../etc/passwd", "acme/x?y=1", "acme/x y"} {
		if _, err := svc.Preflight(context.Background(), PreflightInput{Repo: repo}); err == nil {
			t.Errorf("repo %q accepted", repo)
		}
	}
	if _, err := svc.Preflight(context.Background(), PreflightInput{Repo: "acme/x", Engine: "tgi"}); err == nil {
		t.Error("bad engine accepted")
	}
	got, err := svc.Preflight(context.Background(), PreflightInput{Repo: "llama3.1:8b", Engine: EngineOllama})
	if err != nil || got.Status != HFStatusUnsupported {
		t.Errorf("ollama tag: %+v %v", got, err)
	}
}

func TestParseHFRepo(t *testing.T) {
	tests := []struct {
		in, repo, quant string
		ok              bool
	}{
		{"acme/model", "acme/model", "", true},
		{"acme/model:Q4_K_M", "acme/model", "Q4_K_M", true},
		{"hf.co/acme/model:Q8_0", "acme/model", "Q8_0", true},
		{"https://huggingface.co/acme/model", "acme/model", "", true},
		{"  acme/model  ", "acme/model", "", true},
		{"acme", "", "", false},
		{"acme/model:", "acme/model", "", true},
		{"acme/mo del", "", "", false},
	}
	for _, tt := range tests {
		repo, quant, ok := ParseHFRepo(tt.in)
		if repo != tt.repo || quant != tt.quant || ok != tt.ok {
			t.Errorf("ParseHFRepo(%q) = %q %q %v, want %q %q %v", tt.in, repo, quant, ok, tt.repo, tt.quant, tt.ok)
		}
	}
}

func ptr(v int64) *int64 { return &v }

func TestHFClient_DefaultClientRefusesInternalAddresses(t *testing.T) {
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "")
	_, cfg := newHub(t, map[string]fakeRepo{"acme/x": {files: llamaFiles()}})
	c := NewHFClient(cfg, nil, nil)
	_, _, err := c.Repo(context.Background(), "acme/x", "")
	var he *HFError
	if !errors.As(err, &he) || he.Status != HFStatusUnavailable || !strings.Contains(he.Message, "blocked") {
		t.Errorf("err = %v, want unavailable with a blocked-address message", err)
	}
}
