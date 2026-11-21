package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudAWSServicesCLI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/cloud-accounts/aws-prod/aws/ecr/repositories":
			_, _ = w.Write([]byte(`{"repositories":[{"name":"web","uri":"1.dkr.ecr.eu-west-1.amazonaws.com/web","scan_on_push":true}]}`))
		case "/api/v1/cloud-accounts/aws-prod/aws/guard":
			_, _ = w.Write([]byte(`{"kill_switch":false,"monthly_cap_usd":100,"per_create_max_usd":10,"mtd_spend_usd":12.5,"spend_state":"ok"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	table, _ := runCLIExpectOK(t, []string{"cloud", "aws", "ecr", "list", "aws-prod", "--api-url", srv.URL})
	if !strings.Contains(table, "web") {
		t.Fatalf("ecr table = %q", table)
	}
	js, _ := runCLIExpectOK(t, []string{"cloud", "aws", "guard", "aws-prod", "--json", "--api-url", srv.URL})
	if !strings.Contains(js, "monthly_cap_usd") {
		t.Fatalf("guard json = %q", js)
	}
}
