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

func TestCoolifyClient_ListApplications(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]coolifyApplication{{UUID: "u1", Name: "web", BuildPack: "dockerfile"}})
	}))
	defer srv.Close()

	c := NewCoolifyClient(srv.URL, "coolify-token")
	apps, err := c.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("ListApplications() error = %v", err)
	}
	if gotAuth != "Bearer coolify-token" {
		t.Errorf("Authorization = %q, want Bearer coolify-token", gotAuth)
	}
	if gotPath != "/api/v1/applications" {
		t.Errorf("path = %q, want /api/v1/applications", gotPath)
	}
	if len(apps) != 1 || apps[0].Name != "web" {
		t.Errorf("apps = %+v, want one app named web", apps)
	}
}

func TestCoolifyClient_GetApplication(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(coolifyApplication{UUID: "u1", Name: "web", FQDN: "https://web.example.com"})
	}))
	defer srv.Close()

	c := NewCoolifyClient(srv.URL, "t")
	app, err := c.GetApplication(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetApplication() error = %v", err)
	}
	if gotPath != "/api/v1/applications/u1" {
		t.Errorf("path = %q, want /api/v1/applications/u1", gotPath)
	}
	if app.FQDN != "https://web.example.com" {
		t.Errorf("FQDN = %q, want https://web.example.com", app.FQDN)
	}
}

func TestCoolifyClient_ListEnvs(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]coolifyEnvVar{{Key: "FOO", Value: "***"}})
	}))
	defer srv.Close()

	c := NewCoolifyClient(srv.URL, "t")
	envs, err := c.ListEnvs(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListEnvs() error = %v", err)
	}
	if gotPath != "/api/v1/applications/u1/envs" {
		t.Errorf("path = %q, want /api/v1/applications/u1/envs", gotPath)
	}
	if len(envs) != 1 || envs[0].Key != "FOO" {
		t.Errorf("envs = %+v, want one entry with key FOO", envs)
	}
}

func TestCoolifyClient_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthenticated."}`))
	}))
	defer srv.Close()

	c := NewCoolifyClient(srv.URL, "bad-token")
	_, err := c.ListApplications(context.Background())
	if err == nil {
		t.Fatalf("ListApplications() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "Unauthenticated.") {
		t.Errorf("error = %v, want it to contain the Coolify message body", err)
	}
	var apiErr *coolifyAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want a *coolifyAPIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusUnauthorized)
	}
}
