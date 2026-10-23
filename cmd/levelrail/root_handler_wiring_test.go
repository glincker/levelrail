package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Guards against main.go forgetting to pass the git-provider secrets option, which router tests cannot catch.
func TestRootHandler_GitProviderAppSecretsWired(t *testing.T) {
	dataDir := t.TempDir()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(dataDir, "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	secretsManager, masterKeyFilePath, err := loadSecretsManager(db, dataDir)
	if err != nil {
		t.Fatalf("loadSecretsManager() error = %v", err)
	}

	token, _, err := api.MintAPIToken(ctx, db, "test", []string{api.AbilityRoot}, nil)
	if err != nil {
		t.Fatalf("MintAPIToken() error = %v", err)
	}

	// Keeps rootHandler from trying to dial its own loopback address to
	// mint the AI assistant's self-call token: irrelevant here and
	// otherwise a real (failing) network call on every test run.
	t.Setenv("APP_HTTP_ADDR", "not-a-host-port")

	b := &brand.Brand{Name: "test", ShortName: "test", BinaryName: "levelrail"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, _ := rootHandler(rootDeps{logger: logger, b: b, db: db, secretsManager: secretsManager, masterKeyFilePath: masterKeyFilePath})

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"github app manual connect", http.MethodPut, "/api/v1/github-app/manual"},
		{"git provider connection create", http.MethodPost, "/api/v1/git-connections"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotImplemented && strings.Contains(rec.Body.String(), "requires a master key") {
				t.Fatalf("%s %s returned %d %q: the App secrets dependency is still nil, rootHandler isn't wiring it into the real router", tc.method, tc.path, rec.Code, rec.Body.String())
			}
		})
	}
}
