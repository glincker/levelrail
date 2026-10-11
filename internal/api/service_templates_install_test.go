package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newTestRouterWithComposeSecrets(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithComposeSecrets(secrets.NewManager(db, mk))), db
}

func TestInstallServiceTemplate(t *testing.T) {
	rt, db := newTestRouterWithComposeSecrets(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	tests := []struct {
		name string
		path string
		body string
		want int
	}{
		{"unknown template", "/api/v1/service-templates/nope/install", `{}`, http.StatusNotFound},
		{"bad name", "/api/v1/service-templates/pairdrop/install", `{"name":"Bad Name"}`, http.StatusBadRequest},
		{"unknown setting", "/api/v1/service-templates/pairdrop/install", `{"name":"drop","env":{"NOPE":"x"}}`, http.StatusBadRequest},
		{"bad domain", "/api/v1/service-templates/pairdrop/install", `{"name":"drop","domains":{"pairdrop":"not a host"}}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			composeDo(t, rt, cookie, http.MethodPost, tt.path, tt.body, tt.want)
		})
	}

	var resp composeDeployResponse
	_ = json.Unmarshal(composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/wikijs/install",
		`{"name":"wiki","domains":{"wiki":"wiki.example.com"}}`, http.StatusOK), &resp)
	if resp.AppID != "wiki" || len(resp.Services) != 2 || resp.Revision != 1 {
		t.Fatalf("install = %+v", resp)
	}
	wiki, err := db.GetDesiredService(ctx, "wiki-wiki")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if strings.Join(wiki.Domains, ",") != "wiki.example.com" || strings.Join(wiki.DependsOnHealthy, ",") != "db" {
		t.Errorf("wiki service = domains %v healthy deps %v", wiki.Domains, wiki.DependsOnHealthy)
	}
	if _, ok := wiki.Env["DB_PASS"]; ok {
		t.Error("generated DB_PASS landed in plain env, want it secret-backed")
	}
	rev, err := db.GetComposeRevision(ctx, "wiki", 0)
	if err != nil || rev.Source != store.ComposeRevisionSourceTemplate || rev.TemplateID != "wikijs" {
		t.Errorf("revision = %+v, %v", rev, err)
	}
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/wikijs/install", `{"name":"wiki"}`, http.StatusConflict)

	var detail serviceTemplateDetail
	_ = json.Unmarshal(composeDo(t, rt, cookie, http.MethodGet, "/api/v1/service-templates/wikijs", "", http.StatusOK), &detail)
	if detail.Manifest == nil || len(detail.Manifest.Env) != 1 || detail.Manifest.Env[0].Generate != "password" {
		t.Errorf("detail manifest = %+v", detail.Manifest)
	}
}
