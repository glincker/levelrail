package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/mailcert"
	"github.com/GLINCKER/levelrail/internal/store"
)

// The marker stands in for key bytes; no real key is needed to prove the routes refuse.
const protectedMarker = "managed-pair-bytes"

func TestVolumeFilesProtectMailManagedPair(t *testing.T) {
	vol := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vol, "certs"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{mailcert.KeyPath, mailcert.CertPath, mailcert.PrevKeyPath, mailcert.NewKeyPath, mailcert.JournalPath, "certs/.upload-x"} {
		if err := os.WriteFile(filepath.Join(vol, p), []byte(protectedMarker), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(vol, "mox.conf"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("certs", filepath.Join(vol, "alias")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink("managed.key", filepath.Join(vol, "certs", "link.pem")); err != nil {
		t.Fatal(err)
	}
	node := dirVolumeNode{dir: vol}
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return node, nil }))
	cookie := loginTestSession(t, rt, db)
	svc := store.DesiredService{Name: "mail", Image: "i:1", Port: mailWebPort, TemplateID: mailTemplateID,
		Volumes: []store.ServiceVolume{{Name: "app-mail-mail_config", ContainerPath: "/mox/config"}}}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatal(err)
	}
	const files = "/api/v1/apps/mail/volumes/mail_config/files"

	bypasses := []string{
		mailcert.KeyPath, mailcert.CertPath, mailcert.PrevKeyPath, mailcert.NewKeyPath, mailcert.JournalPath, "certs/.upload-x",
		"certs//managed.key", "./certs/managed.key", "certs/./managed.key", "/certs/managed.key", "CERTS/MANAGED.KEY",
		"certs/../certs/managed.key", "alias/managed.key", "certs/link.pem",
	}
	for _, p := range bypasses {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			rec := do(t, rt, cookie, method, files+"/content?path="+p, "overwritten")
			if rec.Code < 400 || strings.Contains(rec.Body.String(), protectedMarker) {
				t.Errorf("%s %q = %d %q, want a refusal", method, p, rec.Code, rec.Body.String())
			}
		}
	}
	rec := do(t, rt, cookie, http.MethodGet, files+"/content?path="+mailcert.KeyPath, "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "mail cert") {
		t.Fatalf("plain key read = %d %s, want a 403 naming mail cert", rec.Code, rec.Body.String())
	}
	for _, p := range []string{mailcert.KeyPath, mailcert.CertPath, mailcert.PrevKeyPath} {
		b, err := os.ReadFile(filepath.Join(vol, p)) //nolint:gosec // p is one of the fixed managed paths under the test dir
		if err != nil || string(b) != protectedMarker {
			t.Fatalf("%s changed: %q %v", p, b, err)
		}
	}

	// The restore route goes through the same guard after loading its backup.
	req := authedRequest(t, cookie, http.MethodPost, files+"/restore", "")
	req.SetPathValue("name", "mail")
	for _, p := range []string{mailcert.KeyPath, "alias/managed.key"} {
		w := httptest.NewRecorder()
		if rt.guardVolumePath(w, req, node, "app-mail-mail_config", p) || w.Code != http.StatusForbidden {
			t.Errorf("restore guard let %q through: %d", p, w.Code)
		}
	}

	rec = do(t, rt, cookie, http.MethodGet, files+"/content?path=mox.conf", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("ordinary file = %d %q", rec.Code, rec.Body.String())
	}
	rec = do(t, rt, cookie, http.MethodGet, files+"?path=certs", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "managed.key") || strings.Contains(rec.Body.String(), protectedMarker) {
		t.Fatalf("listing must show names only: %d %s", rec.Code, rec.Body.String())
	}
}
