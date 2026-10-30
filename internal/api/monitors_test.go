package api

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/monitor"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

func TestMonitors_OffReturns404(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/monitors"},
		{http.MethodPost, "/api/v1/monitors"},
		{http.MethodDelete, "/api/v1/monitors/mon_x"},
		{http.MethodGet, "/api/v1/monitors/mon_x/results"},
	} {
		if rec := apiDo(t, rt, cookie, c.method, c.path, `{}`); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", c.method, c.path, rec.Code)
		}
	}
}

func TestMonitors_CreateListResultsDelete(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "false")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	cfg, err := monitor.ConfigFromEnv(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	rt.SetMonitors(monitor.New(monitor.Options{Config: cfg, Store: db, Logger: slog.New(slog.NewTextHandler(discardWriter{}, nil))}))

	body := `{"name":"site","kind":"http","url":"https://example.com/health","interval_seconds":60,"timeout_ms":3000}`
	rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/monitors", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	id := between(rec.Body.String(), `"id":"`, `"`)
	for name, bad := range map[string]string{
		"below floor":             `{"name":"x","kind":"http","url":"https://example.com","interval_seconds":1,"timeout_ms":500}`,
		"private host":            `{"name":"x","kind":"tcp","host":"127.0.0.1","port":80,"interval_seconds":60,"timeout_ms":1000}`,
		"headers without a vault": `{"name":"x","kind":"http","url":"https://example.com","interval_seconds":60,"timeout_ms":1000,"headers":{"X-Probe":"value"}}`,
	} {
		if r := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/monitors", bad); r.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400 (%s)", name, r.Code, r.Body.String())
		}
	}
	if r := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/monitors", ""); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), id) {
		t.Errorf("list = %d %s", r.Code, r.Body.String())
	}
	if r := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/monitors/"+id+"/results?window=2h&limit=5", ""); r.Code != http.StatusOK {
		t.Errorf("results = %d %s", r.Code, r.Body.String())
	}
	if r := apiDo(t, rt, cookie, http.MethodDelete, "/api/v1/monitors/"+id, ""); r.Code != http.StatusNoContent {
		t.Errorf("delete = %d", r.Code)
	}
	if r := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/monitors/"+id+"/results", ""); r.Code != http.StatusNotFound {
		t.Errorf("results after delete = %d, want 404", r.Code)
	}
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	s = s[i+len(start):]
	if j := strings.Index(s, end); j >= 0 {
		return s[:j]
	}
	return s
}
