package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/vmruntime"
)

const testToken = "tok-abcdef-123456"

func runningVM(t *testing.T) (*vmEnv, *fakeLima) {
	t.Helper()
	e, f := testVM(t)
	f.status = "Running"
	s := vmruntime.Settings{Version: 1, MemoryMiB: 3072, CPUs: 2, DiskGiB: 30, DashboardPort: 18080, IngressPort: 18880, UpgradeHealth: "60ms"}
	if err := vmruntime.Save(e.settingsPath, s); err != nil {
		t.Fatal(err)
	}
	return e, f
}

func readArchive(t *testing.T, r io.Reader) map[string]string {
	t.Helper()
	out := map[string]string{}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		out[h.Name] = string(b)
	}
}

func TestVMExportSuccessAndRefuseOverwrite(t *testing.T) {
	e, f := runningVM(t)
	f.token = testToken
	dest := filepath.Join(t.TempDir(), "backup.tar")
	code, out, errs := runVMCmd(e, nil, "export", dest)
	if code != 0 {
		t.Fatalf("export: %d %s", code, errs)
	}
	raw, _ := os.ReadFile(dest) //nolint:gosec // test temp path
	sum := sha256.Sum256(raw)
	if !strings.Contains(out, hex.EncodeToString(sum[:])) || !strings.Contains(out, "NOT encrypted") {
		t.Fatalf("output lacks sha256 or the warning: %s", out)
	}
	if fi, _ := os.Stat(dest); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	files := readArchive(t, bytes.NewReader(raw))
	if files[vmExportDataName] != "TARDATA" || !strings.Contains(files[vmExportSettings], "dashboard_port") {
		t.Fatalf("archive: %v", files)
	}
	if !f.did("shell levelrail -- sudo systemctl stop") || !f.did("shell levelrail -- sudo systemctl start") {
		t.Fatalf("control plane was not stopped and restarted: %v", f.calls)
	}
	if strings.Contains(out+errs, testToken) {
		t.Fatal("token leaked")
	}
	if code, _, errs = runVMCmd(e, nil, "export", dest); code == 0 || !strings.Contains(errs, "already exists") {
		t.Fatalf("overwrite not refused: %d %s", code, errs)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(dest), ".vm-export-*")); len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}
}

func TestVMExportEncrypted(t *testing.T) {
	e, _ := runningVM(t)
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "backup.tar.age")
	if code, out, errs := runVMCmd(e, nil, "export", "--encrypt", id.Recipient().String(), dest); code != 0 {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	fh, _ := os.Open(dest) //nolint:gosec // test temp path
	defer func() { _ = fh.Close() }()
	r, err := age.Decrypt(fh, id)
	if err != nil {
		t.Fatal(err)
	}
	if readArchive(t, r)[vmExportDataName] != "TARDATA" {
		t.Fatal("decrypted archive lost the data")
	}
}

func TestVMExportNeedsRunningVMAndKeepsNoFile(t *testing.T) {
	e, f := runningVM(t)
	f.status = "Stopped"
	dest := filepath.Join(t.TempDir(), "b.tar")
	if code, _, _ := runVMCmd(e, nil, "export", dest); code == 0 {
		t.Fatal("export of a stopped VM should fail")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("no file should be created")
	}
}

func seedHostState(t *testing.T, e *vmEnv) string {
	t.Helper()
	for _, p := range []string{e.adminPath(), filepath.Join(e.dataDir, "levelrail.yaml"), filepath.Join(e.instanceDir(), "lima.yaml")} {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	keep := filepath.Join(e.dataDir, "export.tar")
	if err := os.WriteFile(keep, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	return keep
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestVMResetRequiresYesAndKeepsExports(t *testing.T) {
	e, f := runningVM(t)
	keep := seedHostState(t, e)
	code, out, _ := runVMCmd(e, nil, "reset")
	if code != exitUsage || !strings.Contains(out, "delete: "+e.settingsPath) || f.did("delete") {
		t.Fatalf("reset without --yes: %d %s %v", code, out, f.calls)
	}
	if code, _, errs := runVMCmd(e, nil, "reset", "--yes", "--keep-credentials"); code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if !f.did("delete --force levelrail") || exists(e.settingsPath) || exists(e.instanceDir()) {
		t.Fatalf("not deleted: %v", f.calls)
	}
	if !exists(e.adminPath()) || !exists(keep) {
		t.Fatal("credentials or export was deleted")
	}
	if code, _, _ := runVMCmd(e, nil, "reset", "--yes"); code != 0 || exists(e.adminPath()) || !exists(keep) {
		t.Fatal("plain reset should drop credentials and keep exports")
	}
}

func TestVMUninstall(t *testing.T) {
	e, f := runningVM(t)
	keep := seedHostState(t, e)
	if code, _, _ := runVMCmd(e, nil, "uninstall"); code != exitUsage || !exists(e.settingsPath) {
		t.Fatal("uninstall must need --yes")
	}
	code, out, errs := runVMCmd(e, nil, "uninstall", "--yes")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if !f.did("stop --force") || !f.did("delete --force") || exists(e.limaDir()) || exists(e.settingsPath) || exists(e.adminPath()) {
		t.Fatalf("not removed: %v", f.calls)
	}
	if !strings.Contains(out, "delete: "+e.limaDir()) || !exists(keep) {
		t.Fatalf("summary or export wrong: %s", out)
	}
}

func fakeBin(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "levelrail-linux")
	if err := os.WriteFile(p, []byte("bin"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVMUpgradeSuccess(t *testing.T) {
	e, f := runningVM(t)
	code, out, errs := runVMCmd(e, nil, "upgrade", "--control-plane-bin", fakeBin(t))
	if code != 0 {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	for _, want := range []string{
		"sudo install -D -m 0644", "systemctl restart lr-isolation.service", "sudo cp -a /var/lib/levelrail/data /var/lib/levelrail/upgrade-backup-",
		"install -m 0755 /tmp/levelrail-cp /usr/local/bin/levelrail", "systemctl restart lr-isolation-check.service",
	} {
		found := false
		for _, c := range f.calls {
			found = found || strings.Contains(c, want)
		}
		if !found {
			t.Errorf("missing call %q in %v", want, f.calls)
		}
	}
	s, _ := vmruntime.Load(e.settingsPath)
	if s.UpgradeBackup == nil || s.UpgradeBackup.Result != "ok" || !strings.Contains(s.UpgradeBackup.Path, "upgrade-backup-") {
		t.Fatalf("backup not recorded: %+v", s.UpgradeBackup)
	}
	_, js, _ := runVMCmd(e, nil, "status", "--json")
	if !strings.Contains(js, `"upgrade_backup"`) || !strings.Contains(js, `"lima_home_short": false`) {
		t.Fatalf("status json: %s", js)
	}
}

func TestVMUpgradeRollsBackOnFailedHealth(t *testing.T) {
	e, f := runningVM(t)
	restored := false
	f.hook = func(line string) {
		if strings.Contains(line, "/levelrail.bin /usr/local/bin/levelrail") {
			restored = true
		}
	}
	e.probe = func(context.Context, int) error {
		if restored {
			return nil
		}
		return errors.New("down")
	}
	code, _, errs := runVMCmd(e, nil, "upgrade", "--control-plane-bin", fakeBin(t))
	if code == 0 || !strings.Contains(errs, "rolled back") {
		t.Fatalf("want rollback failure, got %d %s", code, errs)
	}
	if !restored || !f.did("shell levelrail -- sudo cp -a /var/lib/levelrail/upgrade-backup-") {
		t.Fatalf("binary or data not restored: %v", f.calls)
	}
	s, _ := vmruntime.Load(e.settingsPath)
	if s.UpgradeBackup == nil || s.UpgradeBackup.Result != "rolled_back" {
		t.Fatalf("%+v", s.UpgradeBackup)
	}
}

func TestVMUpgradeRollsBackOnFailedIsolation(t *testing.T) {
	e, f := runningVM(t)
	f.isoResult = "fail"
	code, _, errs := runVMCmd(e, nil, "upgrade", "--control-plane-bin", fakeBin(t))
	if code == 0 || !strings.Contains(errs, "isolation") || !strings.Contains(errs, "rolled back") {
		t.Fatalf("%d %s", code, errs)
	}
	if !f.did("stop --force") {
		t.Fatalf("an unsafe VM must be stopped: %v", f.calls)
	}
}

func TestVMUpgradeNeedsBinary(t *testing.T) {
	e, _ := runningVM(t)
	if code, _, errs := runVMCmd(e, nil, "upgrade"); code == 0 || !strings.Contains(errs, "control-plane-bin") {
		t.Fatalf("%d %s", code, errs)
	}
}

func TestUpgradeTimeoutStoredWins(t *testing.T) {
	env := vmEnvMap(map[string]string{vmruntime.EnvUpgradeHealth: "9m"})
	if d, _ := (vmruntime.Settings{}).UpgradeTimeout(env); d != 9*time.Minute {
		t.Fatalf("env: %v", d)
	}
	if d, _ := (vmruntime.Settings{UpgradeHealth: "2m"}).UpgradeTimeout(env); d != 2*time.Minute {
		t.Fatalf("stored: %v", d)
	}
	if d, _ := (vmruntime.Settings{}).UpgradeTimeout(vmEnvMap(nil)); d != 5*time.Minute {
		t.Fatalf("default: %v", d)
	}
}

func TestChooseLimaHome(t *testing.T) {
	short := "/Users/ann/Library/Application Support/Levelrail/vm/lima"
	h, isShort, err := chooseLimaHome(short, "/Users/ann", vmInstance, 104)
	if err != nil || isShort || h != short {
		t.Fatalf("%q %v %v", h, isShort, err)
	}
	long := "/Users/" + strings.Repeat("a", 30) + "/Library/Application Support/Levelrail/vm/lima"
	h, isShort, err = chooseLimaHome(long, "/Users/"+strings.Repeat("a", 30), vmInstance, 104)
	if err != nil || !isShort || h != "/Users/"+strings.Repeat("a", 30)+"/.lr-lima" {
		t.Fatalf("%q %v %v", h, isShort, err)
	}
	if _, _, err = chooseLimaHome(long, "/Users/"+strings.Repeat("b", 90), vmInstance, 104); err == nil {
		t.Fatal("a home too long for any layout must error")
	}
	if _, _, err = chooseLimaHome(short, "/Users/ann", strings.Repeat("i", 90), 104); err == nil {
		t.Fatal("a long instance name must error")
	}
	if _, isShort, _ = chooseLimaHome(short, "/Users/ann", vmInstance, 60); !isShort {
		t.Fatal("the limit must come from the argument")
	}
}

func TestEnsureLimaHome(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, ".lr-lima")
	if err := ensureLimaHome(home); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(home); fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	if err := ensureLimaHome(home); err != nil {
		t.Fatalf("second call: %v", err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(home, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := ensureLimaHome(link); err == nil {
		t.Fatal("a symlink home must be refused")
	}
}

func TestVMTokenCheckAndExchange(t *testing.T) {
	var gotHost, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost, gotAuth = r.Host, r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/api/desktop/exchange" {
			http.NotFound(w, r)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "session_token", Value: "sess-value", Path: "/", Expires: time.Now().Add(time.Hour), HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		_, _ = w.Write([]byte(`{"expires_at":"2026-11-01T12:00:00Z"}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())

	e, f := runningVM(t)
	s, _ := vmruntime.Load(e.settingsPath)
	s.DashboardPort = port
	if err := vmruntime.Save(e.settingsPath, s); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runVMCmd(e, nil, "token", "--check"); code == 0 {
		t.Fatal("check must fail without a token file")
	}
	if code, _, _ := runVMCmd(e, nil, "token"); code == 0 {
		t.Fatal("token without a mode must fail")
	}
	f.token = testToken
	if code, out, _ := runVMCmd(e, nil, "token", "--check"); code != 0 || out != "" {
		t.Fatalf("check: %d %q", code, out)
	}
	jar := filepath.Join(t.TempDir(), "jar.txt")
	code, out, errs := runVMCmd(e, nil, "token", "--exchange", "--cookie-jar", jar)
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if gotAuth != "Bearer "+testToken || gotHost != "levelrail.localhost:"+strconv.Itoa(port) {
		t.Fatalf("request: %q %q", gotAuth, gotHost)
	}
	if !strings.Contains(out, "2026-11-01T12:00:00Z") || strings.Contains(out+errs, testToken) || strings.Contains(out, "sess-value") {
		t.Fatalf("output: %s", out)
	}
	raw, _ := os.ReadFile(jar) //nolint:gosec // test temp path
	fi, _ := os.Stat(jar)
	if fi.Mode().Perm() != 0o600 || !strings.Contains(string(raw), "session_token\tsess-value") {
		t.Fatalf("jar: %v %s", fi.Mode().Perm(), raw)
	}
}

func TestReadTokenNeverEchoesErrors(t *testing.T) {
	f := &fakeLima{}
	if _, err := vmruntime.ReadToken(context.Background(), f); err == nil {
		t.Fatal("want error")
	}
	f.token = " " + testToken + " "
	if tok, err := vmruntime.ReadToken(context.Background(), f); err != nil || tok != testToken {
		t.Fatalf("%q %v", tok, err)
	}
}
