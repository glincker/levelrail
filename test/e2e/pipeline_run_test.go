// TestPipelineRun_Live is this package's live proof for "Pipelines":
// internal/pipeline/live_test.go proves the engine against Docker at the
// package level, internal/api/pipelines_test.go proves the handlers
// against a fake runner. Neither drives real YAML through the real HTTP
// API end to end, so this test does: validate, start a run, and check a
// real container's real outcome over a real cookie-authenticated session.
package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2ePipelineAdminUsername = "e2e-pipeline-admin"
	e2ePipelineAdminPassword = "e2e-pipeline-correct-horse" //nolint:gosec // test fixture credential, not a real secret

	e2ePipelineAppName  = "e2e-pipeline-app"
	e2ePipelineName     = "verify"
	e2ePipelineMarker   = "e2e-pipeline-marker-ok"
	e2ePipelineNamePfx  = "e2eplrun"
	e2ePipelineTickRate = 150 * time.Millisecond
)

// e2ePipelineValidYAML runs one job with a single shell step on a real
// alpine container: the simplest real step type the engine has, the same
// kind of step internal/pipeline/live_test.go already proves executes
// against a real daemon.
const e2ePipelineValidYAML = "version: 1\n" +
	"jobs:\n" +
	"  verify:\n" +
	"    image: alpine:3.20\n" +
	"    steps:\n" +
	"      - run: echo " + e2ePipelineMarker + "\n"

// e2ePipelineInvalidYAML omits the required "jobs" key, which the
// embedded JSON Schema (internal/pipeline/schema/pipeline.schema.json)
// requires alongside "version". A deliberately real, deterministic
// schema violation rather than a YAML syntax error.
const e2ePipelineInvalidYAML = "version: 1\n"

func TestPipelineRun_Live(t *testing.T) {
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelPing()
	if err := client.Ping(pingCtx); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	svcStore := openLiveStore(t)

	engineCtx, stopEngine := context.WithCancel(context.Background())
	engine := pipeline.New(pipeline.Config{
		Store:      svcStore,
		NamePrefix: e2ePipelineNamePfx,
		Logger:     discardTestLogger(),
		Runtime:    func(string) (pipeline.Runtime, error) { return client, nil },
	})
	t.Cleanup(func() {
		stopEngine()
		engine.Close()
	})
	go func() { _ = engine.Run(engineCtx, e2ePipelineTickRate) }()

	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, svcStore, api.WithPipelines(svcStore, engine))
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(context.Background(), svcStore, e2ePipelineAdminUsername, e2ePipelineAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	httpClient := loginE2EClient(t, ts.URL, e2ePipelineAdminUsername, e2ePipelineAdminPassword)

	// The app the pipeline is scoped to just needs to exist; the job's
	// own container runs an unrelated alpine image, not this service's.
	if err := svcStore.SaveDesiredService(context.Background(), store.DesiredService{
		Name: e2ePipelineAppName, Image: "alpine:3.20", Port: 8080,
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	t.Run("ValidateRejectsInvalidYAML", func(t *testing.T) {
		status, body := postJSON(t, httpClient, ts.URL+"/api/v1/pipelines/validate", jsonMarshal(t, map[string]string{"yaml": e2ePipelineInvalidYAML}))
		if status != http.StatusOK {
			t.Fatalf("validate: status = %d, want %d, body = %s", status, http.StatusOK, body)
		}
		var resp struct {
			Valid  bool `json:"valid"`
			Issues []struct {
				Message string `json:"message"`
			} `json:"issues"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("decode validate response: %v; body = %s", err, body)
		}
		if resp.Valid {
			t.Fatalf("validate: valid = true for YAML missing the required jobs key; body = %s", body)
		}
		if len(resp.Issues) == 0 {
			t.Fatalf("validate: issues is empty for invalid YAML; body = %s", body)
		}
	})

	t.Run("ValidateAcceptsThenRunExecutesAgainstDocker", func(t *testing.T) {
		status, body := postJSON(t, httpClient, ts.URL+"/api/v1/pipelines/validate", jsonMarshal(t, map[string]string{"yaml": e2ePipelineValidYAML}))
		if status != http.StatusOK {
			t.Fatalf("validate: status = %d, want %d, body = %s", status, http.StatusOK, body)
		}
		var validateResp struct {
			Valid bool `json:"valid"`
		}
		if err := json.Unmarshal([]byte(body), &validateResp); err != nil {
			t.Fatalf("decode validate response: %v; body = %s", err, body)
		}
		if !validateResp.Valid {
			t.Fatalf("validate: valid = false for a real, well-formed pipeline; body = %s", body)
		}

		status, body = postJSON(t, httpClient, ts.URL+"/api/v1/apps/"+e2ePipelineAppName+"/pipelines",
			jsonMarshal(t, map[string]any{"name": e2ePipelineName, "yaml": e2ePipelineValidYAML}))
		if status != http.StatusCreated {
			t.Fatalf("create pipeline: status = %d, want %d, body = %s", status, http.StatusCreated, body)
		}

		status, body = postJSON(t, httpClient, ts.URL+"/api/v1/apps/"+e2ePipelineAppName+"/pipelines/"+e2ePipelineName+"/runs", "{}")
		if status != http.StatusAccepted {
			t.Fatalf("start run: status = %d, want %d, body = %s", status, http.StatusAccepted, body)
		}
		var run struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(body), &run); err != nil {
			t.Fatalf("decode start-run response: %v; body = %s", err, body)
		}
		if run.ID == "" {
			t.Fatalf("start run: response carries no run id; body = %s", body)
		}

		final := pollPipelineRunUntilTerminal(t, httpClient, ts.URL, e2ePipelineAppName, run.ID)
		if final.Status != store.PipelineStatusSucceeded {
			t.Fatalf("run finished with status = %q, reason = %q, want %q", final.Status, final.Reason, store.PipelineStatusSucceeded)
		}

		status, body = httpGet(t, httpClient, ts.URL+"/api/v1/apps/"+e2ePipelineAppName+"/pipeline-runs/"+run.ID+"/logs")
		if status != http.StatusOK {
			t.Fatalf("logs: status = %d, want %d, body = %s", status, http.StatusOK, body)
		}
		if !strings.Contains(body, e2ePipelineMarker) {
			t.Fatalf("logs do not contain the job's own marker %q, so the step did not really run; body = %s", e2ePipelineMarker, body)
		}

		assertNoLeftoverPipelineContainers(t, client)
	})
}

type pipelineRunStatusView struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// pollPipelineRunUntilTerminal polls the real HTTP API, not the store
// directly, so the assertion is on what the API actually reports back to
// a caller, matching the shape every other run-status check in this
// package makes over real HTTP.
func pollPipelineRunUntilTerminal(t *testing.T, httpClient *http.Client, baseURL, app, runID string) pipelineRunStatusView {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		status, body := httpGet(t, httpClient, baseURL+"/api/v1/apps/"+app+"/pipeline-runs/"+runID)
		if status != http.StatusOK {
			t.Fatalf("get run: status = %d, want %d, body = %s", status, http.StatusOK, body)
		}
		var run pipelineRunStatusView
		if err := json.Unmarshal([]byte(body), &run); err != nil {
			t.Fatalf("decode run response: %v; body = %s", err, body)
		}
		if store.IsPipelineTerminal(run.Status) {
			return run
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for run %s to finish; last status = %q, body = %s", runID, run.Status, body)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// assertNoLeftoverPipelineContainers proves the engine cleaned up after
// itself, the same no-containers-left-behind check
// internal/pipeline/live_test.go already makes at the package level.
func assertNoLeftoverPipelineContainers(t *testing.T, client docker.Runtime) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	left, err := client.ListByPrefix(ctx, e2ePipelineNamePfx+"-pl-")
	if err != nil {
		t.Fatalf("ListByPrefix() error = %v", err)
	}
	if len(left) != 0 {
		t.Errorf("containers left behind after the run finished: %+v", left)
	}
}

func jsonMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(b)
}

// httpGet issues a real GET through client and returns the real status
// code and response body, the GET counterpart to postJSON
// (live_api_helpers.go) that every POST-only live test in this package
// already uses.
func httpGet(t *testing.T, client *http.Client, url string) (status int, respBody string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil) //nolint:noctx // test helper, url is loopback-only
	if err != nil {
		t.Fatalf("NewRequest(GET %s) error = %v", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("closing response body: %v", closeErr)
		}
	}()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	return resp.StatusCode, string(b)
}
