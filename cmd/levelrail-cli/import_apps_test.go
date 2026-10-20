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

func appImportAPI(t *testing.T) (*httptest.Server, *[]string, *apiclient.AppImportRequest) {
	t.Helper()
	var calls []string
	var got apiclient.AppImportRequest
	view := apiclient.AppImportView{ID: "appimp-1", Platform: "coolify", States: map[string]int{},
		Suggested: []apiclient.AppImportMapping{{From: "pguuid", To: "pg-main"}},
		Items: []apiclient.AppImportItem{{SourceID: "a1", Name: "web", State: "planned", Selected: true,
			Entry: apiclient.AppImportEntry{Name: "web", Verdict: "ready-with-notes", Source: "git", MapsTo: "dockerfile",
				Findings: []apiclient.AppImportFinding{{Reason: "1 persistent volume start empty here", Next: "copy it"}}}}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/plan") || strings.HasSuffix(r.URL.Path, "/sessions") && r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &got
}

func TestImportAppsPlanSendsTokenInBodyOnly(t *testing.T) {
	srv, calls, got := appImportAPI(t)
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "apps", "--from", "coolify", "--url", "https://c.example.com", "--plan", "--map", "old=new", "--only", "web", "--api-url", srv.URL},
		&out, &errb, importEnv(map[string]string{envImportSourceToken: "tok-env", "APP_API_TOKEN": "cp"}))
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if len(*calls) != 1 || (*calls)[0] != "POST /api/v1/migration/apps/plan" {
		t.Errorf("calls: %v", *calls)
	}
	if got.Token != "tok-env" || len(got.Mappings) != 1 || got.Mappings[0].To != "new" || len(got.Only) != 1 {
		t.Errorf("request: %+v", got)
	}
	for _, want := range []string{"web", "ready-with-notes", "next: copy it", "pguuid -> pg-main"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout lacks %q: %s", want, out.String())
		}
	}
	if strings.Contains(out.String()+errb.String(), "tok-env") {
		t.Error("token printed")
	}
}

func TestImportAppsApplyStagesThenSuggestsVerify(t *testing.T) {
	srv, calls, _ := appImportAPI(t)
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "apps", "--url", "https://c.example.com", "--apply", "--accept-suggested-maps", "--api-url", srv.URL},
		&out, &errb, importEnv(map[string]string{envImportSourceToken: "tok", "APP_API_TOKEN": "cp"}))
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	want := []string{"POST /api/v1/migration/apps/sessions", "PUT /api/v1/migration/apps/sessions/appimp-1/plan", "POST /api/v1/migration/apps/sessions/appimp-1/stage"}
	for i, w := range want {
		if i >= len(*calls) || (*calls)[len(*calls)-3+i] != w {
			t.Fatalf("calls = %v, want tail %v", *calls, want)
		}
	}
	if !strings.Contains(out.String(), "import apps --session appimp-1 --verify") {
		t.Errorf("no next step: %s", out.String())
	}
}

func TestImportAppsUsageErrors(t *testing.T) {
	cases := [][]string{
		{"import", "apps"},
		{"import", "apps", "--plan", "--apply", "--url", "https://x"},
		{"import", "apps", "--plan"},
		{"import", "apps", "--plan", "--url", "https://x", "--map", "nope"},
	}
	for _, args := range cases {
		var out, errb bytes.Buffer
		if c := run("cli", args, &out, &errb, importEnv(map[string]string{envImportSourceToken: "t"})); c != exitUsage {
			t.Errorf("%v: exit %d, want usage", args, c)
		}
	}
}
