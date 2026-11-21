package macosrt

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls [][]string
	fail  map[string]bool
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if f.fail[args[0]] {
		return nil, errors.New("fail")
	}
	return nil, nil
}

func spec(t *testing.T) Spec {
	t.Helper()
	return Spec{Role: RoleAgent, Binary: "/usr/local/bin/levelrail-agent", LogDir: t.TempDir(),
		Env: map[string]string{"B": "x&y", "A": "1"}}
}

func TestPlistDeterministicAndEscaped(t *testing.T) {
	s := spec(t)
	a, err := s.Plist()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.Plist()
	if string(a) != string(b) {
		t.Fatal("plist not deterministic")
	}
	out := string(a)
	for _, want := range []string{"com.levelrail.agent", "x&amp;y", "<integer>30</integer>", "KeepAlive"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Index(out, "<key>A</key>") > strings.Index(out, "<key>B</key>") {
		t.Error("env not sorted")
	}
}

func TestValidateRejects(t *testing.T) {
	s := spec(t)
	s.Binary = "relative"
	if _, err := s.Plist(); err == nil {
		t.Error("relative binary accepted")
	}
	s = spec(t)
	s.Env = map[string]string{"APP_JOIN_TOKEN": "x"}
	if _, err := s.Plist(); err == nil {
		t.Error("secret env accepted")
	}
}

func TestInstallUninstall(t *testing.T) {
	r := &fakeRunner{fail: map[string]bool{"bootout": true}}
	m := Manager{Dir: t.TempDir(), UID: 501, Runner: r}
	s := spec(t)
	p, err := m.Install(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	if got := r.calls[len(r.calls)-1]; got[0] != "bootstrap" || got[1] != "gui/501" {
		t.Errorf("bootstrap call %v", got)
	}
	if st := m.Status(context.Background(), RoleAgent); !st.Installed || !st.Loaded {
		t.Errorf("status %+v", st)
	}
	removed, err := m.Uninstall(context.Background(), RoleAgent)
	if err != nil || !removed {
		t.Fatalf("uninstall %v %v", removed, err)
	}
	if removed, _ := m.Uninstall(context.Background(), RoleAgent); removed {
		t.Error("second uninstall should report nothing removed")
	}
}

func deps() Deps {
	return Deps{
		GOOS: "darwin", GOARCH: "arm64", MinFreeGiB: 40, MinMacOSMajor: 13, Ports: []int{8080},
		LookPath:  func(string) (string, error) { return "/opt/homebrew/bin/limactl", nil },
		Sysctl:    func(string) (string, error) { return "1\n", nil },
		Version:   func() (string, error) { return "14.5", nil },
		FreeBytes: func(string) (int64, error) { return 100 << 30, nil },
		PortFree:  func(int) bool { return true },
	}
}

func statuses(rs []Result) map[string]string {
	m := map[string]string{}
	for _, r := range rs {
		m[r.Code] = r.Status
	}
	return m
}

func TestDoctorAllOK(t *testing.T) {
	for c, s := range statuses(Run(deps())) {
		if s != StatusOK {
			t.Errorf("%s = %s", c, s)
		}
	}
}

func TestDoctorFailures(t *testing.T) {
	d := deps()
	d.LookPath = func(string) (string, error) { return "", errors.New("no") }
	d.Sysctl = func(string) (string, error) { return "0", nil }
	d.Version = func() (string, error) { return "12.1", nil }
	d.FreeBytes = func(string) (int64, error) { return 1 << 30, nil }
	d.PortFree = func(int) bool { return false }
	s := statuses(Run(d))
	want := map[string]string{"macos_lima": StatusFail, "macos_vz": StatusFail, "macos_version": StatusFail,
		"macos_disk": StatusWarn, "macos_port_8080": StatusWarn}
	for c, w := range want {
		if s[c] != w {
			t.Errorf("%s = %s want %s", c, s[c], w)
		}
	}
}

func TestDoctorNotDarwin(t *testing.T) {
	d := deps()
	d.GOOS = "linux"
	if rs := Run(d); len(rs) != 1 || rs[0].Status != StatusWarn {
		t.Fatalf("%+v", rs)
	}
}
