package k8sconn

import (
	"context"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func serverCreds(srv *httptest.Server) Credentials {
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	return Credentials{Server: srv.URL, CA: ca, Token: "tok"}
}

func TestClientGuardAndRedirect(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/redir" {
			http.Redirect(w, r, "https://169.254.169.254/", http.StatusFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	ctx := context.Background()
	to := 5 * time.Second

	// Loopback refused unless allowed.
	if _, err := NewClient(ctx, Config{Creds: serverCreds(srv), Timeout: to}); err == nil {
		t.Fatal("loopback accepted without AllowLoopback")
	}
	// Metadata refused even with private and loopback allowed.
	allow := trafficpolicy.EgressGuard{AllowPrivate: true, AllowLoopback: true}
	meta := Credentials{Server: "https://169.254.169.254", Token: "t"}
	if _, err := NewClient(ctx, Config{Creds: meta, Guard: allow, Timeout: to}); err == nil {
		t.Fatal("metadata address accepted")
	}
	// Plain http refused.
	if _, err := NewClient(ctx, Config{Creds: Credentials{Server: "http://10.0.0.1"}, Guard: allow}); err == nil {
		t.Fatal("http accepted")
	}

	c, err := NewClient(ctx, Config{Creds: serverCreds(srv), Guard: allow, Timeout: to})
	if err != nil {
		t.Fatal(err)
	}
	body, err := c.Do(ctx, "GET", "/version", nil)
	if err != nil || !strings.Contains(string(body), "ok") {
		t.Fatalf("get: %s %v", body, err)
	}
	before := hits.Load()
	_, err = c.Do(ctx, "GET", "/redir", nil)
	var se *StatusError
	ok := errors.As(err, &se)
	if !ok || se.Code != http.StatusFound {
		t.Fatalf("redirect not surfaced as status: %v", err)
	}
	if hits.Load() != before+1 {
		t.Error("redirect was followed")
	}
}
