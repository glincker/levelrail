package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

type formFakeChannels struct {
	NotificationChannels
}

func (formFakeChannels) GetNotificationChannel(_ context.Context, id string) (*alerting.NotificationChannel, error) {
	if id == "missing" {
		return nil, alerting.ErrNotificationChannelNotFound
	}
	return &alerting.NotificationChannel{ID: id, Enabled: true}, nil
}

type formNoticeRecorder struct {
	mu    sync.Mutex
	texts []string
	err   error
}

func (n *formNoticeRecorder) SendNotice(_ context.Context, _ alerting.NotificationChannel, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.texts = append(n.texts, text)
	return n.err
}

func formsTestRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db, cookie := newSafetyRouter(t, stubResolver{digest: newDigest})
	return rt, db, cookie
}

func createTestForm(t *testing.T, rt *Router, cookie *http.Cookie, body string) formResource {
	t.Helper()
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/forms", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body)
	}
	var f formResource
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func publicPost(rt *Router, path, ctype, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.RemoteAddr = "203.0.113.9:4444"
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return serve(rt, req)
}

func TestFormCreateValidation(t *testing.T) {
	rt, _, cookie := formsTestRouter(t)
	rt.notificationChannels = formFakeChannels{}
	createTestForm(t, rt, cookie, `{"name":"contact","redirect_url":"https://site.test/thanks"}`)

	tests := []struct {
		name string
		body string
		want int
	}{
		{"duplicate", `{"name":"contact"}`, http.StatusConflict},
		{"bad name", `{"name":"Bad Name"}`, http.StatusBadRequest},
		{"js redirect", `{"name":"a","redirect_url":"javascript:alert(1)"}`, http.StatusBadRequest},
		{"bad honeypot", `{"name":"a","honeypot_field":"1x"}`, http.StatusBadRequest},
		{"unknown channel", `{"name":"a","notify_channel_ids":["missing"]}`, http.StatusBadRequest},
		{"ok with channel", `{"name":"b","notify_channel_ids":["ch1","ch1"]}`, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/forms", tt.body))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/nope/forms", `{"name":"x"}`)); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown app = %d", rec.Code)
	}
}

func TestFormPublicSubmitFlows(t *testing.T) {
	rt, db, cookie := formsTestRouter(t)
	f := createTestForm(t, rt, cookie, `{"name":"contact","redirect_url":"https://site.test/thanks"}`)
	if !strings.Contains(f.Snippet, f.EndpointPath) || !strings.Contains(f.Snippet, `name="bot-field"`) {
		t.Fatalf("snippet = %s", f.Snippet)
	}
	path := f.EndpointPath
	form := "application/x-www-form-urlencoded"

	tests := []struct {
		name     string
		ctype    string
		body     string
		headers  []string
		wantCode int
		wantLoc  string
	}{
		{"browser post redirects", form, "email=a%40b.c&message=hi", nil, http.StatusSeeOther, "https://site.test/thanks"},
		{"json client gets ok", "application/json", `{"email":"a@b.c"}`, nil, http.StatusOK, ""},
		{"fetch with accept json", form, "email=a%40b.c", []string{"Accept", "application/json"}, http.StatusOK, ""},
		{"honeypot looks like success", form, "email=a%40b.c&bot-field=gotcha", nil, http.StatusSeeOther, "https://site.test/thanks"},
		{"wrong type", "text/plain", "hi", nil, http.StatusUnsupportedMediaType, ""},
		{"empty", form, "", nil, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := publicPost(rt, path, tt.ctype, tt.body, tt.headers...)
			if rec.Code != tt.wantCode || rec.Header().Get("Location") != tt.wantLoc {
				t.Fatalf("status=%d loc=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body)
			}
			if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
				t.Error("CORS header missing")
			}
		})
	}

	stored, err := db.ListFormSubmissions(context.Background(), f.ID, store.FormSubmissionQuery{})
	if err != nil || len(stored) != 4 {
		t.Fatalf("stored %d rows, err %v", len(stored), err)
	}
	spam := 0
	for _, s := range stored {
		if s.Spam {
			spam++
		}
		if strings.Contains(s.IPHash, "203.0.113") || s.IPHash == "" {
			t.Errorf("ip hash %q", s.IPHash)
		}
		if _, ok := s.Fields["bot-field"]; ok {
			t.Error("honeypot value was stored")
		}
	}
	if spam != 1 {
		t.Fatalf("spam rows = %d, want 1", spam)
	}

	if rec := publicPost(rt, "/api/v1/forms/web/nope", form, "a=1"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown form = %d", rec.Code)
	}
	pre := httptest.NewRequest(http.MethodOptions, path, nil)
	if rec := serve(rt, pre); rec.Code != http.StatusNoContent || !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "POST") {
		t.Fatalf("preflight = %d %v", rec.Code, rec.Header())
	}
}

func TestFormPublicLimits(t *testing.T) {
	t.Setenv("FORMS_RATE_PER_IP_PER_MINUTE", "2")
	t.Setenv("FORMS_MAX_BODY_BYTES", "64")
	rt, _, cookie := formsTestRouter(t)
	f := createTestForm(t, rt, cookie, `{"name":"contact"}`)
	form := "application/x-www-form-urlencoded"

	if rec := publicPost(rt, f.EndpointPath, form, "a="+strings.Repeat("x", 100)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize = %d", rec.Code)
	}
	if rec := publicPost(rt, f.EndpointPath, form, "a=1"); rec.Code != http.StatusOK {
		t.Fatalf("second request = %d: %s", rec.Code, rec.Body)
	}
	rec := publicPost(rt, f.EndpointPath, form, "a=1")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("limited = %d retry=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestFormNotifyHalfSucceeded(t *testing.T) {
	rt, db, cookie := formsTestRouter(t)
	n := &formNoticeRecorder{err: errors.New("slack down")}
	rt.deviceNotifier = n
	rt.notificationChannels = formFakeChannels{}
	rt.formsRuntime().svc.Dispatch = func(fn func()) { fn() }
	f := createTestForm(t, rt, cookie, `{"name":"contact","notify_channel_ids":["ch1"]}`)

	if rec := publicPost(rt, f.EndpointPath, "application/json", `{"email":"a@b.c"}`); rec.Code != http.StatusOK {
		t.Fatalf("notify failure must not fail the post: %d %s", rec.Code, rec.Body)
	}
	if rec := publicPost(rt, f.EndpointPath, "application/json", `{"email":"x@y.z","bot-field":"bot"}`); rec.Code != http.StatusOK {
		t.Fatalf("spam post = %d", rec.Code)
	}
	if len(n.texts) != 1 || !strings.Contains(n.texts[0], "contact") {
		t.Fatalf("notices = %v, want exactly one for the non-spam post", n.texts)
	}
	rows, _ := db.ListFormSubmissions(context.Background(), f.ID, store.FormSubmissionQuery{})
	if len(rows) != 2 {
		t.Fatalf("stored %d, want 2", len(rows))
	}
}

func TestFormInboxAndIsolation(t *testing.T) {
	rt, _, cookie := formsTestRouter(t)
	a := createTestForm(t, rt, cookie, `{"name":"a"}`)
	b := createTestForm(t, rt, cookie, `{"name":"b"}`)
	form := "application/x-www-form-urlencoded"
	publicPost(rt, a.EndpointPath, form, "email=ada%40x.test&message=hello")
	publicPost(rt, a.EndpointPath, form, "email=bob%40x.test&message==SUM(1)")
	publicPost(rt, b.EndpointPath, form, "email=carol%40x.test")

	get := func(path string) *httptest.ResponseRecorder {
		return serve(rt, authedRequest(t, cookie, http.MethodGet, path, ""))
	}
	var list formSubmissionsResponse
	rec := get("/api/v1/apps/web/forms/a/submissions?q=ada")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Submissions) != 1 || strings.Contains(rec.Body.String(), "carol") {
		t.Fatalf("search = %s", rec.Body)
	}
	rec = get("/api/v1/apps/web/forms/a/submissions?limit=1")
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Submissions) != 1 || list.NextBefore == 0 {
		t.Fatalf("paging = %s", rec.Body)
	}
	firstID := list.Submissions[0].ID

	if rec := serve(rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/forms/b/submissions/"+itoa(firstID), "")); rec.Code != http.StatusNotFound {
		t.Fatalf("deleting another form's submission = %d, want 404", rec.Code)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/forms/a/submissions/"+itoa(firstID)+"/spam", `{"spam":true}`)); rec.Code != http.StatusNoContent {
		t.Fatalf("mark spam = %d %s", rec.Code, rec.Body)
	}
	rec = get("/api/v1/apps/web/forms/a/submissions?spam=true")
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Submissions) != 1 || list.Submissions[0].SpamReason != "manual" {
		t.Fatalf("spam filter = %s", rec.Body)
	}

	rec = get("/api/v1/apps/web/forms/a/export")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") || !strings.Contains(rec.Body.String(), "'=SUM(1)") || strings.Contains(rec.Body.String(), "carol") {
		t.Fatalf("export = %d %s", rec.Code, rec.Body)
	}

	if rec := serve(rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/forms/a", "")); rec.Code != http.StatusNoContent {
		t.Fatalf("delete form = %d", rec.Code)
	}
	if rec := get("/api/v1/apps/web/forms/a/submissions"); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted form inbox = %d", rec.Code)
	}
	if rec := get("/api/v1/apps/web/forms/b/submissions"); !strings.Contains(rec.Body.String(), "carol") {
		t.Fatalf("form b lost its data: %s", rec.Body)
	}
}

func TestFormUnauthenticatedAdminRefused(t *testing.T) {
	rt, _, cookie := formsTestRouter(t)
	f := createTestForm(t, rt, cookie, `{"name":"a"}`)
	for _, path := range []string{"/api/v1/apps/web/forms", "/api/v1/apps/web/forms/a/submissions", "/api/v1/apps/web/forms/a/export"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if rec := serve(rt, req); rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
			t.Errorf("%s anonymous = %d", path, rec.Code)
		}
	}
	_ = f
}

func TestDeleteAppRemovesForms(t *testing.T) {
	rt, db, cookie := formsTestRouter(t)
	f := createTestForm(t, rt, cookie, `{"name":"a"}`)
	publicPost(rt, f.EndpointPath, "application/json", `{"x":"1"}`)
	if err := db.DeleteDesiredService(context.Background(), "web"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetForm(context.Background(), "web", "a"); !errors.Is(err, store.ErrFormNotFound) {
		t.Fatalf("form survived app delete: %v", err)
	}
	var n int
	_ = db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM form_submissions`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d submissions survived app delete", n)
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
