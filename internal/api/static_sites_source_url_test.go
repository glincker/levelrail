package api

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/kit/netguard"
)

func zipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	return buf.Bytes()
}

func symlinkArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777})
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

// serveArchive starts a TLS server for body and points the download client at it.
func serveArchive(t *testing.T, body []byte) string {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	t.Cleanup(srv.Close)
	old := pagesURLClient
	pagesURLClient = srv.Client
	t.Cleanup(func() { pagesURLClient = old })
	t.Setenv(EnvPagesURLAllowPrivate, "true")
	return srv.URL + "/site"
}

func newURLSiteRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithBuilder(newFakeBuilder("web:tag", nil)), WithDataDir(t.TempDir()))
	return rt, db, loginTestSession(t, rt, db)
}

func createFromURL(rt *Router, cookie *http.Cookie, t *testing.T, name, src string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "source_url": src})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites", string(body)))
	return rec
}

func TestCreateStaticSiteFromURL(t *testing.T) {
	cases := map[string][]byte{
		"zip": zipArchive(t, map[string]string{"index.html": "<h1>hi</h1>"}),
		"tgz": uploadArchive(t, map[string]string{"index.html": "<h1>hi</h1>"}),
	}
	for kind, arc := range cases {
		t.Run(kind, func(t *testing.T) {
			rt, db, cookie := newURLSiteRouter(t)
			rec := createFromURL(rt, cookie, t, "urlsite", serveArchive(t, arc))
			if rec.Code != http.StatusCreated {
				t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
			}
			var got createStaticSiteResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.DeployID == "" {
				t.Fatalf("want a deploy id, got %s", rec.Body.String())
			}
			if _, err := db.GetDesiredService(t.Context(), "urlsite"); err != nil {
				t.Fatalf("site missing: %v", err)
			}
		})
	}
}

func TestCreateStaticSiteFromURLRejectsUnsafeArchives(t *testing.T) {
	cases := map[string][]byte{
		"tar traversal":  uploadArchive(t, map[string]string{"../evil.html": "x"}),
		"tar absolute":   uploadArchive(t, map[string]string{"/etc/cron.d/x": "x"}),
		"tar symlink":    symlinkArchive(t),
		"zip traversal":  zipArchive(t, map[string]string{"../evil.html": "x"}),
		"not an archive": []byte("just text, not an archive"),
	}
	for name, arc := range cases {
		t.Run(name, func(t *testing.T) {
			rt, db, cookie := newURLSiteRouter(t)
			rec := createFromURL(rt, cookie, t, "badsite", serveArchive(t, arc))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; %s", rec.Code, rec.Body.String())
			}
			if _, err := db.GetDesiredService(t.Context(), "badsite"); err == nil {
				t.Fatal("a rejected archive must not leave a site behind")
			}
		})
	}
}

func TestCreateStaticSiteFromURLRejectsBadURLs(t *testing.T) {
	rt, _, cookie := newURLSiteRouter(t)
	for _, src := range []string{
		"http://example.com/site.zip",
		"https://user:pw@example.com/site.zip",
		"https://127.0.0.1/site.zip",
		"https://169.254.169.254/latest/meta-data",
		"https://[::1]/site.zip",
		"https://10.0.0.5/site.zip",
		"file:///etc/passwd",
	} {
		if rec := createFromURL(rt, cookie, t, "ssrf", src); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", src, rec.Code)
		}
	}
}

func TestCreateStaticSiteFromURLBlocksLoopbackAtDial(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("x")) }))
	t.Cleanup(srv.Close)
	rt, _, cookie := newURLSiteRouter(t)
	// localhost resolves to loopback, so only the connection guard can stop it.
	src := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1) + "/site.zip"
	if rec := createFromURL(rt, cookie, t, "dialblock", src); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateStaticSiteFromURLRefusesRedirectToHTTP(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("x")) }))
	t.Cleanup(plain.Close)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	old := pagesURLClient
	pagesURLClient = srv.Client
	t.Cleanup(func() { pagesURLClient = old })
	t.Setenv(EnvPagesURLAllowPrivate, "true")
	rt, _, cookie := newURLSiteRouter(t)
	if rec := createFromURL(rt, cookie, t, "redir", srv.URL+"/x"); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateStaticSiteFromURLSizeCap(t *testing.T) {
	t.Setenv(EnvPagesURLMaxMB, "1")
	big := bytes.Repeat([]byte{'a'}, 2<<20)
	rt, _, cookie := newURLSiteRouter(t)
	if rec := createFromURL(rt, cookie, t, "bigsite", serveArchive(t, big)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestFromURLNotilWidenedByNotifyOptIn(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("x")) }))
	t.Cleanup(srv.Close)
	t.Setenv(netguard.AllowPrivateEnv, "true")
	t.Setenv(EnvPagesURLAllowPrivate, "")
	rt, _, cookie := newURLSiteRouter(t)
	src := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1) + "/site.zip"
	if rec := createFromURL(rt, cookie, t, "notify-optin", src); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestFromURLErrorDetailDropsQuery(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://h/x?sig=secret", Err: errors.New("connection refused")}
	if got := fetchErrDetail(err); strings.Contains(got, "secret") || got != "connection refused" {
		t.Fatalf("detail = %q", got)
	}
}

func TestFromURLExistingAppStagesNothing(t *testing.T) {
	hits := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits++; _, _ = w.Write([]byte("x")) }))
	t.Cleanup(srv.Close)
	old := pagesURLClient
	pagesURLClient = srv.Client
	t.Cleanup(func() { pagesURLClient = old })
	t.Setenv(EnvPagesURLAllowPrivate, "true")
	rt, _, cookie := newURLSiteRouter(t)
	first := httptest.NewRecorder()
	rt.Handler().ServeHTTP(first, authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites", `{"name":"dupe"}`))
	if first.Code != http.StatusCreated {
		t.Fatalf("seed = %d %s", first.Code, first.Body.String())
	}
	if rec := createFromURL(rt, cookie, t, "dupe", srv.URL+"/a.zip"); rec.Code != http.StatusConflict || hits != 0 {
		t.Fatalf("status = %d hits = %d, want 409 and no download", rec.Code, hits)
	}
}

func TestFromURLRefusesNonStaticApp(t *testing.T) {
	rt, _, cookie := newURLSiteRouter(t)
	seed := httptest.NewRecorder()
	rt.Handler().ServeHTTP(seed, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"plain","image":"nginx:1","port":80}`))
	if seed.Code >= 300 {
		t.Fatalf("seed = %d %s", seed.Code, seed.Body.String())
	}
	body := `{"source_url":"https://example.com/a.zip","build":{"type":"static"}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/plain/builds", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s, want 400", rec.Code, rec.Body.String())
	}
}
