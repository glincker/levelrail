package models

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestUsableVRAM(t *testing.T) {
	devs := []gpu.Device{
		{Index: 0, VRAMTotalMiB: 24 * 1024, VRAMUsedMiB: 0},
		{Index: 1, VRAMTotalMiB: 24 * 1024, VRAMUsedMiB: 20 * 1024},
		{Index: 2, VRAMTotalMiB: 8 * 1024, VRAMUsedMiB: 9 * 1024},
	}
	tests := []struct {
		name      string
		count     int
		wantFree  int64
		wantTotal int64
	}{
		{"all when zero", 0, 28 * 1024 * mib, 56 * 1024 * mib},
		{"all when negative", -1, 24*1024*mib + 4*1024*mib, 56 * 1024 * mib},
		{"one gpu takes the tightest", 1, 0, 8 * 1024 * mib},
		{"two gpus take the two tightest", 2, 4 * 1024 * mib, 32 * 1024 * mib},
		{"more than present means all", 9, 24*1024*mib + 4*1024*mib, 56 * 1024 * mib},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, free := usableVRAM(devs, tt.count)
			if total != tt.wantTotal || free != tt.wantFree {
				t.Errorf("total=%d free=%d, want %d %d", total, free, tt.wantTotal, tt.wantFree)
			}
		})
	}
}

func TestPreflight_GPUCountLimitsFit(t *testing.T) {
	_, cfg := newHub(t, map[string]fakeRepo{"acme/open-GGUF": {files: llamaFiles()}})
	svc := newPreflightSvc(t, cfg, nil)
	db := svc.store.(*store.DB)
	info := gpu.Info{Present: true, RuntimeInstalled: true, Devices: []gpu.Device{
		{Index: 0, VRAMTotalMiB: 24 * 1024},
		{Index: 1, VRAMTotalMiB: 4 * 1024},
	}}
	if err := db.SetNodeGPU(context.Background(), store.LocalNodeGPUKey, info); err != nil {
		t.Fatal(err)
	}
	all, err := svc.Preflight(context.Background(), PreflightInput{Repo: "acme/open-GGUF", Engine: EngineLlamaCpp})
	if err != nil {
		t.Fatal(err)
	}
	one, err := svc.Preflight(context.Background(), PreflightInput{Repo: "acme/open-GGUF", Engine: EngineLlamaCpp, GPUCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if all.RecommendedQuant != "Q8_0" || one.RecommendedQuant != "Q4_K_M" {
		t.Errorf("recommended all=%q one=%q, want Q8_0 and Q4_K_M", all.RecommendedQuant, one.RecommendedQuant)
	}
	if *one.Node.VRAMFreeBytes != 4*1024*mib {
		t.Errorf("free vram = %d", *one.Node.VRAMFreeBytes)
	}
}

func TestPreflight_GatedOllamaIsBlockedEvenWithAccess(t *testing.T) {
	_, cfg := newHub(t, map[string]fakeRepo{
		"acme/gated-GGUF": {gated: "manual", files: llamaFiles(), tokenOK: "hf_good"},
	})
	svc := newPreflightSvc(t, cfg, nil)
	tests := []struct {
		engine string
		want   string
	}{
		{EngineOllama, HFStatusGated},
		{EngineLlamaCpp, HFStatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			got, err := svc.Preflight(context.Background(), PreflightInput{Repo: "acme/gated-GGUF", Engine: tt.engine, Token: "hf_good"})
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tt.want {
				t.Errorf("status = %q, want %q (%s)", got.Status, tt.want, got.Message)
			}
			if tt.engine == EngineOllama && !strings.Contains(got.Message, "Ollama") {
				t.Errorf("message = %q", got.Message)
			}
		})
	}
}

func TestPreflight_ExplicitFilePastListLimitIsFound(t *testing.T) {
	files := make([]hubFile, 0, 60)
	for i := range 60 {
		files = append(files, hubFile{Name: "shard-" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".safetensors", Size: gib})
	}
	files = append(files, hubFile{Name: "tiny.gguf", Size: 10})
	_, cfg := newHub(t, map[string]fakeRepo{"acme/many": {files: files}})
	svc := newPreflightSvc(t, cfg, nil)
	svc.preflight.maxFiles = 5

	got, err := svc.Preflight(context.Background(), PreflightInput{Repo: "acme/many", File: "tiny.gguf"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.FilesTruncated {
		t.Fatal("listing should be truncated")
	}
	if got.Selected == nil || got.Selected.Label != "tiny.gguf" || got.Selected.Bytes != 10 {
		t.Errorf("selected = %+v warnings %v", got.Selected, got.Warnings)
	}
}

func TestHFCacheKeyKeepsFullTokenDigest(t *testing.T) {
	a, b := cacheKey("acme/x", "token-a"), cacheKey("acme/x", "token-b")
	if a == b {
		t.Fatal("distinct tokens share a key")
	}
	if _, digest, _ := strings.Cut(a, "|"); len(digest) != 64 {
		t.Errorf("digest length = %d, want 64", len(digest))
	}
}
