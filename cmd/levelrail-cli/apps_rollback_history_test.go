package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsRollback_DeployHistoryTag(t *testing.T) {
	const history = `[{"id":"dep_new","image":"shop/web:latest","image_digest":"sha256:bbb","started_at":"2026-10-12T10:00:00Z"},` +
		`{"id":"dep_old","image":"shop/web:plain","image_digest":"sha256:aaa","started_at":"2026-10-12T09:00:00Z"}]`
	tests := []struct {
		name       string
		image      string
		wantExit   int
		wantImage  string
		wantStderr string
	}{
		{"history tag is accepted and pinned", "shop/web:plain", exitOK, "shop/web@sha256:aaa", ""},
		{"unknown tag names a real deploy id", "shop/web:nope", exitValidation, "", "apps deploys rollback-to web dep_new"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody deployTriggerRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/images"):
					_, _ = w.Write([]byte(`[{"tag":"shop/web@sha256:bbb"}]`))
				case r.Method == http.MethodGet:
					_, _ = w.Write([]byte(history))
				default:
					_ = json.NewDecoder(r.Body).Decode(&gotBody)
					_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: gotBody.Image, Port: 3000})
				}
			}))
			defer srv.Close()

			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"apps", "rollback", "web", "--image", tt.image, "--api-url", srv.URL}, &stdout, &stderr, envMap())
			if got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tt.wantExit, stderr.String())
			}
			if gotBody.Image != tt.wantImage {
				t.Errorf("deployed image = %q, want %q", gotBody.Image, tt.wantImage)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
