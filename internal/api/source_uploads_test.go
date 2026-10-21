package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func uploadArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Size: int64(len(body)), Mode: 0o644}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func newUploadRouter(t *testing.T) (*Router, *store.DB, *fakeBuilder, *http.Cookie) {
	t.Helper()
	fb := newFakeBuilder("web:tag", nil)
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithBuilder(fb), WithDataDir(t.TempDir()))
	seedWebApp(t, db)
	return rt, db, fb, loginTestSession(t, rt, db)
}

func postUpload(rt *Router, cookie *http.Cookie, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/uploads", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestUploadSource_StoresDedupesAndBuilds(t *testing.T) {
	rt, db, fb, cookie := newUploadRouter(t)
	arc := uploadArchive(t, map[string]string{"Dockerfile": "FROM scratch", "main.go": "package main"})

	rec := postUpload(rt, cookie, arc)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var up sourceUploadResource
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil || up.FileCount != 2 || len(up.Hash) != 64 {
		t.Fatalf("upload = %+v, %v", up, err)
	}
	if again := postUpload(rt, cookie, arc); again.Code != http.StatusOK || !strings.Contains(again.Body.String(), `"deduped":true`) {
		t.Fatalf("second upload = %d %s, want 200 deduped", again.Code, again.Body.String())
	}

	fb.release = make(chan struct{})
	_, resp := postTriggerBuildAccepted(t, rt, cookie, `{"upload_hash":"`+up.Hash+`","build":{"type":"dockerfile"}}`)
	req := fb.awaitCall(t)
	if req.CommitSHA != up.Hash[:40] {
		t.Errorf("CommitSHA = %q, want the upload hash prefix", req.CommitSHA)
	}
	if b, err := os.ReadFile(filepath.Join(req.SourceDir, "main.go")); err != nil || string(b) != "package main" {
		t.Errorf("extracted main.go = %q, %v", b, err)
	}
	close(fb.release)
	a := awaitDeployAttemptFinished(t, db, resp.ID)
	if a.Source != store.DeployAttemptSourceUpload || a.CommitSHA != up.Hash[:40] {
		t.Errorf("attempt source=%q commit=%q", a.Source, a.CommitSHA)
	}
	if !strings.Contains(a.CommitMessage, "Uploaded from CLI, 2 files, "+up.Hash[:12]) {
		t.Errorf("commit message = %q", a.CommitMessage)
	}
}

func TestUploadSource_RejectsBadInput(t *testing.T) {
	rt, _, _, cookie := newUploadRouter(t)
	t.Setenv(envUploadMaxBytes, "200")
	tests := []struct {
		name   string
		body   []byte
		status int
	}{
		{"not an archive", []byte("hello"), http.StatusBadRequest},
		{"path traversal", uploadArchive(t, map[string]string{"../evil": "x"}), http.StatusBadRequest},
		{"absolute path", uploadArchive(t, map[string]string{"/etc/x": "x"}), http.StatusBadRequest},
		{"over the size cap", uploadArchive(t, map[string]string{"a": strings.Repeat("abcdefghij", 5000)}), http.StatusRequestEntityTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if rec := postUpload(rt, cookie, tc.body); rec.Code != tc.status {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}

func TestUploadSource_RequiresDeployAbility(t *testing.T) {
	rt, db, _, _ := newUploadRouter(t)
	const plaintext = "read-only-upload-token" //nolint:gosec // fake fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_ro", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/uploads", bytes.NewReader(uploadArchive(t, map[string]string{"a": "x"})))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	anon := httptest.NewRecorder()
	rt.Handler().ServeHTTP(anon, httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/uploads", strings.NewReader("x")))
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anon.Code)
	}
}

func TestTriggerBuild_UploadHashErrors(t *testing.T) {
	rt, _, _, cookie := newUploadRouter(t)
	missing := strings.Repeat("a", 64)
	tests := []struct {
		name, body string
		status     int
	}{
		{"unknown hash", `{"upload_hash":"` + missing + `"}`, http.StatusNotFound},
		{"traversal hash", `{"upload_hash":"../../etc/passwd"}`, http.StatusBadRequest},
		{"with repo url", `{"upload_hash":"` + missing + `","repo_url":"https://example.com/a.git","ref":"main"}`, http.StatusBadRequest},
		{"image build", `{"upload_hash":"` + missing + `","build":{"type":"image","image":"nginx"}}`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", tc.body))
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}

func TestUploadSource_RetentionCleanupAndListing(t *testing.T) {
	rt, _, _, cookie := newUploadRouter(t)
	rec := postUpload(rt, cookie, uploadArchive(t, map[string]string{"a": "x"}))
	var up sourceUploadResource
	_ = json.Unmarshal(rec.Body.Bytes(), &up)

	list := httptest.NewRecorder()
	rt.Handler().ServeHTTP(list, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/uploads", ""))
	var lr sourceUploadListResponse
	if err := json.Unmarshal(list.Body.Bytes(), &lr); err != nil || len(lr.Items) != 1 || lr.UsageBytes <= 0 {
		t.Fatalf("list = %+v, %v", lr, err)
	}

	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(rt.dataDir, "uploads", "web", up.Hash+".tar.gz"), old, old); err != nil {
		t.Fatal(err)
	}
	rt.CleanupUploads(context.Background())
	list = httptest.NewRecorder()
	rt.Handler().ServeHTTP(list, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/uploads", ""))
	_ = json.Unmarshal(list.Body.Bytes(), &lr)
	if len(lr.Items) != 0 {
		t.Fatalf("expired upload still listed: %+v", lr)
	}
}
