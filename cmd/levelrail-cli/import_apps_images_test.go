package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestImportAppsTransferImages(t *testing.T) {
	var got apiclient.AppImportImagesTransfer
	var calls []string
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		v := apiclient.AppImportImages{Running: true, Supported: true, Source: "root@old:22",
			Images: []apiclient.AppImportImage{{App: "web", Image: "webimg:1", State: "running", Bytes: 2048}}}
		switch {
		case strings.HasSuffix(r.URL.Path, "/images/transfer"):
			_ = json.NewDecoder(r.Body).Decode(&got)
		case strings.HasSuffix(r.URL.Path, "/images/status"):
			polls++
			v.Running = false
			v.Images[0].State, v.Images[0].Verified = "verified", true
		}
		_ = json.NewEncoder(w).Encode(v)
	}))
	t.Cleanup(srv.Close)
	keyFile := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyFile, []byte("KEY-MATERIAL"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "apps", "--session", "appimp-1", "--transfer-images", "--ssh", "root@old", "--ssh-port", "2222",
		"--ssh-key", keyFile, "--only", "web", "--api-url", srv.URL}, &out, &errb,
		importEnv(map[string]string{"APP_API_TOKEN": "cp", envImportSSHPassphrase: "pp"}))
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %s", code, errb.String())
	}
	if got.SSH != "root@old" || got.Port != 2222 || got.PrivateKey != "KEY-MATERIAL" || got.Passphrase != "pp" || len(got.Items) != 1 {
		t.Fatalf("body = %+v", got)
	}
	if polls != 1 || !strings.Contains(out.String(), "verified") || !strings.Contains(out.String(), "--verify") {
		t.Fatalf("polls = %d, out = %s", polls, out.String())
	}
	if strings.Contains(out.String()+errb.String(), "KEY-MATERIAL") {
		t.Fatal("the key was printed")
	}
}

func TestImportAppsImagesNeedSession(t *testing.T) {
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "apps", "--transfer-images", "--ssh", "root@old"}, &out, &errb, importEnv(map[string]string{}))
	if code != exitUsage || !strings.Contains(errb.String(), "--session") {
		t.Fatalf("code = %d, stderr = %s", code, errb.String())
	}
}

func TestImportAppsTransferFailureExitCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.AppImportImages{Supported: true,
			Images: []apiclient.AppImportImage{{App: "web", Image: "webimg:1", State: "failed", Error: "ssh login failed"}}})
	}))
	t.Cleanup(srv.Close)
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "apps", "--session", "s", "--transfer-images", "--ssh", "root@old", "--ssh-agent", "--api-url", srv.URL},
		&out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"}))
	if code != exitCheckFailed || !strings.Contains(out.String(), "ssh login failed") {
		t.Fatalf("code = %d, out = %s", code, out.String())
	}
}
