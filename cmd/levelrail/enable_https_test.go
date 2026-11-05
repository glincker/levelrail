package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRunEnableHTTPS(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "levelrail.db")
	open := func(ctx context.Context) (*store.DB, error) { return store.Open(ctx, dbPath) }
	tests := []struct {
		name        string
		args        []string
		wantErr     string
		wantDir     string
		wantEnabled bool
	}{
		{name: "missing email", args: []string{"--domain", "app.mydomain.dev"}, wantErr: "usage"},
		{name: "ip is refused", args: []string{"--domain", "203.0.113.5", "--email", "ops@mydomain.dev"}, wantErr: "cannot get a public certificate"},
		{name: "staging", args: []string{"--domain", "App.Mydomain.dev", "--email", "ops@mydomain.dev", "--staging"}, wantDir: api.ACMEStagingDirectoryURL, wantEnabled: true},
		{name: "production", args: []string{"--domain", "app.mydomain.dev", "--email", "ops@mydomain.dev"}, wantEnabled: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runEnableHTTPS(ctx, tc.args, &out, open)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			db, err := open(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			got, err := db.GetIngressSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got.PrimaryDomain != "app.mydomain.dev" || got.ACMEEnabled != tc.wantEnabled || got.ACMEDirectoryURL != tc.wantDir {
				t.Fatalf("settings = %+v", got)
			}
		})
	}
}
