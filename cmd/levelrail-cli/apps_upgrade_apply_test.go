package main

import (
	"bytes"
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
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(apiclient.TemplateUpgradePlan{AppID: "web", HasChanges: true})
			return
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.TemplateUpgradeRun{AppID: "web", State: "watching", FromRevision: 1, ToRevision: 2})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "upgrade", "web", "--yes", "--no-wait", "--api-url", srv.URL})
	if !strings.Contains(gotBody, `"accept_risk":true`) || !strings.Contains(stdout, "watching") {
		t.Fatalf("body %q stdout %q", gotBody, stdout)
	}
}

func TestRun_AppsUpgradeYesNeedsCapabilityAcceptance(t *testing.T) {
	var posted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(apiclient.TemplateUpgradePlan{AppID: "web", HasChanges: true, AddedCapabilities: []string{"web:NET_ADMIN"}})
			return
		}
		b, _ := io.ReadAll(r.Body)
		posted = string(b)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.TemplateUpgradeRun{AppID: "web", State: "watching"})
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	if code := run("levelrail-cli-test", []string{"apps", "upgrade", "web", "--yes", "--no-wait", "--api-url", srv.URL}, &out, &errOut, envMap()); code == exitOK || posted != "" {
		t.Fatalf("without --accept-capabilities: exit %d, posted %q; want a refusal and no POST", code, posted)
	}
	_, _ = runCLIExpectOK(t, []string{"apps", "upgrade", "web", "--yes", "--no-wait", "--accept-capabilities", "--api-url", srv.URL})
	if !strings.Contains(posted, `"accept_capabilities":["web:NET_ADMIN"]`) {
		t.Fatalf("posted %q, want the plan's capabilities echoed", posted)
	}
}
