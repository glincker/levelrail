package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDokployClient_ListApplications(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/project.all":
			_ = json.NewEncoder(w).Encode([]dokployProject{{ProjectID: "p1", Name: "default"}})
		case r.URL.Path == "/api/project.one" && r.URL.Query().Get("projectId") == "p1":
			_ = json.NewEncoder(w).Encode(dokployProjectDetail{
				ProjectID:    "p1",
				Applications: []dokployApplicationSummary{{ApplicationID: "a1", Name: "web"}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewDokployClient(srv.URL, "dokploy-key")
	apps, err := c.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("ListApplications() error = %v", err)
	}
	if gotKey != "dokploy-key" {
		t.Errorf("x-api-key = %q, want dokploy-key", gotKey)
	}
	if len(apps) != 1 || apps[0].Name != "web" {
		t.Errorf("apps = %+v, want one app named web", apps)
	}
}

func TestDokployClient_GetApplication(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("applicationId")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dokployApplication{ApplicationID: "a1", Name: "web", SourceType: "github"})
	}))
	defer srv.Close()

	c := NewDokployClient(srv.URL, "t")
	app, err := c.GetApplication(context.Background(), "a1")
	if err != nil {
		t.Fatalf("GetApplication() error = %v", err)
	}
	if gotPath != "/api/application.one" {
		t.Errorf("path = %q, want /api/application.one", gotPath)
	}
	if gotQuery != "a1" {
		t.Errorf("applicationId query = %q, want a1", gotQuery)
	}
	if app.SourceType != "github" {
		t.Errorf("SourceType = %q, want github", app.SourceType)
	}
}

func TestDokployClient_ListDomains(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]dokployDomain{{Host: "app.example.com", Port: 3000}})
	}))
	defer srv.Close()

	c := NewDokployClient(srv.URL, "t")
	domains, err := c.ListDomains(context.Background(), "a1")
	if err != nil {
		t.Fatalf("ListDomains() error = %v", err)
	}
	if gotPath != "/api/domain.byApplicationId" {
		t.Errorf("path = %q, want /api/domain.byApplicationId", gotPath)
	}
	if len(domains) != 1 || domains[0].Host != "app.example.com" {
		t.Errorf("domains = %+v, want one entry for app.example.com", domains)
	}
}

func TestDokployClient_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer srv.Close()

	c := NewDokployClient(srv.URL, "bad-key")
	_, err := c.ListProjects(context.Background())
	if err == nil {
		t.Fatalf("ListProjects() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "Invalid API key") {
		t.Errorf("error = %v, want it to contain the Dokploy message body", err)
	}
	var apiErr *dokployAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want a *dokployAPIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusUnauthorized)
	}
}
