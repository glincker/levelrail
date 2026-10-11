package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_Templates_List_Filters(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]serviceTemplateListItem{
			{ID: "gitea-with-postgresql", Name: "Gitea With Postgresql", Category: "Developer Tools", Slogan: "Git hosting", Installable: true},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"templates", "list", "--api-url", srv.URL, "--search", "gitea", "--category", "Developer Tools", "--installable"})

	for _, want := range []string{"q=gitea", "category=Developer+Tools", "installable=true"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query = %q, want it to contain %q", gotQuery, want)
		}
	}
	if !strings.Contains(stdout, "gitea-with-postgresql") || !strings.Contains(stdout, "form") {
		t.Errorf("stdout = %q, want the template listed with its install form", stdout)
	}
}

func TestRun_Templates_Get_ShowsInstallInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(serviceTemplateDetail{
			ID: "gitea-with-postgresql", Name: "Gitea", Compose: "services: {}\n",
			Stack: []apiclient.TemplateStackService{{Name: "gitea", Image: "gitea/gitea:latest", Ports: []int{3000}, Volumes: []string{"gitea-data:/data"}}},
			Manifest: &apiclient.TemplateManifestInfo{
				Env:     []apiclient.TemplateSetting{{Key: "POSTGRESQL_PASSWORD", Generate: "password"}, {Key: "SMTP_HOST"}},
				Domains: []apiclient.TemplateDomainPrompt{{Service: "gitea", Port: 3000}},
				Source:  &apiclient.TemplateSource{Name: "coolify", Path: "templates/compose/gitea.yaml", License: "Apache-2.0"},
			},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"templates", "get", "gitea-with-postgresql", "--api-url", srv.URL})

	for _, want := range []string{"gitea/gitea:latest", "gitea-data:/data", "POSTGRESQL_PASSWORD  generated password", "SMTP_HOST  optional", "--domain gitea=HOST", "licence Apache-2.0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRun_Templates_Updates(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.TemplateUpdateReport{Apps: []apiclient.TemplateUpdateApp{{
			App: "wiki", TemplateID: "wikijs",
			Services: []apiclient.TemplateUpdateService{
				{Service: "db", Image: "postgres:16-alpine", Status: "newer_available", NewerTags: []string{"18-alpine", "17-alpine"}},
				{Service: "wiki", Image: "ghcr.io/requarks/wiki:2", Status: "unchecked", Note: "tag listing is supported for Docker Hub images only"},
			},
		}}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"templates", "updates", "--api-url", srv.URL})

	if gotPath != "/api/v1/service-templates/updates" {
		t.Errorf("path = %s", gotPath)
	}
	for _, want := range []string{"wiki", "wikijs", "newer_available", "18-alpine,17-alpine", "Docker Hub images only"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRun_Templates_Updates_None(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.TemplateUpdateReport{})
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"templates", "updates", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no apps installed from a template") {
		t.Errorf("stdout = %q", stdout)
	}
}
