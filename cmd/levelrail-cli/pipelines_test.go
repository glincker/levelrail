package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func writePipelineFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ci.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRun_PipelinesValidate(t *testing.T) {
	good := writePipelineFile(t, "version: 1\nname: ci\njobs:\n  t:\n    image: alpine\n    steps:\n      - run: echo hi\n")
	var stdout, stderr bytes.Buffer
	if got := run("cli", []string{"pipelines", "validate", good}, &stdout, &stderr, envMap()); got != exitOK || !strings.Contains(stdout.String(), "is valid") {
		t.Fatalf("good file: exit %d stdout %q stderr %q", got, stdout.String(), stderr.String())
	}

	bad := writePipelineFile(t, "version: 1\njobs:\n  t:\n    needs: [nope]\n    steps:\n      - run: echo hi\n")
	stdout.Reset()
	stderr.Reset()
	if got := run("cli", []string{"pipelines", "validate", bad}, &stdout, &stderr, envMap()); got != exitValidation || !strings.Contains(stderr.String(), "needs unknown job") {
		t.Fatalf("bad file: exit %d stderr %q", got, stderr.String())
	}
}

func TestRun_PipelinesRunAndApprove(t *testing.T) {
	var startBody apiclient.PipelineStartRequest
	var decided []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps/web/pipelines/ci/runs":
			_ = json.NewDecoder(r.Body).Decode(&startBody)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(apiclient.PipelineRunResource{ID: "r1", Number: 4, Status: "queued"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/pipeline-runs/r1":
			_ = json.NewEncoder(w).Encode(apiclient.PipelineRunResource{ID: "r1", Number: 4, Status: "running", Approvals: []apiclient.PipelineApproval{
				{ID: 7, Job: "ship", RequiredAbility: "deploy"}, {ID: 8, Job: "old", Decision: "approved"},
			}})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/approvals/"):
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			decided = append(decided, r.URL.Path+"|"+body["decision"]+"|"+body["comment"])
			_ = json.NewEncoder(w).Encode(map[string]string{"decision": body["decision"]})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("cli", []string{"pipelines", "run", "web", "ci", "--api-url", srv.URL, "--ref", "refs/heads/main", "--input", "env=prod", "--json"}, &stdout, &stderr, envMap())
	if got != exitOK || startBody.Ref != "refs/heads/main" || startBody.Inputs["env"] != "prod" {
		t.Fatalf("run: exit %d body %+v stderr %q", got, startBody, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id": "r1"`) {
		t.Errorf("stdout = %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	got = run("cli", []string{"pipelines", "approve", "web", "r1", "--api-url", srv.URL, "--reject", "--comment", "nope"}, &stdout, &stderr, envMap())
	if got != exitOK || len(decided) != 1 || decided[0] != "/api/v1/apps/web/pipeline-runs/r1/approvals/7|rejected|nope" {
		t.Fatalf("approve: exit %d decided %v stderr %q", got, decided, stderr.String())
	}
}

func TestRun_PipelinesUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("cli", []string{"pipelines", "bogus"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want usage", got)
	}
}

func TestRun_PipelinesSaveDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".brand", "pipelines")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	body := "version: 1\nname: ci\njobs:\n  t:\n    image: alpine\n    steps:\n      - run: echo hi\n"
	if err := os.WriteFile(filepath.Join(dir, "ci.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	var created []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/brand":
			_, _ = w.Write([]byte(`{"ShortName":"brand"}`))
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"pipeline not found"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps/web/pipelines":
			var req apiclient.PipelineSaveRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			created = append(created, req.Name)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(apiclient.PipelineResource{Name: req.Name})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("cli", []string{"pipelines", "save", "web", root, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK || len(created) != 1 || created[0] != "ci" {
		t.Fatalf("exit %d created %v stderr %q", got, created, stderr.String())
	}
}
