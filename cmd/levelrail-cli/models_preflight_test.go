package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_ModelsPreflight(t *testing.T) {
	var got apiclient.ModelPreflightRequest
	var gotPath string
	free := int64(50 << 30)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(apiclient.ModelPreflightResult{
			Repo: "acme/chat-GGUF", Status: "ok", Exists: true, Message: "Repository found.", License: "mit", FileCount: 3, TotalBytes: 6 << 30,
			EngineHint: "GGUF only", Quants: []apiclient.PreflightQuant{{Name: "Q4_K_M", Bytes: 2 << 30, Fit: "fits", Recommended: true}},
			Node: apiclient.PreflightNode{DiskFreeBytes: &free}, Disk: apiclient.PreflightDisk{Status: "ok", RequiredBytes: 2 << 30, Message: "Enough free disk for the download."},
			EstimateNote: "Fit figures are estimates.",
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	env := func(k string) (string, bool) {
		if k == "HF_TOKEN" {
			return " hf_abc ", true
		}
		return "", false
	}
	args := []string{"models", "preflight", "acme/chat-GGUF", "--engine", "llamacpp", "--quant", "Q4_K_M", "--hf-token-from-env", "--api-url", srv.URL}
	if code := run("levelrail-cli-test", args, &stdout, &stderr, env); code != exitOK {
		t.Fatalf("exit = %d stderr=%s", code, stderr.String())
	}
	if gotPath != "/api/v1/models/preflight" || got.Repo != "acme/chat-GGUF" || got.Engine != "llamacpp" || got.Quant != "Q4_K_M" || got.HFToken != "hf_abc" {
		t.Errorf("request = %s %+v", gotPath, got)
	}
	for _, want := range []string{"acme/chat-GGUF: ok", "Q4_K_M", "recommended", "fits", "disk: ok", "estimates", "free VRAM: unknown"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing %q: %s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "hf_abc") {
		t.Error("token printed")
	}
}

func TestRun_ModelsPreflight_TokenEnvUnset(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"models", "preflight", "acme/x", "--hf-token-from-env"}
	if code := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); code != exitValidation || !strings.Contains(stderr.String(), "HF_TOKEN") {
		t.Errorf("exit = %d stderr=%q", code, stderr.String())
	}
}

func TestRun_ModelsCache(t *testing.T) {
	size := int64(5 << 30)
	var pruneReq struct {
		Volumes []string `json:"volumes"`
		DryRun  bool     `json:"dry_run"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/model-cache":
			_ = json.NewEncoder(w).Encode(apiclient.ModelCacheReport{UnusedDays: 30, Note: "note", Nodes: []apiclient.ModelCacheNode{{
				Name: "local", Supported: true, TotalBytes: size, UniqueBytes: size, ReclaimableBytes: size,
				Entries: []apiclient.ModelCacheEntry{{Volume: "lr-model-old-cache", SizeBytes: &size, UnusedDays: 45, Prunable: true}},
			}}})
		case "/api/v1/model-cache/prune":
			_ = json.NewDecoder(r.Body).Decode(&pruneReq)
			_ = json.NewEncoder(w).Encode(apiclient.ModelCachePruneResult{DryRun: pruneReq.DryRun,
				Candidates: []apiclient.ModelCacheEntry{{Volume: "lr-model-old-cache"}}, Skipped: []apiclient.ModelCacheSkip{{Volume: "lr-model-live-cache", Reason: "mounted by a container"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"models", "cache", "list", "--api-url", srv.URL})
	for _, want := range []string{"lr-model-old-cache", "5.0 GiB", "prunable", "45d"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("list stdout missing %q: %s", want, stdout)
		}
	}
	stdout, _ = runCLIExpectOK(t, []string{"models", "cache", "prune", "--dry-run", "lr-model-old-cache", "--api-url", srv.URL})
	if !pruneReq.DryRun || len(pruneReq.Volumes) != 1 {
		t.Errorf("prune request = %+v", pruneReq)
	}
	if !strings.Contains(stdout, "would remove lr-model-old-cache") || !strings.Contains(stdout, "kept lr-model-live-cache: mounted by a container") {
		t.Errorf("prune stdout = %s", stdout)
	}
}
