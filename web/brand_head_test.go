package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GLINCKER/levelrail/internal/brand"
)

const shell = `<html><head><meta name="description" content="x" /><meta property="og:title" content="x" /><title>Dashboard</title></head><body></body></html>`

func brandedHandler(b *brand.Brand) http.Handler {
	return handlerFromFS(fstest.MapFS{
		"dist/index.html": &fstest.MapFile{Data: []byte(shell)},
	}, b)
}

func TestManifest_UsesBrand(t *testing.T) {
	b := &brand.Brand{Name: "Acme", ShortName: "AC", PrimaryColor: "#123456"}
	rec := httptest.NewRecorder()
	brandedHandler(b).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, manifestPath, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var m webManifest
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "Acme" || m.ShortName != "AC" || m.ThemeColor != "#123456" || len(m.Icons) == 0 {
		t.Errorf("manifest = %+v", m)
	}
}

func TestManifest_NilBrandIsNeutral(t *testing.T) {
	rec := httptest.NewRecorder()
	brandedHandler(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, manifestPath, nil))
	var m webManifest
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != neutralTitle {
		t.Errorf("name = %q", m.Name)
	}
}

func TestIndex_InjectsBrandHead(t *testing.T) {
	b := &brand.Brand{Name: "A<c>me", PrimaryColor: "#123456"}
	req := httptest.NewRequest(http.MethodGet, "/apps/x", nil)
	req.Host = "dash.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	brandedHandler(b).ServeHTTP(rec, req)

	body := rec.Body.String()
	if n := strings.Count(body, `property="og:title"`); n != 1 {
		t.Errorf("og:title count = %d, want 1", n)
	}
	for _, want := range []string{
		"<title>A&lt;c&gt;me</title>",
		`og:image" content="https://dash.example.com/apple-touch-icon.png"`,
		`og:site_name" content="A&lt;c&gt;me"`,
		`theme-color" content="#123456"`,
		`twitter:card`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestIndex_NilBrandUnchanged(t *testing.T) {
	rec := httptest.NewRecorder()
	brandedHandler(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Body.String() != shell {
		t.Errorf("body = %q", rec.Body.String())
	}
}
