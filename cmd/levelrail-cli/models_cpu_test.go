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

func TestRun_ModelsDeployCPU(t *testing.T) {
	var got apiclient.CreateModelRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		d := apiclient.ModelDetail{ModelResource: sampleModel(), ModelCPUFields: apiclient.ModelCPUFields{Compute: "cpu", CPULimit: 2, MemoryLimitMiB: 4096}}
		_ = json.NewEncoder(w).Encode(apiclient.CreateModelDetailResponse{ModelDetail: d, APIKey: "lr-secretsecret"}) //nolint:gosec // test fixture
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"models", "deploy", "--catalog", "granite-4.1-3b", "--cpus", "2", "--memory-mib", "4096", "--api-url", srv.URL})
	if got.Compute != "cpu" || got.CatalogID != "granite-4.1-3b" || got.Name != "granite-4-1-3b" || got.Engine != "llamacpp" || got.CPUs != 2 || got.MemoryMiB != 4096 || got.GPUCount != 0 {
		t.Errorf("request = %+v", got)
	}
	for _, want := range []string{"cpu mode", "2.00 CPUs", "4096 MiB", "other apps keep priority"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}
}

func TestRun_ModelsDeployCPU_Validation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"cpu needs a model", []string{"models", "deploy", "--compute", "cpu", "--name", "x"}, "--catalog"},
		{"bad compute", []string{"models", "deploy", "--compute", "tpu", "--name", "x"}, "--compute"},
		{"negative cpus", []string{"models", "deploy", "--catalog", "granite-4.1-3b", "--cpus", "-1"}, "negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run("levelrail-cli-test", tt.args, &stdout, &stderr, envMap()); code != exitValidation {
				t.Fatalf("exit = %d, want validation; stderr=%s", code, stderr.String())
			}
			if !strings.Contains(stderr.String()+stdout.String(), tt.want) {
				t.Errorf("output lacks %q: %s %s", tt.want, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRun_ModelsCatalogFitAndBenchmark(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.RequestURI())
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/model-catalog"):
			_ = json.NewEncoder(w).Encode(apiclient.LocalModelCatalog{VerifiedOn: "2026-10-16", Note: "Provisional.", Models: []apiclient.LocalModelCard{{
				LocalModel: apiclient.LocalModel{ID: "granite-4.1-3b", Tier: "default", DownloadBytes: 2099501664, MinRAMMiB: 3200, License: "Apache-2.0"},
				Fit:        apiclient.CPUFit{Verdict: "fits_comfortably", TokensPerSecLo: 1.9, TokensPerSecHi: 5.3},
			}}})
		case r.URL.Path == "/api/v1/models/fit":
			var req apiclient.ModelFitRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Compute != "cpu" || req.CatalogID != "granite-4.1-3b" {
				t.Errorf("fit request = %+v", req)
			}
			_ = json.NewEncoder(w).Encode(apiclient.CPUFitReport{CPUFit: apiclient.CPUFit{Verdict: "tight", Reason: "little to spare", CapBytes: 1 << 32, CPUMilli: 2000, MemoryMiB: 4096,
				Arithmetic: "weights 2.0 GiB", TokensPerSecLo: 1, TokensPerSecHi: 4, Note: "estimates"}})
		case strings.HasSuffix(r.URL.Path, "/benchmark"):
			_ = json.NewEncoder(w).Encode(apiclient.ModelBenchmarkResult{GenTPS: 6.6, PromptTPS: 88.4, TTFTMillis: 420, PeakMemoryBytes: 3 << 30, MemoryLimitBytes: 4 << 30, DurationMillis: 9000, Note: "Measured."})
		}
	}))
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"models", "catalog", "--api-url", srv.URL})
	for _, want := range []string{"granite-4.1-3b", "Apache-2.0", "1.9-5.3", "fits_comfortably", "2026-10-16"} {
		if !strings.Contains(out, want) {
			t.Errorf("catalog output missing %q: %s", want, out)
		}
	}
	out, _ = runCLIExpectOK(t, []string{"models", "fit", "--compute", "cpu", "--catalog", "granite-4.1-3b", "--api-url", srv.URL})
	for _, want := range []string{"verdict:   tight", "4096 MiB", "estimate"} {
		if !strings.Contains(out, want) {
			t.Errorf("fit output missing %q: %s", want, out)
		}
	}
	out, _ = runCLIExpectOK(t, []string{"models", "benchmark", "local", "--api-url", srv.URL})
	for _, want := range []string{"6.6 tokens per second (measured)", "420 ms", "3.0 GiB of 4.0 GiB"} {
		if !strings.Contains(out, want) {
			t.Errorf("benchmark output missing %q: %s", want, out)
		}
	}
	if paths[len(paths)-1] != "POST /api/v1/models/local/benchmark" {
		t.Errorf("paths = %v", paths)
	}
}
