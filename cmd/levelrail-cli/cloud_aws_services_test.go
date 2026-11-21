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

func TestCloudAWSRDSCLI(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/cloud-accounts/aws-prod/aws/rds/instances":
			_, _ = w.Write([]byte(`{"instances":[{"id":"db1","engine":"postgres","class":"db.t4g.micro","status":"available","managed":true}]}`))
		case "POST /api/v1/cloud-accounts/aws-prod/aws/rds/instances/db1/actions":
			b := make([]byte, 256)
			n, _ := r.Body.Read(b)
			got = string(b[:n])
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"db1","action":"stop"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	js, _ := runCLIExpectOK(t, []string{"cloud", "aws", "rds", "list", "aws-prod", "--json", "--api-url", srv.URL})
	if !strings.Contains(js, `"db1"`) {
		t.Fatalf("rds list json = %q", js)
	}
	runCLIExpectOK(t, []string{"cloud", "aws", "rds", "stop", "aws-prod", "db1", "--api-url", srv.URL})
	if !strings.Contains(got, `"action":"stop"`) {
		t.Fatalf("stop body = %q", got)
	}
}

func TestCloudAWSOpsCLI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/cloud-accounts/aws-prod/aws/ebs/snapshots":
			_, _ = w.Write([]byte(`{"snapshots":[{"id":"snap-1","volume_id":"vol-1","state":"completed","size_gb":8,"managed":true}]}`))
		case "GET /api/v1/cloud-accounts/aws-prod/aws/idle":
			_, _ = w.Write([]byte(`{"dry_run":true,"suggestions":[{"kind":"ebs_unattached","id":"vol-9","action":"snapshot_delete","est_monthly_usd":8}]}`))
		case "GET /api/v1/cloud-accounts/aws-prod/aws/cloudwatch/metrics":
			if r.URL.Query().Get("instance_id") != "i-1" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"series":[{"name":"CPUUtilization","points":[{"t":"2026-11-21T11:05:00Z","v":3}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	js, _ := runCLIExpectOK(t, []string{"cloud", "aws", "snapshots", "list", "aws-prod", "--json", "--api-url", srv.URL})
	if !strings.Contains(js, `"snap-1"`) {
		t.Fatalf("snapshots json = %q", js)
	}
	table, _ := runCLIExpectOK(t, []string{"cloud", "aws", "idle", "aws-prod", "--api-url", srv.URL})
	if !strings.Contains(table, "vol-9") {
		t.Fatalf("idle table = %q", table)
	}
	out, _ := runCLIExpectOK(t, []string{"cloud", "aws", "metrics", "aws-prod", "i-1", "--api-url", srv.URL})
	if !strings.Contains(out, "CPUUtilization") {
		t.Fatalf("metrics out = %q", out)
	}
}
