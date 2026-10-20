package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsDeployNotifyTargetsCreate(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody createDeployNotifyTargetRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(deployNotifyTargetResource{
			ID: "dnt_1", ChannelID: gotBody.ChannelID, NotifyKind: "slack", Enabled: gotBody.Enabled,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"apps", "deploy-notify-targets", "create", "web",
		"--channel-id", "ch_1",
		"--api-url", srv.URL, "--json",
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/deploy-notify-targets" {
		t.Errorf("path = %q, want /api/v1/apps/web/deploy-notify-targets", gotPath)
	}
	if gotBody.ChannelID != "ch_1" || !gotBody.Enabled {
		t.Errorf("request body = %+v, want channel_id ch_1 enabled", gotBody)
	}
	if !strings.Contains(stdout.String(), `"id": "dnt_1"`) {
		t.Errorf("stdout = %q, want the created target as JSON", stdout.String())
	}
}

func TestRun_AppsDeployNotifyTargetsCreate_Disabled(t *testing.T) {
	var gotBody createDeployNotifyTargetRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(deployNotifyTargetResource{ID: "dnt_2", ChannelID: gotBody.ChannelID, Enabled: gotBody.Enabled})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"apps", "deploy-notify-targets", "create", "web",
		"--channel-id", "ch_2", "--disabled",
		"--api-url", srv.URL,
	})
	if gotBody.Enabled {
		t.Errorf("request body Enabled = true, want false when --disabled is set")
	}
	if !strings.Contains(stdout, `deploy notify target "dnt_2" created for app "web" (channel "ch_2")`) {
		t.Errorf("stdout = %q, want a creation confirmation", stdout)
	}
}

func TestRun_AppsDeployNotifyTargetsCreate_MissingChannelID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-notify-targets", "create", "web"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--channel-id is required") {
		t.Errorf("stderr = %q, want a missing --channel-id error", stderr.String())
	}
}

func TestRun_AppsDeployNotifyTargetsList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []deployNotifyTargetResource{
		{ID: "dnt_1", ChannelID: "ch_1", NotifyKind: "slack", Enabled: true},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "deploy-notify-targets", "list", "web", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/deploy-notify-targets" {
		t.Errorf("path = %q, want /api/v1/apps/web/deploy-notify-targets", gotPath)
	}
	if !strings.Contains(stdout, "dnt_1") || !strings.Contains(stdout, "slack") {
		t.Errorf("stdout = %q, want the target listed", stdout)
	}
}

func TestRun_AppsDeployNotifyTargetsList_Empty(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []deployNotifyTargetResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "deploy-notify-targets", "list", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no deploy notify targets") {
		t.Errorf("stdout = %q, want the empty-list message", stdout)
	}
}

func TestRun_AppsDeployNotifyTargetsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "deploy-notify-targets", "delete", "web", "dnt_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/apps/web/deploy-notify-targets/dnt_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/apps/web/deploy-notify-targets/dnt_1", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `deploy notify target "dnt_1" deleted`) {
		t.Errorf("stdout = %q, want a deletion confirmation", stdout)
	}
}
