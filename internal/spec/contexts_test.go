package spec

import (
	"strings"
	"testing"
)

func TestParseContexts(t *testing.T) {
	const head = "version: 1\nservices:\n  web:\n"
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"valid static", "    build: {type: static, command: make, publish: dist}\n    contexts:\n      deploy-preview:\n        env: {API: stg}\n        build: {command: make preview, publish: out}\n      branch:release/*:\n        env: {API: rel}\n", ""},
		{"valid docker args", "    build: {type: dockerfile, path: Dockerfile}\n    port: 80\n    contexts:\n      branch-deploy:\n        build: {args: {NODE: \"22\"}}\n", ""},
		{"production rejected", "    build: {type: dockerfile, path: Dockerfile}\n    port: 80\n    contexts:\n      production:\n        env: {A: b}\n", "does not match pattern"},
		{"unknown context", "    build: {type: dockerfile, path: Dockerfile}\n    port: 80\n    contexts:\n      staging:\n        env: {A: b}\n", "does not match pattern"},
		{"command on dockerfile", "    build: {type: dockerfile, path: Dockerfile}\n    port: 80\n    contexts:\n      deploy-preview:\n        build: {command: make, publish: dist}\n", "only meaningful"},
		{"bad publish", "    build: {type: static, command: make, publish: dist}\n    contexts:\n      deploy-preview:\n        build: {publish: ../x}\n", "build.publish"},
		{"bad arg", "    build: {type: dockerfile, path: Dockerfile}\n    port: 80\n    contexts:\n      deploy-preview:\n        build: {args: {\"A B\": x}}\n", "not a valid variable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(head + tt.body))
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
