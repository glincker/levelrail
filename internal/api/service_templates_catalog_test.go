package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/kit/dockerhub"
)

func listTemplates(t *testing.T, rt *Router, cookie *http.Cookie, query string) []serviceTemplateListItem {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates"+query, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET templates%s = %d, body = %s", query, rec.Code, rec.Body.String())
	}
	var out []serviceTemplateListItem
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return out
}

func TestListServiceTemplates_Filters(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	all := listTemplates(t, rt, cookie, "")
	if len(all) < 100 {
		t.Fatalf("unfiltered list has %d entries, want the whole catalog", len(all))
	}

	tests := []struct {
		name  string
		query string
		check func(t *testing.T, got []serviceTemplateListItem)
	}{
		{"search by name", "?q=GITEA", func(t *testing.T, got []serviceTemplateListItem) {
			if len(got) == 0 {
				t.Fatal("no gitea results")
			}
			var found bool
			for _, it := range got {
				if it.ID == "gitea-with-postgresql" {
					found = true
				}
			}
			if !found {
				t.Errorf("gitea-with-postgresql missing from %d results", len(got))
			}
		}},
		{"category", "?category=monitoring", func(t *testing.T, got []serviceTemplateListItem) {
			if len(got) == 0 {
				t.Fatal("no monitoring results")
			}
			for _, it := range got {
				if it.Category != "Monitoring" {
					t.Errorf("%s has category %q", it.ID, it.Category)
				}
			}
		}},
		{"installable only", "?installable=true", func(t *testing.T, got []serviceTemplateListItem) {
			if len(got) < 60 {
				t.Fatalf("got %d installable templates, want at least 60", len(got))
			}
			for _, it := range got {
				if !it.Installable {
					t.Errorf("%s is not installable", it.ID)
				}
			}
		}},
		{"combined", "?q=wiki&installable=true&category=applications", func(t *testing.T, got []serviceTemplateListItem) {
			for _, it := range got {
				if !it.Installable || it.Category != "Applications" {
					t.Errorf("%+v leaked through the filters", it)
				}
			}
		}},
		{"no match", "?q=zzzz-no-such-template", func(t *testing.T, got []serviceTemplateListItem) {
			if len(got) != 0 {
				t.Errorf("got %d results, want none", len(got))
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, listTemplates(t, rt, cookie, tt.query))
		})
	}
}

func TestGetServiceTemplate_DescribesTheStack(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates/gitea-with-postgresql", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var detail serviceTemplateDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(detail.Stack) != 2 {
		t.Fatalf("stack = %+v, want gitea and postgresql", detail.Stack)
	}
	if detail.Manifest == nil || detail.Manifest.Source == nil || detail.Manifest.Source.License == "" {
		t.Errorf("manifest source attribution missing: %+v", detail.Manifest)
	}
}

func TestServiceTemplateUpdates(t *testing.T) {
	hub := &fakeDockerHubClient{tags: map[string][]dockerhub.Tag{
		"library/postgres": {{Name: "16-alpine"}, {Name: "17-alpine"}, {Name: "18-alpine"}, {Name: "latest"}, {Name: "17"}},
	}}
	rt, db := newTestRouterWithComposeSecrets(t)
	rt.dockerHubClient = hub
	cookie := loginTestSession(t, rt, db)

	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/wikijs/install",
		`{"name":"wiki","domains":{"wiki":"wiki.example.com"}}`, http.StatusOK)

	var out struct {
		Apps []templateUpdateApp `json:"apps"`
	}
	if err := json.Unmarshal(composeDo(t, rt, cookie, http.MethodGet, "/api/v1/service-templates/updates", "", http.StatusOK), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Apps) != 1 || out.Apps[0].App != "wiki" || out.Apps[0].TemplateID != "wikijs" {
		t.Fatalf("apps = %+v", out.Apps)
	}
	byService := map[string]templateUpdateService{}
	for _, s := range out.Apps[0].Services {
		byService[s.Service] = s
	}
	db1 := byService["db"]
	if db1.Status != templateUpdateNewer || len(db1.NewerTags) != 2 || db1.NewerTags[0] != "18-alpine" {
		t.Errorf("db = %+v, want newer 18-alpine and 17-alpine", db1)
	}
	if wiki := byService["wiki"]; wiki.Status != templateUpdateUnchecked || wiki.Note == "" {
		t.Errorf("wiki = %+v, want an unchecked non-Hub image with a note", wiki)
	}
}
