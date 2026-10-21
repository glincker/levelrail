package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestResolveStaticBuild(t *testing.T) {
	const detected = `{"static_build":{"preset_id":"vite","preset_name":"React / Vite","command":"npm run build","publish_dir":"dist"}}`
	tests := []struct {
		name        string
		buildType   string
		flags       staticBuildFlags
		detectBody  string
		wantCmd     string
		wantPublish string
		wantDetect  bool
		wantErr     string
	}{
		{"explicit pair needs no detection", "static", staticBuildFlags{Command: "make", Publish: "out"}, detected, "make", "out", false, ""},
		{"nothing asked serves as-is", "static", staticBuildFlags{}, detected, "", "", false, ""},
		{"infer fills both", "static", staticBuildFlags{Infer: true}, detected, "npm run build", "dist", true, ""},
		{"override keeps command, infers publish", "static", staticBuildFlags{Command: "pnpm build"}, detected, "pnpm build", "dist", true, ""},
		{"override keeps publish, infers command", "static", staticBuildFlags{Publish: "public"}, detected, "npm run build", "public", true, ""},
		{"not static ignores flags", "railpack", staticBuildFlags{Infer: true}, detected, "", "", false, ""},
		{"unknown framework", "static", staticBuildFlags{Infer: true}, `{}`, "", "", true, "could not infer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detects := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				detects++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.detectBody))
			}))
			defer srv.Close()

			cmd, publish, err := resolveStaticBuild(context.Background(), apiclient.NewClient(srv.URL, "tok"), "web", tt.buildType, tt.flags)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cmd != tt.wantCmd || publish != tt.wantPublish || (detects > 0) != tt.wantDetect {
				t.Fatalf("got %q %q detects=%d", cmd, publish, detects)
			}
		})
	}
}
