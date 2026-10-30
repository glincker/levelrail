package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func saveMailApp(t *testing.T, db *store.DB, bind string, ports ...int) {
	t.Helper()
	svc := store.DesiredService{Name: "mail", Image: "i:1", Port: mailWebPort, TemplateID: mailTemplateID, BindAddress: bind}
	for _, p := range ports {
		svc.PublishedPorts = append(svc.PublishedPorts, store.PublishedPort{ContainerPort: p, HostPort: p, Protocol: store.PortProtocolTCP})
	}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatal(err)
	}
}

func mailPortsCall(t *testing.T, method, path, body string) (*httptest.ResponseRecorder, mailPortsResponse, mailPortsOpenResponse) {
	t.Helper()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	return mailPortsCallOn(t, rt, db, cookie, method, path, body)
}

func mailPortsCallOn(t *testing.T, rt *Router, _ *store.DB, cookie *http.Cookie, method, path, body string) (*httptest.ResponseRecorder, mailPortsResponse, mailPortsOpenResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	var list mailPortsResponse
	var open mailPortsOpenResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	_ = json.Unmarshal(rec.Body.Bytes(), &open)
	return rec, list, open
}

func TestMailPortsListNoMailApp(t *testing.T) {
	rec, got, _ := mailPortsCall(t, http.MethodGet, "/api/v1/mail/ports", "")
	if rec.Code != http.StatusOK || got.Installed || len(got.Ports) != 0 || got.NextStep == "" {
		t.Fatalf("unexpected %d %+v", rec.Code, got)
	}
}

func TestMailPortsListReportsFourPortsAndPrivateWeb(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	saveMailApp(t, db, "")
	rec, got, _ := mailPortsCallOn(t, rt, db, cookie, http.MethodGet, "/api/v1/mail/ports", "")
	if rec.Code != http.StatusOK || !got.Installed || got.App != "mail" || len(got.Ports) != 4 {
		t.Fatalf("unexpected %d %+v", rec.Code, got)
	}
	want := []int{25, 465, 587, 993}
	for i, p := range got.Ports {
		if p.Port != want[i] || p.Port == mailWebPort || p.Bind != "not_published" || p.Published {
			t.Fatalf("port row %d wrong: %+v", i, p)
		}
	}
	if got.WebListener != "private" || !strings.Contains(got.Port25Note, "provider") || got.Warning == "" {
		t.Fatalf("notes wrong: %+v", got)
	}
}

func TestMailPortsOpenRefusals(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec, _, _ := mailPortsCallOn(t, rt, db, cookie, http.MethodPost, "/api/v1/mail/ports/open", `{"confirm":true}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no mail app must be refused, got %d", rec.Code)
	}
	saveMailApp(t, db, "")
	rec, _, _ = mailPortsCallOn(t, rt, db, cookie, http.MethodPost, "/api/v1/mail/ports/open", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing confirm must be refused, got %d", rec.Code)
	}
}

func TestMailPortsOpenNeverTouchesWebListener(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	saveMailApp(t, db, "")
	rec, _, open := mailPortsCallOn(t, rt, db, cookie, http.MethodPost, "/api/v1/mail/ports/open", `{"confirm":true}`)
	if rec.Code != http.StatusConflict || open.Result != "refused" || len(open.Changed) != 0 {
		t.Fatalf("service wide bind must be refused: %d %+v", rec.Code, open)
	}
	for _, p := range open.Pending {
		if p == mailWebPort {
			t.Fatal("web listener must never be a target")
		}
	}
	svc, err := db.GetDesiredService(context.Background(), "mail")
	if err != nil || mailBindOf(*svc) != "private" || len(svc.PublishedPorts) != 0 {
		t.Fatalf("nothing may change: %v %+v", err, svc)
	}
}

func TestMailPortsOpenIdempotentWhenAlreadyOpen(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	saveMailApp(t, db, "public", 25, 465, 587, 993)
	for i := 0; i < 2; i++ {
		rec, _, open := mailPortsCallOn(t, rt, db, cookie, http.MethodPost, "/api/v1/mail/ports/open", `{"confirm":true}`)
		if rec.Code != http.StatusOK || open.Result != "unchanged" || len(open.Changed) != 0 {
			t.Fatalf("run %d: %d %+v", i, rec.Code, open)
		}
	}
	_, list, _ := mailPortsCallOn(t, rt, db, cookie, http.MethodGet, "/api/v1/mail/ports", "")
	if list.WebListener != "public" || list.Ports[0].Bind != "public" {
		t.Fatalf("list must expose the service wide bind: %+v", list)
	}
}
