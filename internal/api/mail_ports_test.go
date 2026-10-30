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

const mailStepUp = `,"step_up":{"method":"password","password":"` + testAdminPassword + `"}`

func saveMailTemplateApp(t *testing.T, db *store.DB) {
	t.Helper()
	svc := store.DesiredService{Name: "mail", Image: "i:1", Port: mailWebPort, TemplateID: mailTemplateID, BindAddress: "private"}
	for _, p := range []int{25, 465, 587, 993} {
		svc.PublishedPorts = append(svc.PublishedPorts, store.PublishedPort{ContainerPort: p, HostPort: p, Protocol: store.PortProtocolTCP, Bind: "private"})
	}
	svc.PublishedPorts = append(svc.PublishedPorts, store.PublishedPort{ContainerPort: 4190, HostPort: 4190, Protocol: store.PortProtocolTCP, Bind: "private"})
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatal(err)
	}
}

func TestMailPortsOpenRefusesWhenTemplatePortsMissing(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	saveMailApp(t, db, "")
	rec, _, open := mailPortsCallOn(t, rt, db, cookie, http.MethodPost, "/api/v1/mail/ports/open", `{"confirm":true`+mailStepUp+`}`)
	if rec.Code != http.StatusConflict || open.Result != "refused" || len(open.Pending) != 4 {
		t.Fatalf("missing published ports must be refused: %d %+v", rec.Code, open)
	}
}

func TestMailPortsOpenNeedsStepUp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	saveMailTemplateApp(t, db)
	rec, _, _ := mailPortsCallOn(t, rt, db, cookie, http.MethodPost, "/api/v1/mail/ports/open", `{"confirm":true}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), errCodeStepUpRequired) {
		t.Fatalf("open without step-up must be refused: %d %s", rec.Code, rec.Body.String())
	}
	svc, _ := db.GetDesiredService(context.Background(), "mail")
	for _, p := range svc.PublishedPorts {
		if p.Bind != "private" {
			t.Fatalf("nothing may change without step-up: %+v", p)
		}
	}
}

func TestMailPortsOpenPublishesExactlyTheFourPorts(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	saveMailTemplateApp(t, db)
	rec, _, open := mailPortsCallOn(t, rt, db, cookie, http.MethodPost, "/api/v1/mail/ports/open", `{"confirm":true`+mailStepUp+`}`)
	if rec.Code != http.StatusOK || open.Result != "opened" || len(open.Changed) != 4 {
		t.Fatalf("open failed: %d %s", rec.Code, rec.Body.String())
	}
	svc, err := db.GetDesiredService(context.Background(), "mail")
	if err != nil {
		t.Fatal(err)
	}
	if svc.BindAddress != "private" || svc.Port != mailWebPort {
		t.Fatalf("the web listener must stay private: %+v", svc)
	}
	for _, p := range svc.PublishedPorts {
		want := "public"
		if p.HostPort == 4190 {
			want = "private"
		}
		if p.HostPort == mailWebPort || p.Bind != want {
			t.Fatalf("port %d bind %q, want %q", p.HostPort, p.Bind, want)
		}
	}
	rules, _ := db.ListFirewallRules(context.Background())
	allowed := map[int]bool{}
	for _, r := range rules {
		if strings.HasPrefix(r.Label, publishedPortRuleLabelPrefix+"mail:") {
			allowed[r.Port] = true
		}
	}
	if len(allowed) != 4 || allowed[mailWebPort] || allowed[4190] || !allowed[25] || !allowed[993] {
		t.Fatalf("firewall rules must cover exactly the four mail ports: %v", allowed)
	}
	_, list, _ := mailPortsCallOn(t, rt, db, cookie, http.MethodGet, "/api/v1/mail/ports", "")
	if list.WebListener != "private" || list.Ports[0].Bind != "public" || list.Ports[3].Bind != "public" {
		t.Fatalf("list must report per-port binds: %+v", list)
	}
	for i := 0; i < 2; i++ {
		rec, _, again := mailPortsCallOn(t, rt, db, cookie, http.MethodPost, "/api/v1/mail/ports/open", `{"confirm":true`+mailStepUp+`}`)
		if rec.Code != http.StatusOK || again.Result != "unchanged" || len(again.Changed) != 0 {
			t.Fatalf("run %d must be idempotent: %d %+v", i, rec.Code, again)
		}
	}
	rules, _ = db.ListFirewallRules(context.Background())
	if n := len(rules); n != 4 {
		t.Fatalf("idempotent open must not add rules, have %d", n)
	}
}
