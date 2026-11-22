package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudInstancesListCLITableAndJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/cloud-accounts/aws-prod/instances" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`[{"id":"i-1","name":"web","region":"eu-west-1","state":"running","shape":"t3.small","managed":true}]`))
	}))
	defer srv.Close()
	table, _ := runCLIExpectOK(t, []string{"cloud", "instances", "list", "aws-prod", "--api-url", srv.URL})
	if !strings.Contains(table, "i-1") || !strings.Contains(table, "running") {
		t.Fatalf("table = %q", table)
	}
	js, _ := runCLIExpectOK(t, []string{"cloud", "instances", "list", "aws-prod", "--json", "--api-url", srv.URL})
	if !strings.Contains(js, `"id": "i-1"`) && !strings.Contains(js, `"id":"i-1"`) {
		t.Fatalf("json = %q", js)
	}
}

func TestCloudDoctorCLITextAndJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/cloud-accounts/aws-prod/doctor" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"ready":false,"steps":[{"id":"a","title":"Identity","status":"ok","detail":"fine"},{"id":"b","title":"Tier","status":"fail","detail":"missing","fix":"relink"}]}`))
	}))
	defer srv.Close()
	text, _ := runCLIExpectOK(t, []string{"cloud", "doctor", "--account", "aws-prod", "--api-url", srv.URL})
	if !strings.Contains(text, "FAIL Tier") || !strings.Contains(text, "fix: relink") || !strings.Contains(text, "not ready") {
		t.Fatalf("text = %q", text)
	}
	js, _ := runCLIExpectOK(t, []string{"cloud", "doctor", "--account", "aws-prod", "--json", "--api-url", srv.URL})
	if !strings.Contains(js, `"ready"`) || !strings.Contains(js, `"fix"`) {
		t.Fatalf("json = %q", js)
	}
}

func TestCloudAccountsAddOCISessionSendsProfileFiles(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".oci"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "[DEFAULT]\ntenancy=ocid1.tenancy.oc1..aaa\nregion=us-ashburn-1\nsecurity_token_file=~/.oci/tok\nkey_file=~/.oci/key.pem\n"
	for name, body := range map[string]string{"config": cfg, "tok": "a.b.c\n", "key.pem": "KEYPEM"} {
		if err := os.WriteFile(filepath.Join(home, ".oci", name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ora-s","provider":"oracle","status":"active"}`))
	}))
	defer srv.Close()
	runCLIExpectOK(t, []string{"cloud", "accounts", "add", "ora-s", "--provider", "oracle", "--auth", "oci_session",
		"--compartment", "ocid1.compartment.oc1..bbb", "--api-url", srv.URL})
	if got["session_token"] != "a.b.c" || got["private_key_pem"] != "KEYPEM" || got["method"] != "oci_session" {
		t.Fatalf("body = %v", got)
	}
	if pc, _ := got["profile_config"].(string); !strings.Contains(pc, "us-ashburn-1") || strings.Contains(pc, "key_file") {
		t.Fatalf("profile_config = %q", pc)
	}
}

func TestCloudSpendTablePrintsForecastServicesAndBudgets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"ok","source":"usage_api","estimated":false,"stale":false,"totals":[],` +
			`"forecast":{"amount":12.5,"currency":"USD","source":"provider"},` +
			`"by_service":[{"name":"s1","amount":1,"currency":"USD"},{"name":"s2","amount":2,"currency":"USD"},{"name":"s3","amount":3,"currency":"USD"},` +
			`{"name":"s4","amount":4,"currency":"USD"},{"name":"s5","amount":5,"currency":"USD"},{"name":"s6","amount":6,"currency":"USD"}],` +
			`"budgets":[{"name":"cap","amount":50,"actual_spend":5,"forecasted_spend":20,"currency":"USD"}]}`))
	}))
	defer srv.Close()
	out, _ := runCLIExpectOK(t, []string{"cloud", "spend", "acct", "--api-url", srv.URL})
	for _, want := range []string{"FORECAST  12.50 USD (provider)", "s6", "s2", "cap", "20.00"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "s1") {
		t.Fatalf("only the top 5 services print: %q", out)
	}
}
