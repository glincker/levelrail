package api

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/githubapp"
)

func TestNormalizeGitHubAppOwner(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty is personal", in: "", want: ""},
		{name: "blank is personal", in: "   ", want: ""},
		{name: "org login", in: "acme-corp", want: "acme-corp"},
		{name: "trimmed", in: " Acme ", want: "Acme"},
		{name: "digit start", in: "42labs", want: "42labs"},
		{name: "leading hyphen", in: "-acme", wantErr: true},
		{name: "slash", in: "acme/evil", wantErr: true},
		{name: "path traversal", in: "..", wantErr: true},
		{name: "space inside", in: "ac me", wantErr: true},
		{name: "underscore", in: "ac_me", wantErr: true},
		{name: "too long", in: strings.Repeat("a", 40), wantErr: true},
		{name: "max length", in: strings.Repeat("a", 39), want: strings.Repeat("a", 39)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeGitHubAppOwner(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGitHubAppManifestActionURL(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		owner    string
		want     string
	}{
		{name: "personal", instance: "https://github.com", want: "https://github.com/settings/apps/new?state=s%2B1"},
		{name: "org", instance: "https://github.com", owner: "acme", want: "https://github.com/organizations/acme/settings/apps/new?state=s%2B1"},
		{name: "ghe personal", instance: "https://ghe.example.com", want: "https://ghe.example.com/settings/apps/new?state=s%2B1"},
		{name: "ghe org", instance: "https://ghe.example.com", owner: "acme", want: "https://ghe.example.com/organizations/acme/settings/apps/new?state=s%2B1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := githubAppManifestActionURL(tt.instance, tt.owner, "s+1"); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

var (
	formActionRe   = regexp.MustCompile(`action="([^"]+)"`)
	formManifestRe = regexp.MustCompile(`name="manifest" value="([^"]*)"`)
)

func TestHandleStartGitHubAppRegistration_OwnerAndPublic(t *testing.T) {
	tests := []struct {
		name       string
		query      url.Values
		wantAction string
		wantPublic bool
		wantStatus int
	}{
		{name: "personal private", query: url.Values{}, wantAction: "https://github.com/settings/apps/new?state=", wantStatus: http.StatusOK},
		{name: "org private", query: url.Values{"owner": {"acme"}}, wantAction: "https://github.com/organizations/acme/settings/apps/new?state=", wantStatus: http.StatusOK},
		{name: "org public", query: url.Values{"owner": {"acme"}, "public": {"true"}}, wantAction: "https://github.com/organizations/acme/settings/apps/new?state=", wantPublic: true, wantStatus: http.StatusOK},
		{name: "personal public", query: url.Values{"public": {"true"}}, wantAction: "https://github.com/settings/apps/new?state=", wantPublic: true, wantStatus: http.StatusOK},
		{name: "ghe org", query: url.Values{"owner": {"acme"}, "instance_url": {"https://ghe.example.com"}}, wantAction: "https://ghe.example.com/organizations/acme/settings/apps/new?state=", wantStatus: http.StatusOK},
		{name: "bad owner", query: url.Values{"owner": {"a/b"}}, wantStatus: http.StatusBadRequest},
		{name: "bad public", query: url.Values{"public": {"maybe"}}, wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
			cookie := loginTestSession(t, rt, db)
			setPrimaryDomain(t, db)

			rec := httptest.NewRecorder()
			target := "/api/v1/github-app/register/start?" + tt.query.Encode()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, target, ""))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			body := rec.Body.String()
			am := formActionRe.FindStringSubmatch(body)
			if am == nil || !strings.HasPrefix(html.UnescapeString(am[1]), tt.wantAction) {
				t.Errorf("form action = %v, want prefix %q", am, tt.wantAction)
			}
			mm := formManifestRe.FindStringSubmatch(body)
			if mm == nil {
				t.Fatalf("no manifest field in form: %s", body)
			}
			var m githubapp.Manifest
			if err := json.Unmarshal([]byte(html.UnescapeString(mm[1])), &m); err != nil {
				t.Fatalf("manifest is not valid json: %v", err)
			}
			if m.Public != tt.wantPublic {
				t.Errorf("manifest public = %v, want %v", m.Public, tt.wantPublic)
			}
		})
	}
}

func TestHandleGetGitHubAppManifestPreview_OwnerAndPublic(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/preview?owner=acme&public=true", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got githubAppManifestPreviewResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Owner != "acme" || !got.Public {
		t.Errorf("preview owner/public = %q/%v, want acme/true", got.Owner, got.Public)
	}

	bad := httptest.NewRecorder()
	rt.Handler().ServeHTTP(bad, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/preview?owner=..", ""))
	if bad.Code != http.StatusBadRequest {
		t.Errorf("bad owner status = %d, want 400", bad.Code)
	}
}
