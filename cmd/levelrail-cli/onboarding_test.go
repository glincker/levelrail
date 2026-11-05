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

func meOnboardingServer(t *testing.T, got *[]string, gotBody *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = append(*got, r.Method+" "+r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		*gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.MeOnboarding{
			State: "in_progress", CurrentStep: "deploy", FlowVersion: 1, Enabled: true,
			Milestone: "https_on", NextMilestone: "first_domain",
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_OnboardingStatus(t *testing.T) {
	var got []string
	var body string
	srv := meOnboardingServer(t, &got, &body)
	stdout, _ := runCLIExpectOK(t, []string{"onboarding", "status", "--api-url", srv.URL})
	if len(got) != 1 || got[0] != "GET /api/v1/me/onboarding" {
		t.Fatalf("requests = %v", got)
	}
	for _, want := range []string{"State: in_progress", "Current step: deploy", "Milestone: https_on", "Next milestone: first_domain", "Enabled: true"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRun_OnboardingSkip(t *testing.T) {
	var got []string
	var body string
	srv := meOnboardingServer(t, &got, &body)
	runCLIExpectOK(t, []string{"onboarding", "skip", "--api-url", srv.URL})
	if got[0] != "PUT /api/v1/me/onboarding" || !strings.Contains(body, `"state":"dismissed"`) {
		t.Fatalf("request = %v body %q", got, body)
	}
}

func TestRun_OnboardingResetNeedsConfirm(t *testing.T) {
	var got []string
	var body string
	srv := meOnboardingServer(t, &got, &body)
	var out, errb strings.Builder
	if code := run("levelrail-cli-test", []string{"onboarding", "reset", "--api-url", srv.URL}, &out, &errb, envMap()); code != exitUsage {
		t.Fatalf("exit = %d, want usage", code)
	}
	if len(got) != 0 {
		t.Fatalf("reset without --yes reached the API: %v", got)
	}
	runCLIExpectOK(t, []string{"onboarding", "reset", "--yes", "--api-url", srv.URL})
	if got[0] != "POST /api/v1/me/onboarding/reset" {
		t.Fatalf("requests = %v", got)
	}
}
