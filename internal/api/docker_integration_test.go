package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerlint"
)

func TestDockerLintEndpoint(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/docker/lint", body))
		return rec
	}

	if rec := do(`{"dockerfile":"  "}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty dockerfile = %d, want 400", rec.Code)
	}
	rec := do(`{"dockerfile":"FROM node\nCMD [\"x\"]\n","has_dockerignore":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("lint = %d %s", rec.Code, rec.Body.String())
	}
	var res dockerlint.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) == 0 || !strings.Contains(res.FixedDockerfile, "USER "+dockerlint.NonRootUser) {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestDockerHealthWithoutClient(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/docker/health", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("health = %d", rec.Code)
	}
	var res dockerHealthResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Reachable {
		t.Fatalf("no client configured must not report reachable: %+v", res)
	}
}

type hungDockerPinger struct{ since time.Time }

func (h *hungDockerPinger) Ping(context.Context) error {
	return fmt.Errorf("ping: %w", docker.ErrDaemonUnresponsive)
}

func (h *hungDockerPinger) UnresponsiveSince() time.Time { return h.since }

func TestDockerHealthReportsUnresponsive(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	WithDockerPinger(&hungDockerPinger{since: time.Now().Add(-time.Minute)})(rt)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/docker/health", ""))
	var res dockerHealthResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Reachable || !res.Unresponsive || res.UnresponsiveSince == nil {
		t.Fatalf("want unresponsive with since set: %+v", res)
	}
	if len(res.Checks) == 0 || res.Checks[0].Message != dockerNotRespondingMessage {
		t.Fatalf("check message = %+v", res.Checks)
	}
}

func TestImageStatusUnknownApp(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/nope/image-status", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("image-status = %d, want 404", rec.Code)
	}
}
