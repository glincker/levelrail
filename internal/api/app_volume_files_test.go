package api

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/volumefiles"
)

// dirVolumeNode backs a node's volume with a temp directory through the real confined store.
type dirVolumeNode struct {
	bareRuntime
	dir string
}

func (n dirVolumeNode) open() (*volumefiles.Store, error) { return volumefiles.Open(n.dir) }

func (n dirVolumeNode) VolumeList(_ context.Context, _, dir string, limit int) ([]volumefiles.Entry, bool, error) {
	s, err := n.open()
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = s.Close() }()
	return s.List(dir, limit)
}

func (n dirVolumeNode) VolumeRead(_ context.Context, _, p string) (io.ReadCloser, error) {
	s, err := n.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	rc, _, err := s.Read(p)
	return rc, err
}

func (n dirVolumeNode) VolumeWrite(_ context.Context, _, p string, r io.Reader, maxBytes int64) error {
	s, err := n.open()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	_, err = s.Write(p, r, maxBytes)
	return err
}

func (n dirVolumeNode) VolumeRemove(_ context.Context, _, p string) error {
	s, err := n.open()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	return s.Remove(p)
}

func TestVolumeFilesRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/volumes/data/files"},
		{http.MethodGet, "/api/v1/apps/web/volumes/data/files/content?path=a"},
		{http.MethodPut, "/api/v1/apps/web/volumes/data/files/content?path=a"},
		{http.MethodDelete, "/api/v1/apps/web/volumes/data/files/content?path=a"},
		{http.MethodPost, "/api/v1/apps/web/volumes/data/files/restore"},
	})
}

func TestVolumeFiles_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	if rec := do(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/files", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestVolumeFiles_CRUDAndEscapes(t *testing.T) {
	base := t.TempDir()
	vol := filepath.Join(base, "vol")
	if err := os.Mkdir(vol, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "secret"), []byte("host"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "secret"), filepath.Join(vol, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	node := dirVolumeNode{dir: vol}
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return node, nil }))
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	const files = "/api/v1/apps/web/volumes/data/files"

	req := authedRequest(t, cookie, http.MethodPut, files+"/content?path=conf/app.ini", "a=1")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put = %d %s", rec.Code, rec.Body.String())
	}

	rec = do(t, rt, cookie, http.MethodGet, files+"?path=conf", "")
	var list volumeFileListResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list.Entries) != 1 || list.Entries[0].Name != "app.ini" {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}

	rec = do(t, rt, cookie, http.MethodGet, files+"/content?path=conf/app.ini", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "a=1" {
		t.Fatalf("get = %d %q", rec.Code, rec.Body.String())
	}

	for _, p := range []string{"../secret", "conf/../../secret", "link"} {
		rec = do(t, rt, cookie, http.MethodGet, files+"/content?path="+p, "")
		if rec.Code < 400 || strings.Contains(rec.Body.String(), "host") {
			t.Fatalf("GET %q = %d %q, want a refusal", p, rec.Code, rec.Body.String())
		}
		rec = do(t, rt, cookie, http.MethodDelete, files+"/content?path="+p, "")
		if p != "link" && rec.Code != http.StatusBadRequest {
			t.Fatalf("DELETE %q = %d, want 400", p, rec.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "secret")); err != nil {
		t.Fatalf("host file touched: %v", err)
	}

	rec = do(t, rt, cookie, http.MethodDelete, files+"/content?path=conf", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("delete dir = %d, want 400", rec.Code)
	}
	rec = do(t, rt, cookie, http.MethodDelete, files+"/content?path=conf/app.ini", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, rt, cookie, http.MethodGet, files+"/content?path=conf/app.ini", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete = %d, want 404", rec.Code)
	}
}

func TestVolumeFiles_UploadOverCapRefused(t *testing.T) {
	t.Setenv("APP_VOLUME_FILE_MAX_UPLOAD_MB", "1")
	vol := t.TempDir()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return dirVolumeNode{dir: vol}, nil }))
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	rec := do(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/volumes/data/files/content?path=big", strings.Repeat("x", 2<<20))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	if left, _ := os.ReadDir(vol); len(left) != 0 {
		t.Fatalf("partial file left: %v", left)
	}
}

func TestTarEntry(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, f := range []struct{ name, body string }{{"./a.txt", "one"}, {"./sub/b.txt", "two"}} {
		_ = tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(f.body))
	}
	_ = tw.Close()

	r, err := tarEntry(bytes.NewReader(buf.Bytes()), "sub/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(r); string(got) != "two" {
		t.Fatalf("got %q", got)
	}
	if _, err := tarEntry(bytes.NewReader(buf.Bytes()), "missing"); !errors.Is(err, errTarEntryMissing) {
		t.Fatalf("err = %v", err)
	}
}
