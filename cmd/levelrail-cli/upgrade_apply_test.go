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

func selfUpgradeServer(t *testing.T, plan apiclient.SelfUpgradePlan, started *string, attempts []apiclient.SelfUpgradeAttempt) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/updates/self-upgrade/plan":
			_ = json.NewEncoder(w).Encode(plan)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/updates/self-upgrade":
			var body struct {
				Target string   `json:"target"`
				Ack    []string `json:"ack"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			*started = body.Target + "|" + strings.Join(body.Ack, ",")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(apiclient.SelfUpgradeStarted{Status: "started", TargetVersion: body.Target})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/updates/self-upgrade/attempts":
			_ = json.NewEncoder(w).Encode(apiclient.SelfUpgradeAttempts{Attempts: attempts})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_UpgradePlanApplyAndAttempts(t *testing.T) {
	plan := apiclient.SelfUpgradePlan{
		CurrentVersion: "v1.0.0", TargetVersion: "v1.1.0", NotesAvailable: true, CanApply: true, Steps: []string{"download", "health"},
		Breaking: []apiclient.SelfUpgradeBreaking{{ID: "ports", Version: "v1.1.0", Summary: "ingress ports move", RequiresAck: true}},
	}
	attempts := []apiclient.SelfUpgradeAttempt{{
		ID: "su-1", FromVersion: "v1.0.0", ToVersion: "v1.1.0", Outcome: "rolled_back", Initiator: "tester", Error: "did not become healthy",
		Steps: []apiclient.SelfUpgradeStep{{Name: "health", Status: "failed", Detail: "timeout"}},
	}}
	var started string
	srv := selfUpgradeServer(t, plan, &started, attempts)

	t.Run("plan lists breaking changes", func(t *testing.T) {
		out, _ := runCLIExpectOK(t, []string{"upgrade", "--plan", "--api-url", srv.URL})
		for _, want := range []string{"v1.0.0 -> v1.1.0", "ACK REQUIRED", "ports"} {
			if !strings.Contains(out, want) {
				t.Errorf("stdout missing %q: %s", want, out)
			}
		}
		if started != "" {
			t.Fatalf("plan started an upgrade: %q", started)
		}
	})
	t.Run("apply refuses without acknowledgement", func(t *testing.T) {
		var outBuf, errBuf bytes.Buffer
		code := run("levelrail-cli-test", []string{"upgrade", "--apply", "--no-wait", "--api-url", srv.URL}, &outBuf, &errBuf, envMap())
		if code == exitOK {
			t.Fatalf("exit = %d, want failure (stdout=%s)", code, outBuf.String())
		}
		if !strings.Contains(outBuf.String()+errBuf.String(), "--ack-breaking ports") {
			t.Errorf("no acknowledgement hint: %s %s", outBuf.String(), errBuf.String())
		}
		if started != "" {
			t.Fatalf("upgrade started without acknowledgement: %q", started)
		}
	})
	t.Run("apply with acknowledgement starts", func(t *testing.T) {
		runCLIExpectOK(t, []string{"upgrade", "--apply", "--no-wait", "--ack-breaking", "ports", "--api-url", srv.URL})
		if started != "v1.1.0|ports" {
			t.Fatalf("started = %q", started)
		}
	})
	t.Run("attempts show the timeline", func(t *testing.T) {
		out, _ := runCLIExpectOK(t, []string{"upgrade", "--attempts", "--api-url", srv.URL})
		for _, want := range []string{"rolled_back", "[failed] health timeout", "did not become healthy"} {
			if !strings.Contains(out, want) {
				t.Errorf("stdout missing %q: %s", want, out)
			}
		}
	})
}
