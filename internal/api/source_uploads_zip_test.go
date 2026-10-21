package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type zipTestFile struct {
	name, body string
	mode       fs.FileMode
}

func zipBytes(t *testing.T, files []zipTestFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: zip.Deflate}
		if f.mode != 0 {
			h.SetMode(f.mode)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(f.body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func postZip(rt *Router, cookie *http.Cookie, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/uploads", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/zip")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestUploadZip_PublishesStaticSite(t *testing.T) {
	rt, db, fb, cookie := newUploadRouter(t)
	zb := zipBytes(t, []zipTestFile{
		{name: "mysite/index.html", body: "<h1>hi</h1>"},
		{name: "mysite/_redirects", body: "/old /new 301"},
		{name: "__MACOSX/mysite/._index.html", body: "junk"},
	})
	rec := postZip(rt, cookie, zb)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var up sourceUploadResource
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	if up.FileCount != 2 || up.HasIndex == nil || !*up.HasIndex || up.SiteRoot != "mysite" {
		t.Fatalf("upload = %+v", up)
	}

	fb.release = make(chan struct{})
	_, resp := postTriggerBuildAccepted(t, rt, cookie, `{"upload_hash":"`+up.Hash+`","build":{"type":"static"}}`)
	req := fb.awaitCall(t)
	if b, err := os.ReadFile(filepath.Join(req.SourceDir, "index.html")); err != nil || string(b) != "<h1>hi</h1>" {
		t.Errorf("extracted index.html = %q, %v", b, err)
	}
	if req.Service.Build.Type != "static" {
		t.Errorf("build type = %q, want static", req.Service.Build.Type)
	}
	close(fb.release)
	if a := awaitDeployAttemptFinished(t, db, resp.ID); a.Source != store.DeployAttemptSourceUpload {
		t.Errorf("attempt source = %q, want upload", a.Source)
	}
}

func TestUploadZip_Rejections(t *testing.T) {
	rt, _, _, cookie := newUploadRouter(t)
	t.Setenv(envUploadMaxFiles, "2")
	tests := []struct {
		name   string
		body   []byte
		status int
		msg    string
	}{
		{"not a zip", []byte("hello"), http.StatusBadRequest, "could not read the zip"},
		{"zip slip", zipBytes(t, []zipTestFile{{name: "../evil", body: "x"}}), http.StatusBadRequest, "zip rejected"},
		{"symlink", zipBytes(t, []zipTestFile{{name: "l", body: "/etc/passwd", mode: fs.ModeSymlink | 0o777}}), http.StatusBadRequest, "zip rejected"},
		{"too many files", zipBytes(t, []zipTestFile{{name: "a", body: "1"}, {name: "b", body: "2"}, {name: "c", body: "3"}}), http.StatusRequestEntityTooLarge, "too large"},
		{"only metadata", zipBytes(t, []zipTestFile{{name: ".DS_Store", body: "x"}}), http.StatusBadRequest, "no files"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := postZip(rt, cookie, tc.body)
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.msg) {
				t.Fatalf("status = %d body = %s, want %d containing %q", rec.Code, rec.Body.String(), tc.status, tc.msg)
			}
		})
	}
}

func TestUploadZip_OverZipSizeCap(t *testing.T) {
	rt, _, _, cookie := newUploadRouter(t)
	zb := zipBytes(t, []zipTestFile{{name: "index.html", body: strings.Repeat("x", 4000)}})
	t.Setenv(envUploadMaxBytes, "100")
	if rec := postZip(rt, cookie, zb); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body = %s", rec.Code, rec.Body.String())
	}
}
