package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsFreezeSetAndShow(t *testing.T) {
	var gotMethod, gotBody string
	until := time.Date(2026, 9, 28, 7, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		res := apiclient.DeployFreezeResource{Windows: []apiclient.FreezeWindowResource{{Cron: "0 17 * * 5", Duration: "64h0m0s", Timezone: "Europe/Berlin", Reason: "weekend"}}}
		res.Status.Frozen, res.Status.Until = true, &until
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "freeze", "set", "web", "--cron", "0 17 * * 5", "--duration", "64h", "--timezone", "Europe/Berlin", "--reason", "weekend", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || !strings.Contains(gotBody, `"cron":"0 17 * * 5"`) || !strings.Contains(gotBody, `"duration":"64h"`) {
		t.Fatalf("request %s %s", gotMethod, gotBody)
	}
	if !strings.Contains(stdout, "FROZEN until 2026-09-28T07:00:00Z") {
		t.Fatalf("stdout = %q", stdout)
	}

	_, _ = runCLIExpectOK(t, []string{"apps", "freeze", "clear", "web", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || !strings.Contains(gotBody, `"windows":[]`) {
		t.Fatalf("clear sent %s %s", gotMethod, gotBody)
	}
}

func TestRun_AppsDeployPullReresolvesCurrentTag(t *testing.T) {
	var deployBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(apiclient.AppResource{Name: "web", Image: "nginx:latest@sha256:old"})
			return
		}
		b, _ := io.ReadAll(r.Body)
		deployBody = string(b)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.AppResource{Name: "web", Image: "nginx:latest@sha256:new", ImageDigest: "sha256:new"})
	}))
	defer srv.Close()

	_, _ = runCLIExpectOK(t, []string{"apps", "deploy", "web", "--pull", "--api-url", srv.URL})
	if !strings.Contains(deployBody, `"image":"nginx:latest"`) || !strings.Contains(deployBody, `"pull":true`) {
		t.Fatalf("deploy body = %s", deployBody)
	}
}

func TestShortDigest(t *testing.T) {
	if got := shortDigest("sha256:0123456789abcdef", "Resolved"); got != "0123456789ab" {
		t.Fatalf("got %q", got)
	}
	if got := shortDigest("sha256:abc", "PullFailedUsingCached"); got != "abc (PullFailedUsingCached)" {
		t.Fatalf("got %q", got)
	}
	if got := shortDigest("", ""); got != "-" {
		t.Fatalf("got %q", got)
	}
}
