package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsUpgradePlanIsReadOnly(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.TemplateUpgradePlan{AppID: "web", FromVersion: "1", ToVersion: "2", Risk: "low", HasChanges: true})
	}))
	defer srv.Close()

	for _, verb := range []string{"upgrade-plan", "upgrade"} {
		_, _ = runCLIExpectOK(t, []string{"apps", verb, "web", "--api-url", srv.URL})
	}
	for _, m := range methods {
		if m != "GET /api/v1/apps/web/template-upgrade" {
			t.Fatalf("unexpected request %q", m)
		}
	}
}

func TestRun_AppsUpgradeYesAppliesWithoutWait(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.TemplateUpgradeRun{AppID: "web", State: "watching", FromRevision: 1, ToRevision: 2})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "upgrade", "web", "--yes", "--no-wait", "--api-url", srv.URL})
	if !strings.Contains(gotBody, `"accept_risk":true`) || !strings.Contains(stdout, "watching") {
		t.Fatalf("body %q stdout %q", gotBody, stdout)
	}
}
