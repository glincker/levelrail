package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GLINCKER/levelrail/internal/brand"
)

func gtmHandler(id string) http.Handler {
	return HandlerFromFS(fstest.MapFS{
		"dist/index.html": &fstest.MapFile{Data: []byte(shell)},
	}, &brand.Brand{Name: "Acme"}, HeadOptions{Title: "Demo", GTMID: id})
}

func getIndex(h http.Handler, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGTMHead_RenderedWithInitBeforeLoader(t *testing.T) {
	rec := getIndex(gtmHandler("GTM-ABC123"), nil)
	body := rec.Body.String()
	init := strings.Index(body, `<script src="/gtm-init.js"></script>`)
	loader := strings.Index(body, `<script async src="https://www.googletagmanager.com/gtm.js?id=GTM-ABC123"></script>`)
	if init < 0 || loader < 0 || init > loader {
		t.Fatalf("want init script before async loader, got %q", body)
	}
	if strings.Contains(body, "<script>") {
		t.Errorf("no inline script allowed: %q", body)
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Sec-GPC") {
		t.Errorf("Vary = %q, want DNT and Sec-GPC", rec.Header().Get("Vary"))
	}
}

func TestGTMHead_OmittedForOptOutAndUnset(t *testing.T) {
	for name, tc := range map[string]struct {
		id  string
		hdr map[string]string
	}{
		"dnt":   {"GTM-ABC123", map[string]string{"DNT": "1"}},
		"gpc":   {"GTM-ABC123", map[string]string{"Sec-GPC": "1"}},
		"unset": {"", nil},
	} {
		body := getIndex(gtmHandler(tc.id), tc.hdr).Body.String()
		if strings.Contains(body, "googletagmanager") || strings.Contains(body, "gtm-init") {
			t.Errorf("%s: tag must be omitted: %q", name, body)
		}
	}
}

func TestGTMInitScript_SeedsDataLayerOnly(t *testing.T) {
	if !strings.Contains(GTMInitScript, "'gtm.start'") || !strings.Contains(GTMInitScript, "event: 'gtm.js'") {
		t.Errorf("init script = %q", GTMInitScript)
	}
	if strings.Contains(GTMInitScript, "createElement") {
		t.Errorf("init script must not inject the loader: %q", GTMInitScript)
	}
}
