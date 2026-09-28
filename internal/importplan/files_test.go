package importplan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPFilesRefusesInternalAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("FROM x"))
	}))
	defer srv.Close()
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "false")

	_, _, err := NewHTTPFiles().ReadFile(context.Background(), srv.URL+"/o/r", "", "Dockerfile")
	if err == nil {
		t.Fatal("loopback repo host must be refused by the SSRF guard")
	}
}

func TestHTTPFilesLimits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/big"):
			_, _ = w.Write([]byte(strings.Repeat("a", 100)))
		case strings.HasSuffix(r.URL.Path, "/private"):
			w.WriteHeader(http.StatusForbidden)
		case strings.HasSuffix(r.URL.Path, "/ok"):
			_, _ = w.Write([]byte("hi"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	h := &HTTPFiles{Client: srv.Client(), MaxBytes: 10, MaxFiles: 4}
	ctx := context.Background()

	if b, ok, err := h.ReadFile(ctx, srv.URL+"/o/r", "", "ok"); err != nil || !ok || string(b) != "hi" {
		t.Errorf("ok: %q %v %v", b, ok, err)
	}
	if _, ok, err := h.ReadFile(ctx, srv.URL+"/o/r", "", "missing"); err != nil || ok {
		t.Errorf("missing: %v %v", ok, err)
	}
	if _, _, err := h.ReadFile(ctx, srv.URL+"/o/r", "", "big"); err == nil {
		t.Error("oversize file must fail")
	}
	if _, _, err := h.ReadFile(ctx, srv.URL+"/o/r", "", "private"); err == nil || !strings.Contains(err.Error(), "private") {
		t.Errorf("private: %v", err)
	}
	if _, _, err := h.ReadFile(ctx, srv.URL+"/o/r", "", "ok"); err != ErrFileLimit {
		t.Errorf("file limit: %v", err)
	}
}
