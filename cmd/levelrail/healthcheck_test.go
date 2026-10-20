package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunHealthcheck(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "healthy", statusCode: http.StatusOK, wantErr: false},
		{name: "server error", statusCode: http.StatusInternalServerError, wantErr: true},
		{name: "not found", statusCode: http.StatusNotFound, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			t.Setenv("APP_HTTP_ADDR", strings.TrimPrefix(srv.URL, "http://"))

			var out bytes.Buffer
			err := runHealthcheck(context.Background(), &out, false)
			if (err != nil) != tt.wantErr {
				t.Fatalf("runHealthcheck() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !strings.Contains(out.String(), "ok") {
				t.Errorf("runHealthcheck() output = %q, want it to contain %q", out.String(), "ok")
			}
		})
	}
}

func TestRunHealthcheck_Unreachable(t *testing.T) {
	t.Setenv("APP_HTTP_ADDR", "127.0.0.1:1")
	var out bytes.Buffer
	if err := runHealthcheck(context.Background(), &out, false); err == nil {
		t.Fatal("runHealthcheck() with an unreachable address: want error, got nil")
	}
}

func TestRunHealthcheck_ReadyPath(t *testing.T) {
	tests := []struct {
		name       string
		ready      bool
		wantPath   string
		statusCode int
		wantErr    bool
	}{
		{"liveness path", false, "/api/v1/brand", http.StatusOK, false},
		{"ready path ok", true, "/readyz", http.StatusOK, false},
		{"ready path 503", true, "/readyz", http.StatusServiceUnavailable, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()
			t.Setenv("APP_HTTP_ADDR", strings.TrimPrefix(srv.URL, "http://"))
			var out bytes.Buffer
			err := runHealthcheck(context.Background(), &out, tt.ready)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
		})
	}
}
