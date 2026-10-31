package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMailStatusNotInstalled(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/mail/status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got mailStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Installed || got.ContainerState != "none" || got.NextStep == "" {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestMailNextStep(t *testing.T) {
	if s := mailNextStep(mailStatusResponse{Installed: true, ContainerState: "running", AdminReachable: true, TLS: "self-signed"}); s == "" {
		t.Fatal("empty")
	}
}

func TestMailTLSKindAndRank(t *testing.T) {
	if mailTLSKind(nil) != "self-signed" || mailTLSKind(map[string]string{"MAIL_TLS_CERT_FILE": mailSelfSignedCert}) != "self-signed" {
		t.Fatal("default must be self-signed")
	}
	if mailTLSKind(map[string]string{"MAIL_TLS_CERT_FILE": "certs/cur.pem"}) != "provided" {
		t.Fatal("custom path must be provided")
	}
	up := mailStatusResponse{ContainerState: "running", AdminReachable: true}
	if mailRank(up) <= mailRank(mailStatusResponse{ContainerState: "running"}) || mailRank(mailStatusResponse{ContainerState: "running"}) <= mailRank(mailStatusResponse{ContainerState: "missing"}) {
		t.Fatal("rank order wrong")
	}
}

func TestMailStatusUnknownApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/mail/status?app=nope", ""))
	var got mailStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Installed || got.App != "" {
		t.Fatalf("unexpected %v %+v", err, got)
	}
}

func TestMailNames(t *testing.T) {
	d, h := mailNames(map[string]string{"MAIL_FIRST_DOMAIN": "Example.Test"})
	if d != "example.test" || h != "mail.example.test" {
		t.Fatalf("got %q %q", d, h)
	}
	d, h = mailNames(map[string]string{"MAIL_FIRST_DOMAIN": "example.test", "MAIL_HOSTNAME": "smtp.example.test"})
	if d != "example.test" || h != "smtp.example.test" {
		t.Fatalf("got %q %q", d, h)
	}
	if d, h = mailNames(map[string]string{"MAIL_FIRST_DOMAIN": "${MAIL_FIRST_DOMAIN}"}); d != "" || h != "" {
		t.Fatalf("placeholder leaked: %q %q", d, h)
	}
}

func TestMailNextStepReadinessFailing(t *testing.T) {
	s := mailNextStep(mailStatusResponse{Installed: true, ContainerState: "running", AdminReachable: true, Ready: false, ReconcileReason: "ReadinessFailed"})
	want := "The mail container is running but the app is failing its readiness check (ReadinessFailed). Check the logs."
	if s != want {
		t.Fatalf("got %q", s)
	}
}
