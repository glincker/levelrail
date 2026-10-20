package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
)

func diagnoseRouter(t *testing.T) (*Router, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.publicHost = "203.0.113.7"
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	seedRunningApp(t, db, "far.example.com")
	called := false
	rt.traffic.doctorProbes = &domaindoctor.Probes{
		DNS:  doctorFakeDNS{ips: []string{"198.51.100.4"}},
		TLS:  doctorFakeTLS{called: &called},
		HTTP: doctorFakeTLS{called: &called},
	}
	return rt, loginTestSession(t, rt, db)
}

func TestDomainDiagnoseByDomain(t *testing.T) {
	rt, cookie := diagnoseRouter(t)
	var rep domaindoctor.Report
	if code := trafficCall(t, rt, cookie, http.MethodPost, "/api/v1/domains/far.example.com/diagnose", &rep); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if rep.App != "web" || rep.Domain != "far.example.com" || rep.Status != domaindoctor.StatusProblems {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Propagation == nil || rep.Propagation.State != domaindoctor.PropagationSettled {
		t.Fatalf("propagation = %+v", rep.Propagation)
	}
	if len(rep.Records) != 1 || rep.Records[0].Type != "A" || rep.Records[0].Value != "203.0.113.7" {
		t.Fatalf("records = %+v", rep.Records)
	}
	for _, c := range rep.Checks {
		if c.Fix != nil && c.Fix.Kind == "" {
			t.Errorf("check %s has a fix without a kind", c.ID)
		}
	}
	if code := trafficCall(t, rt, cookie, http.MethodPost, "/api/v1/domains/nobody.example.com/diagnose", nil); code != http.StatusNotFound {
		t.Errorf("unknown domain status = %d, want 404", code)
	}
}

func TestDomainDiagnoseStream(t *testing.T) {
	rt, cookie := diagnoseRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/domains/far.example.com/diagnose/stream", ""))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status = %d type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var checks, done int
	var first string
	sc := bufio.NewScanner(rec.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, `data: {"type":"check"`):
			if checks == 0 {
				first = line
			}
			checks++
		case strings.HasPrefix(line, `data: {"type":"done"`):
			done++
		}
	}
	if checks == 0 || done != 1 {
		t.Fatalf("checks = %d done = %d", checks, done)
	}
	if !strings.Contains(first, domaindoctor.IDDNSResolves) {
		t.Errorf("first streamed check = %s, want %s", first, domaindoctor.IDDNSResolves)
	}
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/nobody.example.com/diagnose/stream", nil); code != http.StatusNotFound {
		t.Errorf("unknown domain status = %d, want 404", code)
	}
}
