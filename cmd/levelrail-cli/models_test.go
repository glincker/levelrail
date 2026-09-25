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

func sampleModel() apiclient.ModelResource {
	return apiclient.ModelResource{
		Name: "chat", Engine: "ollama", Model: "llama3.1:8b", APIKeyPrefix: "lr-abcd", EndpointURL: "https://chat.example.com/v1",
		Status: apiclient.ModelStatusResource{Reason: "Downloading", Message: "downloading llama3.1:8b: 12%"},
	}
}

func TestRun_ModelsList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []apiclient.ModelResource{sampleModel()})
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"models", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/models" {
		t.Errorf("path = %q", gotPath)
	}
	for _, want := range []string{"chat", "ollama", "Downloading", "12%", "local"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}
}

func TestRun_ModelsList_Empty(t *testing.T) {
	srv := newListEchoServer(t, nil, []apiclient.ModelResource{})
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"models", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no models deployed") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_ModelsGet(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, sampleModel())
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"models", "get", "chat", "--api-url", srv.URL})
	if gotPath != "/api/v1/models/chat" || !strings.Contains(stdout, "https://chat.example.com/v1") || !strings.Contains(stdout, "lr-abcd") {
		t.Errorf("path=%q stdout=%q", gotPath, stdout)
	}
}

func TestRun_ModelsDeploy(t *testing.T) {
	var got apiclient.CreateModelRequest
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.CreateModelResponse{ModelResource: sampleModel(), APIKey: "lr-secretsecret"}) //nolint:gosec // test fixture, not a credential
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	env := func(k string) (string, bool) {
		if k == "HF_TOKEN" {
			return " hf_abc ", true
		}
		return "", false
	}
	args := []string{"models", "deploy", "--name", "chat", "--engine", "vllm", "--model", "org/m", "--gpus", "2", "--context", "4096", "--hf-token-from-env", "--api-url", srv.URL}
	if code := run("levelrail-cli-test", args, &stdout, &stderr, env); code != exitOK {
		t.Fatalf("exit = %d stderr=%s", code, stderr.String())
	}
	if gotMethod != http.MethodPost || got.Name != "chat" || got.Engine != "vllm" || got.GPUCount != 2 || got.ContextLength != 4096 || got.HFToken != "hf_abc" {
		t.Errorf("request = %s %+v", gotMethod, got)
	}
	if !strings.Contains(stdout.String(), "lr-secretsecret") || !strings.Contains(stdout.String(), "not shown again") {
		t.Errorf("stdout = %q, want the one-time key and warning", stdout.String())
	}
}

func TestRun_ModelsDeploy_Validation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  func(string) (string, bool)
		want string
	}{
		{"missing flags", []string{"models", "deploy", "--name", "x"}, envMap(), "required"},
		{"bad gpus", []string{"models", "deploy", "--name", "x", "--engine", "ollama", "--model", "a", "--gpus", "lots"}, envMap(), "--gpus"},
		{"hf env unset", []string{"models", "deploy", "--name", "x", "--engine", "vllm", "--model", "o/m", "--hf-token-from-env"}, envMap(), "HF_TOKEN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run("levelrail-cli-test", tt.args, &stdout, &stderr, tt.env); code != exitValidation {
				t.Fatalf("exit = %d, want %d", code, exitValidation)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestRun_ModelsDeploy_ServerError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"engine \"tgi\" is not supported"}`)
	stderr := runCLIExpectAPIError(t, []string{"models", "deploy", "--name", "x", "--engine", "tgi", "--model", "a", "--api-url", srv.URL})
	if !strings.Contains(stderr, "not supported") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRun_ModelsActions(t *testing.T) {
	tests := []struct {
		verb, method, path, want string
	}{
		{"delete", http.MethodDelete, "/api/v1/models/chat", `model "chat" deleted`},
		{"restart", http.MethodPost, "/api/v1/models/chat/restart", `model "chat" restarting`},
	}
	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			srv, gotPath, gotMethod := newNoContentEchoServer(t)
			defer srv.Close()
			stdout, _ := runCLIExpectOK(t, []string{"models", tt.verb, "chat", "--api-url", srv.URL})
			if *gotMethod != tt.method || *gotPath != tt.path || !strings.Contains(stdout, tt.want) {
				t.Errorf("request = %s %s stdout = %q", *gotMethod, *gotPath, stdout)
			}
		})
	}
	testMetricsNoName(t, []string{"models", "delete"})
}

func TestRun_ModelsRotateKey(t *testing.T) {
	var gotMethod, gotPath string
	srv := newEchoServer(t, &gotMethod, &gotPath, apiclient.ModelAPIKeyResource{APIKey: "lr-newnewnew"}) //nolint:gosec // test fixture, not a credential
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"models", "rotate-key", "chat", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/models/chat/api-key" || !strings.Contains(stdout, "lr-newnewnew") {
		t.Errorf("request = %s %s stdout = %q", gotMethod, gotPath, stdout)
	}
}

func TestRun_ModelsLogs(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{"entries":[{"timestamp":"2026-09-24T10:00:00Z","stream":"stdout","message":"pulling manifest"}]}`))
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"models", "logs", "chat", "--since", "1h", "-q", "pull", "--api-url", srv.URL})
	if gotPath != "/api/v1/models/chat/logs" || !strings.Contains(gotQuery, "q=pull") || !strings.Contains(stdout, "pulling manifest") {
		t.Errorf("path=%q query=%q stdout=%q", gotPath, gotQuery, stdout)
	}
	stderr := runCLIExpectValidationError(t, []string{"models", "logs", "chat", "--follow", "--tail", "5", "--api-url", srv.URL})
	if !strings.Contains(stderr, "--follow") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRun_ModelsGPUs(t *testing.T) {
	nodes := []apiclient.GPUNodeResource{{
		Name: "gpu-1", Present: true, GPUCount: 2, TotalVRAMMiB: 81920, UsedVRAMMiB: 2048, DriverVersion: "550.1", ModelCount: 1,
		Hint: "Install nvidia-container-toolkit",
	}}
	srv := newListEchoServer(t, nil, nodes)
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"models", "gpus", "--api-url", srv.URL})
	for _, want := range []string{"gpu-1", "2048/81920 MiB", "nvidia runtime missing", "Install nvidia-container-toolkit"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}
	empty := newListEchoServer(t, nil, []apiclient.GPUNodeResource{})
	defer empty.Close()
	stdout, _ = runCLIExpectOK(t, []string{"models", "gpus", "--api-url", empty.URL})
	if !strings.Contains(stdout, "no node has reported") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_ModelsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run("levelrail-cli-test", []string{"models"}, &stdout, &stderr, envMap()); code != exitUsage {
		t.Errorf("exit = %d, want usage", code)
	}
	if code := run("levelrail-cli-test", []string{"models", "bogus"}, &stdout, &stderr, envMap()); code != exitUsage {
		t.Errorf("unknown verb exit = %d", code)
	}
	if out, _ := runCLIExpectOK(t, []string{"models", "help"}); !strings.Contains(out, "models deploy") {
		t.Errorf("help = %q", out)
	}
}
