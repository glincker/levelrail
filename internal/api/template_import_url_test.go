package api

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func tlsTestOpts(srv *httptest.Server, maxBytes int) templateFetchOpts {
	cfg := srv.Client().Transport.(*http.Transport).TLSClientConfig
	return templateFetchOpts{MaxBytes: maxBytes, Timeout: 5 * time.Second, AllowPrivate: true, TLSConfig: cfg}
}

func TestTemplateImportURLFetch(t *testing.T) {
	const compose = "services:\n  web:\n    image: nginx:1\n"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			http.Error(w, "credentials sent", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte(compose))
		case "/big":
			_, _ = w.Write([]byte(strings.Repeat("a", 5000)))
		case "/redir":
			http.Redirect(w, r, "/ok", http.StatusFound)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	got, err := fetchTemplateImport(ctx, srv.URL+"/ok", tlsTestOpts(srv, 4096))
	if err != nil || got != compose {
		t.Fatalf("success = %q, %v", got, err)
	}
	if _, err := fetchTemplateImport(ctx, srv.URL+"/big", tlsTestOpts(srv, 4096)); !errors.Is(err, errTemplateURLTooLarge) {
		t.Fatalf("oversize = %v", err)
	}
	if _, err := fetchTemplateImport(ctx, srv.URL+"/redir", tlsTestOpts(srv, 4096)); !errors.Is(err, errTemplateURLRedirect) {
		t.Fatalf("redirect = %v", err)
	}
	if _, err := fetchTemplateImport(ctx, "http://example.com/x.yaml", templateFetchOpts{MaxBytes: 4096, Timeout: time.Second}); !errors.Is(err, errTemplateURLScheme) {
		t.Fatalf("http = %v", err)
	}
	priv := templateFetchOpts{MaxBytes: 4096, Timeout: 5 * time.Second, TLSConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // test server
	if _, err := fetchTemplateImport(ctx, srv.URL+"/ok", priv); !errors.Is(err, errTemplateURLBlocked) {
		t.Fatalf("private = %v", err)
	}
}

func TestTemplateImportURLHandler(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("services:\n  web:\n    image: nginx:1\n"))
	}))
	defer srv.Close()
	rec := httptest.NewRecorder()
	body := `{"id":"from-url","name":"From url","url":"` + srv.URL + `/x"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/templates/custom", body))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("loopback url = %d: %s", rec.Code, rec.Body.String())
	}
	both := `{"id":"from-url","name":"x","url":"https://example.com/a","compose_yaml":"services: {}"}`
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/templates/custom", both))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("both = %d", rec.Code)
	}
}
