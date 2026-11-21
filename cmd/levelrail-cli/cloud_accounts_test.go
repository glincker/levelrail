package main

import (
	"net/http"
	"net/http/httptest"
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
