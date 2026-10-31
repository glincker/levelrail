package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/statuswebhook"
	"github.com/GLINCKER/levelrail/internal/store"
)

type hookCall struct {
	sig, ts, eventType string
	body               []byte
}

type statusHookReceiver struct {
	srv   *httptest.Server
	mu    sync.Mutex
	calls []hookCall
	code  int
}

func newStatusHookReceiver(t *testing.T, code int) *statusHookReceiver {
	t.Helper()
	h := &statusHookReceiver{code: code}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		h.mu.Lock()
		h.calls = append(h.calls, hookCall{sig: r.Header.Get("X-Signature"), ts: r.Header.Get("X-Timestamp"), eventType: r.Header.Get("X-Event-Type"), body: b})
		h.mu.Unlock()
		w.WriteHeader(h.code)
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *statusHookReceiver) snapshot() []hookCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]hookCall(nil), h.calls...)
}

func webhookEnv(t *testing.T) {
	t.Helper()
	t.Setenv(statuswebhook.EnvAllowPrivate, "true")
	t.Setenv(statuswebhook.EnvAllowHTTP, "true")
	t.Setenv(statuswebhook.EnvBackoffBase, "1ms")
	t.Setenv(statuswebhook.EnvBackoffMax, "2ms")
}

func enableStatusPage(t *testing.T, rt *Router, cookie *http.Cookie) {
	t.Helper()
	if rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/status-page", `{"enabled":true,"title":"Acme"}`); rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body.String())
	}
}

func createStatusHook(t *testing.T, rt *Router, cookie *http.Cookie, url, events string) (id, secret string) {
	t.Helper()
	rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/webhooks", `{"url":"`+url+`","label":"ops","events":`+events+`}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create webhook = %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ID            string `json:"id"`
		SigningSecret string `json:"signing_secret"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.SigningSecret == "" {
		t.Fatalf("create body = %s", rec.Body.String())
	}
	return out.ID, out.SigningSecret
}

func drainStatusHooks(t *testing.T, rt *Router, rounds int) {
	t.Helper()
	for i := 0; i < rounds; i++ {
		if _, err := rt.statusHooks.ProcessDue(context.Background()); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStatusWebhook_SignedDeliverySecretOnceAndPing(t *testing.T) {
	webhookEnv(t)
	rt, _, cookie := newStatusRouter(t, 100)
	enableStatusPage(t, rt, cookie)
	recv := newStatusHookReceiver(t, http.StatusOK)
	id, secret := createStatusHook(t, rt, cookie, recv.srv.URL, `["incident_opened","incident_resolved","component_changed"]`)

	list := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/status-page/webhooks", "").Body.String()
	if strings.Contains(list, secret) || strings.Contains(list, "signing_secret") {
		t.Fatalf("list leaks the signing secret: %s", list)
	}

	rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/incidents", `{"kind":"incident","title":"API down","impact":"major","body":"looking"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("incident = %d %s", rec.Code, rec.Body.String())
	}
	var inc statusIncidentResource
	_ = json.Unmarshal(rec.Body.Bytes(), &inc)
	apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/incidents/"+inc.ID+"/updates", `{"status":"identified","body":"found it"}`)
	apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/incidents/"+inc.ID+"/updates", `{"status":"resolved","body":"fixed"}`)
	drainStatusHooks(t, rt, 1)

	calls := recv.snapshot()
	if len(calls) != 2 {
		t.Fatalf("deliveries = %d, want 2 (opened and resolved, the filter drops updated)", len(calls))
	}
	if calls[0].eventType != "incident_opened" || calls[1].eventType != "incident_resolved" {
		t.Errorf("event types = %s, %s", calls[0].eventType, calls[1].eventType)
	}
	for _, c := range calls {
		if !statuswebhook.Verify(secret, c.ts, c.body, c.sig) {
			t.Errorf("signature does not verify for %s", c.eventType)
		}
		if statuswebhook.Verify("whsec_wrong", c.ts, c.body, c.sig) {
			t.Error("signature verified with a wrong secret")
		}
		if !strings.Contains(string(c.body), "API down") || strings.Contains(string(c.body), "target") {
			t.Errorf("unexpected body: %s", c.body)
		}
	}

	ping := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/webhooks/"+id+"/test", "")
	if ping.Code != http.StatusOK || !strings.Contains(ping.Body.String(), `"delivered":true`) {
		t.Fatalf("test ping = %d %s", ping.Code, ping.Body.String())
	}
	last := recv.snapshot()[2]
	if last.eventType != "ping" || !statuswebhook.Verify(secret, last.ts, last.body, last.sig) {
		t.Errorf("ping not signed or typed correctly: %s", last.eventType)
	}

	rot := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/webhooks/"+id+"/rotate", "")
	if rot.Code != http.StatusOK || strings.Contains(rot.Body.String(), secret) || !strings.Contains(rot.Body.String(), "signing_secret") {
		t.Fatalf("rotate = %d %s", rot.Code, rot.Body.String())
	}
	logRec := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/status-page/webhooks/"+id+"/deliveries", "")
	if logRec.Code != http.StatusOK || !strings.Contains(logRec.Body.String(), `"status_code":200`) {
		t.Errorf("deliveries = %d %s", logRec.Code, logRec.Body.String())
	}
}

func TestStatusWebhook_PrivateIncidentNeverDelivered(t *testing.T) {
	webhookEnv(t)
	rt, db, cookie := newStatusRouter(t, 100)
	enableStatusPage(t, rt, cookie)
	recv := newStatusHookReceiver(t, http.StatusOK)
	createStatusHook(t, rt, cookie, recv.srv.URL, `[]`)

	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour)
	priv := store.InternalStatusIncident{
		StatusIncident:         store.StatusIncident{ID: "privhook1", Kind: statuspage.IncidentKind, Title: "PRIVTITLE db-prod-7", Status: "investigating", Impact: "major", StartsAt: now},
		StatusIncidentInternal: store.StatusIncidentInternal{Source: "monitor", SourceRef: "mon1", InternalState: store.IncidentStateTriggered},
	}
	if err := db.CreateInternalStatusIncident(ctx, priv); err != nil {
		t.Fatal(err)
	}
	rt.emitStatusIncidentEvent(ctx, "privhook1", true)

	pub := store.StatusIncident{ID: "pubhook1", Kind: statuspage.IncidentKind, Title: "Public outage", Status: "investigating", Impact: "minor", StartsAt: now}
	if err := db.SaveStatusIncident(ctx, pub); err != nil {
		t.Fatal(err)
	}
	if err := db.AddInternalIncidentUpdate(ctx, store.StatusIncidentUpdate{ID: "privup1", IncidentID: "pubhook1", Status: "investigating", Body: "PRIVNOTE stack trace", CreatedAt: now}, store.UpdateVisibilityInternal, "op"); err != nil {
		t.Fatal(err)
	}
	rt.emitStatusIncidentEvent(ctx, "pubhook1", true)
	drainStatusHooks(t, rt, 1)

	calls := recv.snapshot()
	if len(calls) != 1 {
		t.Fatalf("deliveries = %d, want only the published incident", len(calls))
	}
	for _, n := range []string{"PRIVTITLE", "privhook1", "PRIVNOTE", "db-prod-7"} {
		if strings.Contains(string(calls[0].body), n) {
			t.Errorf("payload leaks %q: %s", n, calls[0].body)
		}
	}
}

func TestStatusWebhook_RetriesThenAutoPause(t *testing.T) {
	webhookEnv(t)
	t.Setenv(statuswebhook.EnvMaxAttempts, "2")
	t.Setenv(statuswebhook.EnvPauseAfter, "2")
	rt, db, cookie := newStatusRouter(t, 100)
	enableStatusPage(t, rt, cookie)
	recv := newStatusHookReceiver(t, http.StatusInternalServerError)
	id, _ := createStatusHook(t, rt, cookie, recv.srv.URL, `[]`)

	for _, title := range []string{"one", "two"} {
		if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/incidents", `{"kind":"incident","title":"`+title+`"}`); rec.Code != http.StatusCreated {
			t.Fatal(rec.Body.String())
		}
	}
	drainStatusHooks(t, rt, 6)

	hook, err := db.GetStatusWebhook(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !hook.Paused || hook.PausedReason == "" {
		t.Fatalf("subscriber should be auto-paused: %+v", hook)
	}
	if n := len(recv.snapshot()); n != 4 {
		t.Errorf("attempts = %d, want 2 events x 2 attempts", n)
	}
	logs, _ := db.ListStatusWebhookDeliveries(context.Background(), id, 10)
	if len(logs) != 2 || logs[0].State != store.WebhookFailed || logs[0].Attempts != 2 || logs[0].LastStatus != 500 || !strings.Contains(logs[0].LastError, "500") {
		t.Errorf("delivery log = %+v", logs)
	}
	before := len(recv.snapshot())
	apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/incidents", `{"kind":"incident","title":"three"}`)
	drainStatusHooks(t, rt, 2)
	if len(recv.snapshot()) != before {
		t.Error("a paused subscriber still received an event")
	}
}

func TestStatusWebhook_RefusesUnsafeURLsAndEnforcesCap(t *testing.T) {
	t.Setenv(statuswebhook.EnvMaxSubscribers, "1")
	rt, _, cookie := newStatusRouter(t, 100)
	enableStatusPage(t, rt, cookie)
	for _, u := range []string{"http://example.com/hook", "https://127.0.0.1/hook", "https://169.254.169.254/latest", "https://10.0.0.5/x", "https://localhost/x", "https://user:pw@example.com/x", "ftp://example.com/x"} {
		rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/webhooks", `{"url":"`+u+`"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", u, rec.Code)
		}
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/webhooks", `{"url":"https://example.com/a","events":["nope"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown event = %d", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/webhooks", `{"url":"https://example.com/a"}`); rec.Code != http.StatusCreated {
		t.Fatalf("first = %d %s", rec.Code, rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/webhooks", `{"url":"https://example.com/b"}`); rec.Code != http.StatusConflict {
		t.Errorf("over cap = %d, want 409", rec.Code)
	}
}

func TestStatusWebhook_EventIsIdempotent(t *testing.T) {
	webhookEnv(t)
	rt, db, cookie := newStatusRouter(t, 100)
	enableStatusPage(t, rt, cookie)
	recv := newStatusHookReceiver(t, http.StatusOK)
	id, _ := createStatusHook(t, rt, cookie, recv.srv.URL, `[]`)
	rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/incidents", `{"kind":"incident","title":"once"}`)
	var inc statusIncidentResource
	_ = json.Unmarshal(rec.Body.Bytes(), &inc)
	rt.emitStatusIncidentEvent(context.Background(), inc.ID, true)
	rt.emitStatusIncidentEvent(context.Background(), inc.ID, true)
	drainStatusHooks(t, rt, 1)
	if n := len(recv.snapshot()); n != 1 {
		t.Errorf("deliveries = %d, want 1", n)
	}
	apiDo(t, rt, cookie, http.MethodDelete, "/api/v1/status-page/webhooks/"+id, "")
	if _, err := db.GetStatusWebhook(context.Background(), id); err == nil {
		t.Error("webhook still exists after delete")
	}
}
