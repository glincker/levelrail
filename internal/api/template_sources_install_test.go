package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func seedSourceEntry(t *testing.T, db *store.DB, id, compose, hash string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.GetTemplateSource(ctx, "acme"); err != nil {
		if err := db.CreateTemplateSource(ctx, store.TemplateSource{Name: "acme", GitURL: "https://git.example.com/acme/t.git"}); err != nil {
			t.Fatal(err)
		}
	}
	e := store.TemplateSourceEntry{Source: "acme", ID: "acme/" + id, Name: "T", TemplateYAML: capTplManifest, ComposeYAML: compose, ContentHash: hash}
	if err := db.ReplaceTemplateSourceEntries(ctx, "acme", "sha", []store.TemplateSourceEntry{e}); err != nil {
		t.Fatal(err)
	}
}

func TestInstallSourceTemplateUsesBuiltinGuards(t *testing.T) {
	rt, db := newTestRouterWithComposeSecrets(t)
	rt.runtimeSettings = runtimesettings.New(db, func(k string) (string, bool) {
		return "true", k == "APP_TEMPLATE_SOURCES_ENABLED"
	})
	cookie := loginTestSession(t, rt, db)
	path := "/api/v1/service-templates/" + url.PathEscape("acme/captpl") + "/install"
	const web = "services:\n  web:\n    image: nginx:1.27.0\n    ports: [\"80\"]\n"
	tests := []struct {
		name, compose string
		want          int
	}{
		{"clean template installs", web, http.StatusOK},
		{"docker socket mount refused", web + "    volumes: [\"/var/run/docker.sock:/var/run/docker.sock\"]\n", http.StatusUnprocessableEntity},
		{"ungranted capability refused", web + "    cap_add: [NET_ADMIN]\n", http.StatusUnprocessableEntity},
		{"bind trick volume refused", web + "    volumes: [\"data:/data\"]\nvolumes:\n  data:\n    driver_opts: {type: none, o: bind, device: /etc}\n", http.StatusUnprocessableEntity},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seedSourceEntry(t, db, "captpl", tc.compose, "h")
			if err := db.ApproveTemplateSourceEntry(context.Background(), "acme", "acme/captpl", "h"); err != nil {
				t.Fatal(err)
			}
			composeDo(t, rt, cookie, http.MethodPost, path, `{"name":"src`+fmt.Sprint(i)+`x","domains":{"web":"cap.example.com"}}`, tc.want)
		})
	}
	t.Run("changed entry needs review", func(t *testing.T) {
		seedSourceEntry(t, db, "captpl", web, "h2")
		composeDo(t, rt, cookie, http.MethodPost, path, `{"name":"srcz"}`, http.StatusConflict)
	})
}
