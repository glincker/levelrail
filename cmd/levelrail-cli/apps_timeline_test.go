package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsTimeline(t *testing.T) {
	var gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.TimelineResponse{
			Items: []apiclient.TimelineItem{
				{ID: "evt_1", At: "2026-09-25T10:00:00Z", Kind: "env_change", Status: "info", Actor: "gagan", Title: "Environment changed: API_URL"},
				{ID: "dpl_a", At: "2026-09-25T09:00:00Z", Kind: "deploy", Status: "succeeded", Actor: "manual", Title: "Deploy to web:2", Detail: "digest abc"},
			},
			NextCursor: "c",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "timeline", "web", "--limit", "5", "--api-url", srv.URL})
	if gotURI != "/api/v1/apps/web/timeline?limit=5" {
		t.Errorf("request = %s", gotURI)
	}
	for _, want := range []string{"env_change", "Environment changed: API_URL", "Deploy to web:2 (digest abc)", "raise --limit"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}
	jsonOut, _ := runCLIExpectOK(t, []string{"apps", "timeline", "web", "--json", "--api-url", srv.URL})
	var got apiclient.TimelineResponse
	if err := json.Unmarshal([]byte(jsonOut), &got); err != nil || len(got.Items) != 2 {
		t.Errorf("json = %q err = %v", jsonOut, err)
	}
}

func TestRun_AppsApply(t *testing.T) {
	for _, pending := range []bool{true, false} {
		var applied bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case servePendingChanges(w, r, pending):
			case strings.HasSuffix(r.URL.Path, "/apply-pending"):
				applied = true
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{}`))
			}
		}))
		stdout, _ := runCLIExpectOK(t, []string{"apps", "apply", "web", "--api-url", srv.URL})
		srv.Close()
		if applied != pending {
			t.Errorf("pending=%v applied=%v", pending, applied)
		}
		if pending && !strings.Contains(stdout, "restarting it (secret API_KEY)") {
			t.Errorf("stdout = %q", stdout)
		}
		if !pending && !strings.Contains(stdout, "nothing pending") {
			t.Errorf("stdout = %q", stdout)
		}
	}
}

func TestRun_AppsStatusShowsPendingAndHold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case servePendingChanges(w, r, true):
		case strings.HasSuffix(r.URL.Path, "/deploys"):
			_, _ = w.Write([]byte(`[]`))
		default:
			_, _ = w.Write([]byte(`{"name":"web","previous_release_held_until":"2099-01-01T00:00:00Z"}`))
		}
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"apps", "status", "web", "--api-url", srv.URL})
	for _, want := range []string{"pending changes: secret API_KEY", "apps apply web", "previous release held until"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}
}

func TestApplyDomainChange(t *testing.T) {
	tests := []struct {
		name    string
		current []string
		verb    string
		wanted  []string
		want    []string
		wantErr bool
	}{
		{"add new", []string{"a.com"}, "add", []string{"b.com"}, []string{"a.com", "b.com"}, false},
		{"add existing is a no-op", []string{"a.com"}, "add", []string{"a.com"}, []string{"a.com"}, false},
		{"remove", []string{"a.com", "b.com"}, "remove", []string{"a.com"}, []string{"b.com"}, false},
		{"remove missing", []string{"a.com"}, "remove", []string{"z.com"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyDomainChange(tt.current, tt.verb, tt.wanted)
			if (err != nil) != tt.wantErr || strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("got %v err %v, want %v err=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestRun_AppsDomainsAddAndConflict(t *testing.T) {
	var putBody map[string]any
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			if status != http.StatusOK {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"domain b.com is already used by app other"}`))
				return
			}
		}
		_, _ = w.Write([]byte(`{"name":"web","image":"nginx","port":80,"domains":["a.com"]}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "domains", "add", "web", "B.com", "--api-url", srv.URL})
	if d, _ := putBody["domains"].([]any); len(d) != 2 || d[1] != "b.com" {
		t.Errorf("PUT domains = %v", putBody["domains"])
	}
	if !strings.Contains(stdout, "a.com, b.com") {
		t.Errorf("stdout = %q", stdout)
	}

	list, _ := runCLIExpectOK(t, []string{"apps", "domains", "list", "web", "--api-url", srv.URL})
	if strings.TrimSpace(list) != "a.com" {
		t.Errorf("list = %q", list)
	}

	status = http.StatusConflict
	var out, errOut strings.Builder
	code := run("levelrail-cli-test", []string{"apps", "domains", "add", "web", "b.com", "--api-url", srv.URL}, &out, &errOut, envMap())
	if code == exitOK || !strings.Contains(errOut.String(), "nothing was changed") || !strings.Contains(errOut.String(), "already used by app other") {
		t.Errorf("conflict: code=%d stderr=%q", code, errOut.String())
	}
}
