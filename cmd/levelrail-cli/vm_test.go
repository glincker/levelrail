package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/GLINCKER/levelrail/internal/vmruntime/guest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/vmruntime"
)

const fakeBoot = "boot-123"

type fakeLima struct {
	calls     []string
	status    string // limactl list status, empty means no instance
	isoResult string
	bootID    string
	failStart bool
	hook      func(line string)
	token     string
}

func (f *fakeLima) Run(_ context.Context, args ...string) ([]byte, error) {
	line := strings.Join(args, " ")
	f.calls = append(f.calls, line)
	if f.hook != nil {
		f.hook(line)
	}
	switch {
	case strings.HasSuffix(line, "desktop-token"):
		if f.token == "" {
			return nil, errors.New("missing")
		}
		if strings.Contains(line, "test -f") {
			return nil, nil
		}
		return []byte(f.token + "\n"), nil
	case strings.Contains(line, "tar -C"):
		return []byte("TARDATA"), nil
	case args[0] == "list":
		if f.status == "" {
			return nil, nil
		}
		return []byte(`{"name":"levelrail","status":"` + f.status + `"}` + "\n"), nil
	case strings.Contains(line, "test -f"):
		return nil, errors.New("missing")
	case args[0] == "create":
		f.status = "Stopped"
	case args[0] == "start":
		if f.failStart {
			return nil, errors.New("boom")
		}
		f.status = "Running"
	case args[0] == "stop":
		f.status = "Stopped"
	case strings.HasSuffix(line, "/proc/sys/kernel/random/boot_id"):
		return []byte(fakeBoot + "\n"), nil
	case strings.Contains(line, "isolation-status.json"):
		res := f.isoResult
		if res == "" {
			res = "pass"
		}
		boot := f.bootID
		if boot == "" {
			boot = fakeBoot
		}
		return []byte(`{"result":"` + res + `","checked_at":"2026-11-01T10:00:00Z","boot_id":"` + boot + `","checks":[{"name":"egress","result":"` + res + `","detail":""}]}`), nil
	case strings.Contains(line, "df -h"):
		return []byte("Used Size Use%\n1G 30G 4%\n"), nil
	}
	return nil, nil
}

func (f *fakeLima) Interactive(_ context.Context, args ...string) error {
	f.calls = append(f.calls, "interactive "+strings.Join(args, " "))
	return nil
}

func (f *fakeLima) did(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func testVM(t *testing.T) (*vmEnv, *fakeLima) {
	t.Helper()
	dir := t.TempDir()
	f := &fakeLima{}
	return &vmEnv{
		runner:       f,
		settingsPath: filepath.Join(dir, "vm", "settings.json"),
		dataDir:      filepath.Join(dir, "vm"),
		homeDir:      filepath.Join(dir, "home"),
		hostMemMiB:   16384,
		hostCPUs:     8,
		arch:         "aarch64",
		portFree:     func(int) bool { return true },
		probe:        func(context.Context, int) error { return nil },
		sleep:        func(time.Duration) {},
	}, f
}

func vmEnvMap(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func runVMCmd(e *vmEnv, env map[string]string, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := vmDispatch(context.Background(), e, "cli", args, &out, &errb, vmEnvMap(env))
	return code, out.String(), errb.String()
}

func TestVMExperimentalGate(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runVM("cli", []string{"status"}, &out, &errb, vmEnvMap(nil)); code != exitUsage || !strings.Contains(errb.String(), "APP_VM_EXPERIMENTAL=on") {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
}

func TestVMPlatformStubError(t *testing.T) {
	if !strings.Contains(errVMUnsupported.Error(), "not supported on this platform yet") {
		t.Fatal(errVMUnsupported)
	}
	if runtime.GOOS != "darwin" {
		if _, err := newVMEnv(vmEnvMap(nil)); !errors.Is(err, errVMUnsupported) {
			t.Fatalf("err = %v", err)
		}
	}
}

func TestVMInitSeedsFromEnvAndStarts(t *testing.T) {
	e, f := testVM(t)
	bin := filepath.Join(t.TempDir(), "levelrail-linux")
	_ = os.WriteFile(bin, []byte("x"), 0o600)
	code, out, errs := runVMCmd(e, map[string]string{"APP_VM_MEMORY_MIB": "4096", "APP_VM_CPUS": "3"}, "init", "--control-plane-bin", bin)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errs)
	}
	s, err := vmruntime.Load(e.settingsPath)
	if err != nil || s.MemoryMiB != 4096 || s.CPUs != 3 || s.DiskGiB != 30 || s.DashboardPort == 0 || s.IngressPort == s.DashboardPort {
		t.Fatalf("settings %+v err %v", s, err)
	}
	if s.Applied == nil {
		t.Fatal("applied not recorded")
	}
	if !f.did("create --tty=false --name=levelrail") || !f.did("start") || !f.did("copy "+bin) {
		t.Fatalf("calls %v", f.calls)
	}
	if !strings.Contains(out, "http://levelrail.localhost:") || !strings.Contains(out, "desktop-token") {
		t.Fatalf("out %q", out)
	}
	if st, _ := os.Stat(e.adminPath()); st == nil || st.Mode().Perm() != 0o600 {
		t.Fatal("admin credentials missing or not 0600")
	}
	if !f.did("copy "+e.adminPath()) || strings.Contains(out, "APP_ADMIN_PASSWORD") {
		t.Fatalf("admin env not installed in the guest, or leaked: %v", f.calls)
	}
	yaml, _ := os.ReadFile(filepath.Join(e.dataDir, "levelrail.yaml"))
	if !strings.Contains(string(yaml), "APP_DESKTOP_ALLOWED_PORTS") {
		t.Fatal("sidecar unit lacks allowed ports")
	}
	for _, want := range []string{"APP_INGRESS_HTTPS_ADDR=127.0.0.1:", "APP_AGENT_ADDR=127.0.0.1:", "APP_NETWORK_POOL=" + guest.DefaultAppNetworkPool, "EnvironmentFile=-" + vmGuestEnvFile} {
		if !strings.Contains(string(yaml), want) {
			t.Fatalf("sidecar unit lacks %q", want)
		}
	}
}

func TestVMStoredValueWinsOverEnv(t *testing.T) {
	e, _ := testVM(t)
	_ = vmruntime.Save(e.settingsPath, vmruntime.Settings{MemoryMiB: 3072, CPUs: 2, DiskGiB: 30, DashboardPort: 9000, IngressPort: 9001})
	_, out, _ := runVMCmd(e, map[string]string{"APP_VM_MEMORY_MIB": "5000"}, "status", "--json")
	var r map[string]any
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err, out)
	}
	if r["memory_mib"].(float64) != 3072 || len(r["env_differs"].([]any)) != 1 {
		t.Fatalf("report %v", r)
	}
	if r["dashboard_url"] != "http://levelrail.localhost:9000/" {
		t.Fatalf("url %v", r["dashboard_url"])
	}
}

func TestVMStartFailingIsolationStopsVM(t *testing.T) {
	e, f := testVM(t)
	_ = vmruntime.Save(e.settingsPath, vmruntime.Settings{MemoryMiB: 3072, CPUs: 2, DiskGiB: 30, DashboardPort: 9000, IngressPort: 9001})
	_ = os.MkdirAll(e.instanceDir(), 0o700)
	f.status, f.isoResult = "Stopped", "fail"
	code, _, errs := runVMCmd(e, nil, "start")
	if code == 0 || !f.did("stop --force") || !strings.Contains(errs, "isolation") {
		t.Fatalf("code=%d calls=%v err=%s", code, f.calls, errs)
	}
}

func TestVMStartStaleStatusFailsClosed(t *testing.T) {
	e, f := testVM(t)
	_ = vmruntime.Save(e.settingsPath, vmruntime.Settings{MemoryMiB: 3072, CPUs: 2, DiskGiB: 30, DashboardPort: 9000, IngressPort: 9001})
	_ = os.MkdirAll(e.instanceDir(), 0o700)
	f.status, f.bootID = "Stopped", "old-boot"
	if code, _, _ := runVMCmd(e, map[string]string{"APP_VM_BOOT_TIMEOUT": "50ms"}, "start"); code == 0 || !f.did("stop --force") {
		t.Fatalf("code=%d calls=%v", code, f.calls)
	}
}

func TestVMStopAndShell(t *testing.T) {
	e, f := testVM(t)
	f.status = "Running"
	if code, _, _ := runVMCmd(e, nil, "stop"); code != 0 || !f.did("stop levelrail") {
		t.Fatalf("stop calls %v", f.calls)
	}
	if code, _, _ := runVMCmd(e, nil, "shell"); code == 0 {
		t.Fatal("shell on a stopped VM must fail")
	}
	f.status = "Running"
	if code, _, _ := runVMCmd(e, nil, "shell"); code != 0 || !f.did("interactive shell levelrail") {
		t.Fatalf("shell calls %v", f.calls)
	}
}

func TestVMSetValidation(t *testing.T) {
	e, _ := testVM(t)
	_ = vmruntime.Save(e.settingsPath, vmruntime.Settings{MemoryMiB: 3072, CPUs: 2, DiskGiB: 30, DashboardPort: 9000, IngressPort: 9001})
	for _, c := range []struct {
		k, v string
		ok   bool
	}{{"memory", "1024", false}, {"memory", "9000", false}, {"memory", "4096", true}, {"disk", "20", false}, {"disk", "40", true}, {"cpus", "99", false}, {"bogus", "1", false}} {
		code, _, errs := runVMCmd(e, nil, "set", c.k, c.v)
		if (code == 0) != c.ok {
			t.Errorf("set %s %s: code=%d err=%s", c.k, c.v, code, errs)
		}
	}
	s, _ := vmruntime.Load(e.settingsPath)
	if s.MemoryMiB != 4096 || s.DiskGiB != 40 {
		t.Fatalf("stored %+v", s)
	}
}

func TestVMSettingsFilePermsAndAtomic(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d", "settings.json")
	if err := vmruntime.Save(p, vmruntime.Settings{MemoryMiB: 3072, CPUs: 2, DiskGiB: 30}); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}
	if err := vmruntime.Save(p, vmruntime.Settings{MemoryMiB: 4096, CPUs: 2, DiskGiB: 30}); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(filepath.Dir(p))
	if len(ents) != 1 {
		t.Fatalf("temp files left: %v", ents)
	}
}

func TestVMRefusesLimaConfigDir(t *testing.T) {
	e, _ := testVM(t)
	cfg := filepath.Join(e.dataDir, "lima", "_config")
	_ = os.MkdirAll(cfg, 0o700)
	// Lima's own generated keys are allowed.
	_ = os.WriteFile(filepath.Join(cfg, "user.pub"), []byte("k"), 0o600)
	if err := e.checkLimaHome(); err != nil {
		t.Fatalf("generated keys must not block: %v", err)
	}
	_ = os.WriteFile(filepath.Join(cfg, "override.yaml"), []byte("x"), 0o600)
	if code, _, errs := runVMCmd(e, nil, "init"); code == 0 || !strings.Contains(errs, "_config") {
		t.Fatalf("code=%d err=%s", code, errs)
	}
}
