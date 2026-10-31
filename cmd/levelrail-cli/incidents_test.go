package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func incidentsServer(t *testing.T, draft string, calls *[]string, publishBody *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		*calls = append(*calls, key)
		w.Header().Set("Content-Type", "application/json")
		switch key {
		case "GET /api/v1/incidents":
			_, _ = w.Write([]byte(`[{"id":"si_1","title":"Db down","internal_state":"triggered","impact":"none","published":false,"updated_at":"2026-10-29T10:00:00Z"}]`))
		case "GET /api/v1/incidents/si_1":
			_, _ = w.Write([]byte(`{"id":"si_1","title":"Db down","internal_state":"triggered","status":"investigating","impact":"none","published":false,"updated_at":"2026-10-29T10:00:00.5Z",` + draft + `"component_ids":[]}`))
		case "POST /api/v1/incidents/si_1/publish":
			b, _ := io.ReadAll(r.Body)
			*publishBody = string(b)
			_, _ = w.Write([]byte(`{"id":"si_1","published":true,"published_at":"2026-10-29T10:01:00Z"}`))
		case "PUT /api/v1/incidents/si_1/draft":
			b, _ := io.ReadAll(r.Body)
			*publishBody = string(b)
			_, _ = w.Write([]byte(`{"id":"si_1","internal_state":"triggered"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestIncidentsCLI_PublishRefusesEmptyDraftAndUsesFetchedStamp(t *testing.T) {
	var calls []string
	var body string
	srv := incidentsServer(t, "", &calls, &body)
	var out, errb bytes.Buffer
	if code := run("cli", []string{"incidents", "publish", "si_1", "--api-url", srv.URL}, &out, &errb, envMap()); code == exitOK {
		t.Fatal("empty draft was published")
	}
	if !strings.Contains(errb.String()+out.String(), "draft is empty") {
		t.Errorf("no clear message: %s %s", errb.String(), out.String())
	}
	for _, c := range calls {
		if strings.Contains(c, "publish") {
			t.Errorf("publish endpoint called for an empty draft: %v", calls)
		}
	}
	srv.Close()

	calls, body = nil, ""
	srv = incidentsServer(t, `"draft":{"title":"T","body":"B"},`, &calls, &body)
	defer srv.Close()
	out.Reset()
	errb.Reset()
	if code := run("cli", []string{"incidents", "publish", "si_1", "--api-url", srv.URL}, &out, &errb, envMap()); code != exitOK {
		t.Fatalf("publish exit %d: %s", code, errb.String())
	}
	if !strings.Contains(body, `"updated_at":"2026-10-29T10:00:00.5Z"`) {
		t.Errorf("publish body = %s, want the fetched updated_at", body)
	}
	out.Reset()
	if code := run("cli", []string{"incidents", "publish", "si_1", "--updated-at", "2026-10-29T09:00:00Z", "--api-url", srv.URL}, &out, &errb, envMap()); code == exitOK {
		t.Error("stale --updated-at was accepted")
	}
}

func TestIncidentsCLI_ListAndDraftComponents(t *testing.T) {
	var calls []string
	var body string
	srv := incidentsServer(t, "", &calls, &body)
	defer srv.Close()
	var out, errb bytes.Buffer
	if code := run("cli", []string{"incidents", "list", "--api-url", srv.URL}, &out, &errb, envMap()); code != exitOK || !strings.Contains(out.String(), "Db down") {
		t.Fatalf("list exit %d out %s err %s", code, out.String(), errb.String())
	}
	args := []string{"incidents", "draft", "si_1", "--title", "T", "--body", "B", "--impact", "major", "--components", "a,b", "--api-url", srv.URL}
	if code := run("cli", args, &out, &errb, envMap()); code != exitOK {
		t.Fatalf("draft exit %d: %s", code, errb.String())
	}
	if !strings.Contains(body, `"impact":"major"`) || !strings.Contains(body, `"component_ids":["a","b"]`) {
		t.Errorf("draft body = %s", body)
	}
}
