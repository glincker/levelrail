package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sourceFixtureDir = "../platformimport/testdata"

// sourceFixtureServer serves another platform's recorded API shapes, read only, with fake credentials.
func sourceFixtureServer(t *testing.T, platform string) *httptest.Server {
	t.Helper()
	routes := map[string]string{}
	header, value := "", ""
	switch platform {
	case "dokploy":
		routes = map[string]string{
			"/api/project.all":                          "dokploy/project_all.json",
			"/api/application.one?applicationId=a-gh":   "dokploy/app_gh.json",
			"/api/application.one?applicationId=a-img":  "dokploy/app_img.json",
			"/api/application.one?applicationId=a-drop": "dokploy/app_drop.json",
		}
		header, value = "x-api-key", "key-fixture"
	case "caprover":
		routes = map[string]string{"/api/v2/user/apps/appDefinitions": "caprover/app_definitions.json"}
		header, value = "x-captain-auth", "session-fixture"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v2/login" {
			_, _ = w.Write([]byte(`{"status":100,"description":"ok","data":{"token":"session-fixture"}}`))
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("non-GET request to the source: %s %s", r.Method, r.URL.Path)
			http.Error(w, "read only", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get(header) != value {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		key := r.URL.Path
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		f, ok := routes[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		raw, err := os.ReadFile(filepath.Join(sourceFixtureDir, f)) //nolint:gosec // fixed fixture paths
		if err != nil {
			t.Error(err)
		}
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestEverySourceConnectsAndStartsAJob(t *testing.T) {
	for platform, token := range map[string]string{"dokploy": "key-fixture", "caprover": "pw-login"} {
		t.Run(platform, func(t *testing.T) {
			h := newAppImportHarness(t)
			src := sourceFixtureServer(t, platform)
			body := `{"platform":"` + platform + `","url":"` + src.URL + `","token":"` + token + `","allow_loopback":true}`

			var res migrationTestResult
			h.decode(h.call(http.MethodPost, appImportBase+"/test", body), &res)
			if !res.OK || res.Apps == 0 {
				t.Fatalf("%s connection test: %+v", platform, res)
			}
			if platform == "dokploy" {
				var denied migrationTestResult
				h.decode(h.call(http.MethodPost, appImportBase+"/test", strings.Replace(body, token, "wrong-fake-token", 1)), &denied)
				if denied.OK || denied.Failure != testCredentials {
					t.Fatalf("dokploy with a wrong key should fail as credentials: %+v", denied)
				}
			}

			rec := h.call(http.MethodPost, appImportBase+"/sessions", body)
			if rec.Code != http.StatusCreated {
				t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), token) {
				t.Fatal("the source credential was echoed back")
			}
			p := h.progress(h.view(rec).ID)
			if p.Platform != platform || p.Total == 0 || !p.Connected {
				t.Fatalf("progress for %s: %+v", platform, p)
			}
		})
	}
}

func TestExitReportNamesTheSourceItIsLeaving(t *testing.T) {
	h := newAppImportHarness(t)
	src := sourceFixtureServer(t, "dokploy")
	body := `{"platform":"dokploy","url":"` + src.URL + `","token":"key-fixture","allow_loopback":true}`

	rec := h.call(http.MethodPost, "/api/v1/migration/exit-report", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("exit report: %d %s", rec.Code, rec.Body.String())
	}
	out := rec.Body.String()
	if !strings.Contains(out, "Back up Dokploy itself") || strings.Contains(out, "Coolify") {
		t.Fatalf("the checklist should name Dokploy and no other platform: %s", out)
	}
	if strings.Contains(out, "key-fixture") {
		t.Fatal("the exit report echoed the credential")
	}

	rec = h.call(http.MethodPost, "/api/v1/migration/exit-report", strings.Replace(body, "dokploy", "compose", 2))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a pasted-file platform has no exit report, got %d", rec.Code)
	}

	rec = h.call(http.MethodPost, appImportBase+"/sessions", body)
	id := h.view(rec).ID
	rec = h.call(http.MethodGet, appImportBase+"/sessions/"+id+"/exit-report", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Retire Dokploy") {
		t.Fatalf("session exit report: %d %s", rec.Code, rec.Body.String())
	}
}
