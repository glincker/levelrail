package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/glincker/levelrail/internal/brand"
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

func demoHead() HeadOptions {
	return HeadOptions{
		Title:       "Levelrail live demo",
		Description: "Try it. Read only.",
		URL:         "https://demo.example.com/",
		ImageURL:    "https://demo.example.com/demo-og.png",
		ImageAlt:    "alt <text>",
		ImageWidth:  1200,
		ImageHeight: 630,
	}
}

func TestHeadOptions_InjectsShareTags(t *testing.T) {
	h := handlerFromFS(fstest.MapFS{
		"dist/index.html": &fstest.MapFile{Data: []byte(shell)},
	}, &brand.Brand{Name: "Acme"}, demoHead())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	for _, want := range []string{
		`<title>Levelrail live demo</title>`,
		`<meta name="description" content="Try it. Read only." />`,
		`<link rel="canonical" href="https://demo.example.com/" />`,
		`<meta property="og:url" content="https://demo.example.com/" />`,
		`<meta property="og:image" content="https://demo.example.com/demo-og.png" />`,
		`<meta property="og:image:width" content="1200" />`,
		`<meta property="og:image:height" content="630" />`,
		`<meta property="og:image:alt" content="alt &lt;text&gt;" />`,
		`<meta name="twitter:card" content="summary_large_image" />`,
		`<meta name="twitter:image" content="https://demo.example.com/demo-og.png" />`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, `content="summary"`) {
		t.Error("summary card should be replaced")
	}
}

func TestHeadOptions_ZeroValueIsUnchanged(t *testing.T) {
	b := &brand.Brand{Name: "Acme", PrimaryColor: "#123456"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	plain := injectBrandHead([]byte(shell), b, req, HeadOptions{})
	rec := httptest.NewRecorder()
	brandedHandler(b).ServeHTTP(rec, req)
	if rec.Body.String() != string(plain) {
		t.Error("default handler output differs from zero HeadOptions")
	}
	if !strings.Contains(string(plain), `content="summary"`) || strings.Contains(string(plain), "canonical") {
		t.Errorf("default head changed: %s", plain)
	}
}
