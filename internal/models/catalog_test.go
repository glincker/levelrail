package models

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEmbeddedCatalogIsValid(t *testing.T) {
	cat, err := LocalCatalog()
	if err != nil {
		t.Fatalf("embedded catalog: %v", err)
	}
	if len(cat.Models) < 5 {
		t.Fatalf("expected the provisional entries, got %d", len(cat.Models))
	}
	for _, e := range cat.Models {
		if e.License != "Apache-2.0" && e.License != "MIT" {
			t.Errorf("%s has non-curated licence %s", e.ID, e.License)
		}
		if e.SHA256 == "" {
			t.Errorf("%s lacks a sha256", e.ID)
		}
	}
	if _, ok := cat.FindWeights("IBM-Granite/granite-4.1-3b-GGUF", "granite-4.1-3b-Q4_K_M.gguf"); !ok {
		t.Error("FindWeights should match the repo case-insensitively")
	}
	sorted := cat.Sorted()
	if sorted[0].Tier != TierLight || sorted[len(sorted)-1].Tier != TierPlus {
		t.Errorf("sorted order wrong: %s ... %s", sorted[0].Tier, sorted[len(sorted)-1].Tier)
	}
}

func TestParseCatalogRejects(t *testing.T) {
	base := func(mut string) string {
		return `{"version":1,"models":[{"id":"a","label":"A","repo":"o/n-GGUF","file":"a.gguf","params_b":1,"quant":"Q4_K_M","download_bytes":10,"min_ram_mib":100,"context_length":10,"license":"Apache-2.0","tier":"light","kind":"chat"` + mut + `}]}`
	}
	tests := []struct {
		name, json, want string
	}{
		{"ok", base(""), ""},
		{"gemma terms licence", strings.Replace(base(""), "Apache-2.0", "Gemma", 1), "only"},
		{"llama licence", strings.Replace(base(""), "Apache-2.0", "llama3.2", 1), "only"},
		{"bad tier", strings.Replace(base(""), `"light"`, `"huge"`, 1), "tier"},
		{"path in file", strings.Replace(base(""), "a.gguf", "../a.gguf", 1), "invalid repo or file"},
		{"not gguf", strings.Replace(base(""), "a.gguf", "a.bin", 1), "invalid repo or file"},
		{"bad sha", base(`,"sha256":"zz"`), "sha256"},
		{"empty", `{"version":1,"models":[]}`, "empty"},
		{"duplicate", strings.Replace(base(""), `}]}`, `},`+base("")[strings.Index(base(""), `{"id"`):], 1), "duplicated"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCatalog([]byte(tt.json))
			if tt.want == "" && err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func cpuHost() HostFactsFunc {
	return func(context.Context, string) (HostFacts, bool) {
		return HostFacts{CPUs: 4, MemTotalBytes: 16 * gib, MemAvailBytes: 12 * gib}, true
	}
}

func TestService_CreateCPULicensePolicy(t *testing.T) {
	ctx := context.Background()
	newCPUSvc := func(t *testing.T) *Service {
		svc, _ := newSvc(t, nil)
		svc.SetHostFacts(cpuHost())
		svc.SetCPUPolicy(testPolicy())
		return svc
	}
	t.Run("curated entry needs no acknowledgement and stores caps", func(t *testing.T) {
		svc := newCPUSvc(t)
		got, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "local"}, CatalogID: "granite-4.1-3b"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		m := got.Model
		if m.Compute != ComputeCPU || m.Engine != EngineLlamaCpp || m.CPUMilli != 2000 || m.MemoryMiB != 4096 || m.GPUCount != 0 ||
			m.WeightsFile != "granite-4.1-3b-Q4_K_M.gguf" || m.WeightsSHA256 == "" || m.LicenseAck || m.ContextLength != 4096 {
			t.Fatalf("model = %+v", m)
		}
	})
	t.Run("custom repository needs an explicit acknowledgement", func(t *testing.T) {
		svc := newCPUSvc(t)
		in := CreateInput{Spec: Spec{Name: "custom", ModelRef: "someone/cool-3B-GGUF:Q4_K_M"}, Compute: ComputeCPU, WeightsFile: "cool-3b-q4_k_m.gguf"}
		if _, err := svc.Create(ctx, in); !errors.Is(err, ErrLicenseAck) {
			t.Fatalf("err = %v, want ErrLicenseAck", err)
		}
		in.LicenseAck = true
		got, err := svc.Create(ctx, in)
		if err != nil {
			t.Fatalf("Create with ack: %v", err)
		}
		if !got.Model.LicenseAck || got.Model.WeightsRepo != "someone/cool-3B-GGUF" || got.Model.WeightsFile != "cool-3b-q4_k_m.gguf" {
			t.Fatalf("model = %+v", got.Model)
		}
	})
	t.Run("a curated repo given by hand is curated", func(t *testing.T) {
		svc := newCPUSvc(t)
		_, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "byhand", ModelRef: "ibm-granite/granite-4.0-1b-GGUF:Q4_K_M"}, Compute: ComputeCPU,
			WeightsFile: "granite-4.0-1b-Q4_K_M.gguf"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
	})
	t.Run("unknown catalog id and gpu engines are refused", func(t *testing.T) {
		svc := newCPUSvc(t)
		if _, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "x"}, CatalogID: "nope"}); !errors.Is(err, ErrInvalid) {
			t.Errorf("unknown id err = %v", err)
		}
		if _, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "y", Engine: EngineVLLM, ModelRef: "a/b"}, CatalogID: "granite-4.1-3b", SwapGroup: "g"}); !errors.Is(err, ErrInvalid) {
			t.Errorf("swap group err = %v", err)
		}
	})
	t.Run("a model that cannot fit the cap is refused with a suggestion", func(t *testing.T) {
		svc := newCPUSvc(t)
		_, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "big"}, CatalogID: "gemma-4-e4b", MemoryMiB: 3072})
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "will not fit") || !strings.Contains(err.Error(), "Try") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a gpu model is untouched by cpu planning", func(t *testing.T) {
		svc := newCPUSvc(t)
		got, err := svc.Create(ctx, CreateInput{Spec: spec("gpuone")})
		if err != nil || got.Model.Compute != ComputeGPU || got.Model.CPUMilli != 0 || got.Model.GPUCount != -1 {
			t.Fatalf("got %+v, %v", got.Model, err)
		}
	})
}
