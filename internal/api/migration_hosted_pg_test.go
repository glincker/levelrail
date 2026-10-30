package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const hostedTestPassword = "s3cr3tPw" //nolint:gosec // fake credential for redaction tests

type hostedCopyRuntime struct{ copyRuntime }

func (h *hostedCopyRuntime) Exec(ctx context.Context, c string, cmd []string) (io.ReadCloser, error) {
	switch cmd[len(cmd)-1] {
	case datamigrate.HostedProbeCommand()[2]:
		return io.NopCloser(strings.NewReader("version_num|160004\nversion|16.4\nsize|1048576\ntables|1\n")), nil
	case datamigrate.DiskFreeCommand()[2]:
		return io.NopCloser(strings.NewReader("free|107374182400\n")), nil
	}
	return h.copyRuntime.Exec(ctx, c, cmd)
}

func hostedBody(frozen bool) string {
	b, _ := json.Marshal(hostedImportRequest{URL: "postgres://app:" + hostedTestPassword + "@db.example.com:5432/d1", WritesFrozen: frozen})
	return string(b)
}

func TestHostedImportRoutes(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{digest: "sha256:abc"})
	ctx := context.Background()
	for _, d := range []store.DesiredDatabase{{Name: "main", Engine: "postgres", Version: "16"}, {Name: "cache", Engine: "redis", Version: "7"}} {
		if err := db.SaveDesiredDatabase(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	rt.execRuntime = func(string) (docker.Runtime, error) { return &hostedCopyRuntime{}, nil }
	base := "/api/v1/imports/platform/databases/"

	t.Run("preflight lists findings and never echoes the url", func(t *testing.T) {
		rec := serve(rt, authedRequest(t, cookie, http.MethodPost, base+"main/hosted/preflight", hostedBody(false)))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), hostedTestPassword) {
			t.Fatal("preflight echoes the source password")
		}
		var out hostedPreflightResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		ids := map[string]string{}
		for _, c := range out.Checks {
			ids[c.ID] = c.Severity
		}
		if !out.Blocked || ids["writes_frozen"] != datamigrate.SeverityBlock || !strings.Contains(out.Source, "db.example.com") {
			t.Errorf("response = %+v", out)
		}
	})
	t.Run("rejects a non postgres target and a bad url without echoing it", func(t *testing.T) {
		if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, base+"cache/hosted/preflight", hostedBody(true))); rec.Code != http.StatusBadRequest {
			t.Errorf("redis target = %d, want 400", rec.Code)
		}
		bad := `{"url":"mysql://u:` + hostedTestPassword + `@h/d","writes_frozen":true}`
		rec := serve(rt, authedRequest(t, cookie, http.MethodPost, base+"main/hosted/run", bad))
		if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), hostedTestPassword) {
			t.Errorf("bad url = %d: %s", rec.Code, rec.Body)
		}
		if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, base+"nope/hosted/run", hostedBody(true))); rec.Code != http.StatusNotFound {
			t.Errorf("unknown database = %d, want 404", rec.Code)
		}
	})
	t.Run("run refuses without writes_frozen", func(t *testing.T) {
		rec := serve(rt, authedRequest(t, cookie, http.MethodPost, base+"main/hosted/run", hostedBody(false)))
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "not frozen") {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body)
		}
	})
	t.Run("run moves and verifies", func(t *testing.T) {
		rec := serve(rt, authedRequest(t, cookie, http.MethodPost, base+"main/hosted/run", hostedBody(true)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("run = %d: %s", rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), hostedTestPassword) {
			t.Fatal("run response echoes the source password")
		}
		deadline := time.Now().Add(5 * time.Second)
		var res dataImportResource
		for time.Now().Before(deadline) {
			get := serve(rt, authedRequest(t, cookie, http.MethodGet, base+"main", ""))
			if strings.Contains(get.Body.String(), hostedTestPassword) {
				t.Fatal("status echoes the source password")
			}
			_ = json.Unmarshal(get.Body.Bytes(), &res)
			if res.Status != datamigrate.StatusCopying {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if res.Status != datamigrate.StatusVerified || res.SourceHost != "db.example.com" {
			t.Fatalf("final status = %+v", res)
		}
	})
}
