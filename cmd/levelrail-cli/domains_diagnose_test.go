package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func diagnoseReport(fixed bool) apiclient.DomainDoctorReport {
	rep := apiclient.DomainDoctorReport{
		Domain: "shop.example.com", Status: "problems", Probed: false,
		Records:     []apiclient.DoctorRecord{{Name: "shop.example.com", Type: "A", Value: "203.0.113.7"}},
		Propagation: &apiclient.DoctorPropagation{State: "waiting", Detail: "no resolver knows this name yet", NextCheckSeconds: 30},
		Checks: []apiclient.DoctorCheck{{
			ID: "dns.resolves", Title: "shop.example.com does not resolve", State: "fail", Tier: 1,
			Fix: &apiclient.DoctorFix{
				Summary: "Add an A record.", Kind: "auto",
				Records: []apiclient.DoctorRecord{{Name: "shop.example.com", Type: "A", Value: "203.0.113.7"}},
				Action: &apiclient.DoctorAction{Kind: "request", Label: "Create the record", API: "POST /api/v1/apps/web/domains/shop.example.com/dns-records",
					Body: map[string]string{"name": "shop", "type": "A", "value": "203.0.113.7"}},
			},
		}},
	}
	if fixed {
		rep.Status = "healthy"
		rep.Checks[0].State = "pass"
		rep.Checks[0].Fix = nil
		rep.Propagation = &apiclient.DoctorPropagation{State: "settled", Detail: "every resolver agrees"}
	}
	return rep
}

func TestRun_DomainsDiagnose_PrintsFixAndRecords(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewEncoder(w).Encode(diagnoseReport(false))
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"domains", "diagnose", "Shop.Example.com", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/domains/shop.example.com/diagnose" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	for _, want := range []string{"1. [fail] shop.example.com does not resolve", "record for dns.resolves", "A", "203.0.113.7", "propagation: waiting", "check again in 30s"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %q", want, stdout)
		}
	}
}

func TestRun_DomainsDiagnose_FixAppliesAutoFixThenRechecks(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var posted map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/dns-records") {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &posted)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		diagnosed := 0
		for _, c := range calls {
			if strings.HasSuffix(c, "/diagnose") {
				diagnosed++
			}
		}
		_ = json.NewEncoder(w).Encode(diagnoseReport(diagnosed > 1))
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"domains", "diagnose", "shop.example.com", "--fix", "--api-url", srv.URL})
	want := []string{"POST /api/v1/domains/shop.example.com/diagnose", "POST /api/v1/apps/web/domains/shop.example.com/dns-records", "POST /api/v1/domains/shop.example.com/diagnose"}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	if posted["name"] != "shop" || posted["type"] != "A" || posted["value"] != "203.0.113.7" {
		t.Errorf("posted = %v", posted)
	}
	if !strings.Contains(stdout, "applied: Create the record (dns.resolves)") || !strings.Contains(stdout, "nothing to fix") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_DomainsDiagnose_FixSkipsGuidedFixes(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		rep := diagnoseReport(false)
		rep.Checks[0].Fix.Kind = "guided"
		_ = json.NewEncoder(w).Encode(rep)
	}))
	defer srv.Close()
	runCLIExpectOK(t, []string{"domains", "diagnose", "shop.example.com", "--fix", "--api-url", srv.URL})
	if len(calls) != 1 {
		t.Fatalf("calls = %v, want only the diagnosis", calls)
	}
}

func TestRun_DomainsDiagnose_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"domains", "diagnose"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_DomainsAdd_PrintsExactRecords(t *testing.T) {
	var gotAdd map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/diagnose"):
			_ = json.NewEncoder(w).Encode(diagnoseReport(false))
		default:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotAdd)
			_, _ = w.Write([]byte(`{"domains":["shop.example.com"],"changed":true}`))
		}
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"domains", "add", "web", "Shop.Example.com", "--api-url", srv.URL})
	if len(gotAdd["add"]) != 1 || gotAdd["add"][0] != "shop.example.com" {
		t.Errorf("add body = %v", gotAdd)
	}
	for _, want := range []string{"shop.example.com added to app \"web\"", "create this record", "203.0.113.7", "domains diagnose shop.example.com"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %q", want, stdout)
		}
	}
}

func TestRun_DomainsAdd_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"domains", "add", "web"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
