package buildernode

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

type fakeLima struct {
	state string
	calls []string
	fail  map[string]bool
	load  string
}

func (f *fakeLima) Run(_ context.Context, args ...string) ([]byte, error) {
	line := strings.Join(args, " ")
	f.calls = append(f.calls, line)
	for k := range f.fail {
		if strings.Contains(line, k) {
			return nil, errors.New("boom")
		}
	}
	switch {
	case args[0] == "list":
		if f.state == "" {
			return nil, nil
		}
		return []byte(Instance + " " + f.state + "\n"), nil
	case args[0] == "start":
		f.state = "Running"
	case args[0] == "stop":
		f.state = "Stopped"
	case strings.HasSuffix(line, "/proc/loadavg"):
		return []byte(f.load + " 0 0 1/1 1\n"), nil
	}
	return nil, nil
}

func (f *fakeLima) did(sub string) bool {
	for _, c := range f.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func noEnv(string) (string, bool) { return "", false }

func TestJoinWritesPrivateConfigAndToken(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	c, err := Join(dir, store, JoinInput{Server: "https://lr.example.com/", Token: " tok ", NodeName: "mac1", Limits: Limits{MaxCPU: 4, MaxMemMiB: 4096, PauseOnBattery: true, IdleStop: 15 * time.Minute}}, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if c.AgentAddr != "lr.example.com:8081" || c.Server != "https://lr.example.com" {
		t.Fatalf("config = %+v", c)
	}
	for _, p := range []string{ConfigPath(dir), store.Path} {
		st, err := os.Stat(p)
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", p, st, err)
		}
	}
	raw, _ := os.ReadFile(ConfigPath(dir))
	if strings.Contains(string(raw), "tok") {
		t.Fatal("config must not hold the token")
	}
	if tok, _ := store.Load(); tok != "tok" {
		t.Fatalf("token = %q", tok)
	}
	got, err := LoadConfig(dir)
	if err != nil || got.IdleStop != "15m0s" || !got.PauseOnBattery {
		t.Fatalf("loaded %+v %v", got, err)
	}
	if err := Logout(dir, store); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(dir); !errors.Is(err, ErrNotJoined) {
		t.Fatalf("after logout: %v", err)
	}
}

func TestJoinRejectsBadInput(t *testing.T) {
	ok := Limits{MaxCPU: 2, MaxMemMiB: 2048}
	for name, in := range map[string]JoinInput{
		"http remote": {Server: "http://lr.example.com", Token: "t", Limits: ok},
		"no token":    {Server: "https://lr.example.com", Limits: ok},
		"no host":     {Server: "lr", Token: "t", Limits: ok},
		"zero cpu":    {Server: "https://lr.example.com", Token: "t", Limits: Limits{MaxMemMiB: 1}},
	} {
		dir := t.TempDir()
		if _, err := Join(dir, NewFileStore(dir), in, noEnv); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	dir := t.TempDir()
	if _, err := Join(dir, NewFileStore(dir), JoinInput{Server: "http://localhost:8080", Token: "t", NodeName: "n", Limits: ok}, noEnv); err != nil {
		t.Errorf("loopback http: %v", err)
	}
}

func TestLookupLimitsFromEnv(t *testing.T) {
	env := func(k string) (string, bool) {
		return map[string]string{EnvMaxCPU: "6", EnvMaxMemMiB: "8192", EnvIdleStop: "10m", EnvPauseBattery: "on"}[k], true
	}
	l, err := LookupLimits(env)
	if err != nil || l.MaxCPU != 6 || l.MaxMemMiB != 8192 || l.IdleStop != 10*time.Minute || !l.PauseOnBattery {
		t.Fatalf("%+v %v", l, err)
	}
	bad := func(string) (string, bool) { return "x", true }
	if _, err := LookupLimits(bad); err == nil {
		t.Fatal("want an env error")
	}
}

func doctorEnv(f *fakeLima, skew time.Duration) DoctorEnv {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	return DoctorEnv{
		Arch: "arm64", HostMemMiB: 16384, DiskFreeGiB: 100, Runner: f, Env: noEnv, ConfigServer: "https://lr.example.com",
		LimaPath:   func() (string, error) { return "/usr/bin/limactl", nil },
		ServerTime: func(context.Context, string) (time.Time, error) { return now.Add(skew), nil },
		Now:        func() time.Time { return now },
	}
}

func byName(cs []Check) map[string]Check {
	m := map[string]Check{}
	for _, c := range cs {
		m[c.Name] = c
	}
	return m
}

func TestDoctorAllPass(t *testing.T) {
	cs, err := Doctor(context.Background(), doctorEnv(&fakeLima{state: "Running"}, 2*time.Second))
	if err != nil || !Passed(cs) {
		t.Fatalf("%+v %v", cs, err)
	}
}

func TestDoctorFailures(t *testing.T) {
	e := doctorEnv(&fakeLima{}, time.Hour)
	e.Arch, e.HostMemMiB, e.DiskFreeGiB = "riscv64", 1024, 1
	e.LimaPath = func() (string, error) { return "", errors.New("missing") }
	cs, _ := Doctor(context.Background(), e)
	m := byName(cs)
	for _, n := range []string{"arch", "ram", "disk", "lima", "vm", "clock"} {
		if m[n].OK {
			t.Errorf("%s should fail: %+v", n, m[n])
		}
	}
	e.ServerTime = func(context.Context, string) (time.Time, error) { return time.Time{}, errors.New("down") }
	if m := byName(mustDoctor(t, e)); m["control-plane"].OK {
		t.Error("unreachable control plane must fail")
	}
	e.ConfigServer = ""
	if m := byName(mustDoctor(t, e)); m["control-plane"].OK {
		t.Error("not joined must fail")
	}
}

func TestDoctorThresholdsFromEnv(t *testing.T) {
	e := doctorEnv(&fakeLima{}, 0)
	e.HostMemMiB = 4096
	e.Env = func(k string) (string, bool) {
		if k == EnvMinRAMMiB {
			return "2048", true
		}
		return "", false
	}
	if !byName(mustDoctor(t, e))["ram"].OK {
		t.Error("env threshold should lower the RAM bar")
	}
	e.Env = func(string) (string, bool) { return "x", true }
	if _, err := Doctor(context.Background(), e); err == nil {
		t.Error("bad env must error")
	}
}

func mustDoctor(t *testing.T, e DoctorEnv) []Check {
	t.Helper()
	cs, err := Doctor(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func testHost(t *testing.T, f *fakeLima) (*Host, string, FileStore) {
	t.Helper()
	dir := t.TempDir()
	store := NewFileStore(dir)
	if err := store.Save("secret-token"); err != nil {
		t.Fatal(err)
	}
	return &Host{Runner: f, Store: store, InstanceDir: dir + "/lima/" + Instance,
		Render: func(int, int) ([]byte, error) { return []byte("yaml"), nil },
		Sleep:  func(time.Duration) {}, Env: noEnv, Out: io.Discard}, dir, store
}

func TestStartEnrollsOnceAndDropsToken(t *testing.T) {
	f := &fakeLima{fail: map[string]bool{"test -f " + guestIdentity: true}}
	h, _, store := testHost(t, f)
	c := Config{AgentAddr: "lr.example.com:8081", NodeName: "mac1", MaxCPU: 2, MaxMemMiB: 2048}
	if err := h.Start(context.Background(), c, "/tmp/agent"); err != nil {
		t.Fatal(err)
	}
	if !f.did("start --tty=false "+Instance) || !f.did("systemctl restart "+guestUnit) || !f.did("copy /tmp/agent") {
		t.Fatalf("calls: %v", f.calls)
	}
	if tok, _ := store.Load(); tok != "" {
		t.Fatal("the spent token must be removed from the host")
	}
}

func TestStartKeepsTokenWhenAlreadyEnrolled(t *testing.T) {
	f := &fakeLima{state: "Stopped"}
	h, _, store := testHost(t, f)
	if err := h.Start(context.Background(), Config{MaxCPU: 1, MaxMemMiB: 1024}, ""); err != nil {
		t.Fatal(err)
	}
	if tok, _ := store.Load(); tok == "" {
		t.Fatal("token must stay when the guest already has an identity")
	}
}

func TestStartFailsClosed(t *testing.T) {
	f := &fakeLima{fail: map[string]bool{"start --tty": true}}
	h, _, _ := testHost(t, f)
	if err := h.Start(context.Background(), Config{MaxCPU: 1, MaxMemMiB: 1024}, ""); err == nil || !f.did("stop "+Instance) {
		t.Fatalf("err=%v calls=%v", err, f.calls)
	}
}

func TestStopAndStatus(t *testing.T) {
	f := &fakeLima{state: "Running"}
	h, dir, _ := testHost(t, f)
	if _, err := Join(dir, h.Store, JoinInput{Server: "https://lr.example.com", Token: "t", NodeName: "n", Limits: Limits{MaxCPU: 2, MaxMemMiB: 2048}}, noEnv); err != nil {
		t.Fatal(err)
	}
	s, err := h.Status(context.Background(), dir)
	if err != nil || !s.Joined || s.VM != "Running" {
		t.Fatalf("%+v %v", s, err)
	}
	if err := h.Stop(context.Background()); err != nil || f.state != "Stopped" {
		t.Fatalf("stop: %v %s", err, f.state)
	}
}

func TestSuperviseIdleStop(t *testing.T) {
	f := &fakeLima{state: "Running", load: "0.01"}
	h, _, _ := testHost(t, f)
	if err := h.Supervise(context.Background(), Config{IdleStop: "1m"}); err != nil || f.state != "Stopped" {
		t.Fatalf("err=%v state=%s", err, f.state)
	}
}

func TestSuperviseBusyDoesNotStopAndBatteryPauses(t *testing.T) {
	f := &fakeLima{state: "Running", load: "3.5"}
	h, _, _ := testHost(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	polls := 0
	h.Sleep = func(time.Duration) {
		if polls++; polls == 3 {
			cancel()
		}
	}
	h.OnBattery = func(context.Context) (bool, error) { return true, nil }
	if err := h.Supervise(ctx, Config{IdleStop: "1m", PauseOnBattery: true}); err != nil {
		t.Fatal(err)
	}
	if f.state != "Running" || !f.did("systemctl stop "+guestUnit) {
		t.Fatalf("state=%s calls=%v", f.state, f.calls)
	}
}
