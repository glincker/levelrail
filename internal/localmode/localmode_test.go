package localmode

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateListenAddr(t *testing.T) {
	tests := []struct {
		addr    string
		wantErr bool
	}{
		{"127.0.0.1:8080", false},
		{"localhost:8080", false},
		{"[::1]:8080", false},
		{":8080", true},
		{"0.0.0.0:8080", true},
		{"192.168.1.5:80", true},
		{"nonsense", true},
	}
	for _, tc := range tests {
		if err := ValidateListenAddr(tc.addr); (err != nil) != tc.wantErr {
			t.Errorf("ValidateListenAddr(%q) err = %v, wantErr %v", tc.addr, err, tc.wantErr)
		}
	}
}

func TestEnabled(t *testing.T) {
	tests := map[string]bool{"1": true, "true": true, " YES ": true, "0": false, "": false, "no": false}
	for v, want := range tests {
		if got := Enabled(func(string) string { return v }); got != want {
			t.Errorf("Enabled(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestAppURL(t *testing.T) {
	tests := []struct {
		host string
		port int
		want string
	}{
		{"web.localhost", 80, "http://web.localhost"},
		{"web.localhost", 8081, "http://web.localhost:8081"},
	}
	for _, tc := range tests {
		if got := AppURL(tc.host, tc.port); got != tc.want {
			t.Errorf("AppURL(%q, %d) = %q, want %q", tc.host, tc.port, got, tc.want)
		}
	}
}

func TestChoosePorts(t *testing.T) {
	busy := func(ports ...int) PortFree {
		return func(p int) bool {
			for _, b := range ports {
				if p == b {
					return false
				}
			}
			return true
		}
	}
	tests := []struct {
		name      string
		requested int
		free      PortFree
		want      Ports
		wantErr   bool
	}{
		{"all free", 0, busy(), Ports{Dashboard: 8080, HTTP: 80, HTTPS: 443, Agent: 9443}, false},
		{"low ports taken", 0, busy(80, 443), Ports{Dashboard: 8080, HTTP: 8081, HTTPS: 8443, Agent: 9443}, false},
		{"dashboard busy falls forward", 0, busy(8080, 80), Ports{Dashboard: 8081, HTTP: 8082, HTTPS: 443, Agent: 9443}, false},
		{"requested port busy", 9000, busy(9000), Ports{}, true},
		{"requested port free", 9000, busy(), Ports{Dashboard: 9000, HTTP: 80, HTTPS: 443, Agent: 9443}, false},
		{"nothing free", 0, func(int) bool { return false }, Ports{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ChoosePorts(tc.requested, tc.free)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("ports = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestWipe(t *testing.T) {
	t.Run("removes contents of a local instance dir", func(t *testing.T) {
		dir := t.TempDir()
		if _, _, err := LoadOrCreateAdmin(dir, bytes.NewReader(make([]byte, 64))); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "x.db"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Wipe(dir); err != nil {
			t.Fatalf("Wipe() = %v", err)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("dir still has %d entries", len(entries))
		}
	})
	t.Run("refuses a foreign non-empty dir", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "precious"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Wipe(dir); err == nil {
			t.Fatal("Wipe() = nil, want refusal")
		}
		if _, err := os.Stat(filepath.Join(dir, "precious")); err != nil {
			t.Errorf("file was removed: %v", err)
		}
	})
	t.Run("missing and empty dirs are fine", func(t *testing.T) {
		if err := Wipe(filepath.Join(t.TempDir(), "absent")); err != nil {
			t.Errorf("missing dir: %v", err)
		}
		if err := Wipe(t.TempDir()); err != nil {
			t.Errorf("empty dir: %v", err)
		}
	})
}

func TestLoadOrCreateAdmin(t *testing.T) {
	dir := t.TempDir()
	a, created, err := LoadOrCreateAdmin(dir, bytes.NewReader(bytes.Repeat([]byte{7}, 32)))
	if err != nil || !created || a.Username == "" || len(a.Password) < 20 {
		t.Fatalf("first call = %+v, %v, %v", a, created, err)
	}
	info, err := os.Stat(AdminFilePath(dir))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("admin file mode = %v, err %v, want 0600", info.Mode().Perm(), err)
	}
	b, created, err := LoadOrCreateAdmin(dir, nil)
	if err != nil || created || b != a {
		t.Fatalf("second call = %+v, %v, %v; want the stored admin", b, created, err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, ok, err := ReadState(dir); ok || err != nil {
		t.Fatalf("empty ReadState = %v, %v", ok, err)
	}
	if err := WriteState(State{PID: 42, DataDir: dir, HTTPPort: 8081}); err != nil {
		t.Fatal(err)
	}
	st, ok, err := ReadState(dir)
	if err != nil || !ok || st.PID != 42 || st.HTTPPort != 8081 {
		t.Fatalf("ReadState = %+v, %v, %v", st, ok, err)
	}
	ClearState(dir)
	if _, ok, _ := ReadState(dir); ok {
		t.Error("state survived ClearState")
	}
}

func TestResetMarker(t *testing.T) {
	dir := t.TempDir()
	if ResetRequested(dir) {
		t.Fatal("marker present before RequestReset")
	}
	if err := RequestReset(dir); err != nil {
		t.Fatal(err)
	}
	if !ResetRequested(dir) {
		t.Fatal("marker missing after RequestReset")
	}
}

func TestDetectDocker(t *testing.T) {
	home := "/home/dev"
	sockets := func(live ...string) DockerEnv {
		set := map[string]bool{}
		for _, s := range live {
			set[s] = true
		}
		return DockerEnv{
			Getenv: func(k string) string {
				if k == "HOME" {
					return home
				}
				return ""
			},
			Exists: func(p string) bool { return set[p] },
			Dial: func(p string) error {
				if set[p] {
					return nil
				}
				return errors.New("refused")
			},
			GOOS: "darwin",
		}
	}
	t.Run("docker desktop user socket wins over the system default", func(t *testing.T) {
		got, err := DetectDocker(sockets(home+"/.docker/run/docker.sock", "/var/run/docker.sock"))
		if err != nil || got != home+"/.docker/run/docker.sock" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("falls back to the system socket", func(t *testing.T) {
		got, err := DetectDocker(sockets("/var/run/docker.sock"))
		if err != nil || got != "/var/run/docker.sock" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("explicit DOCKER_HOST first", func(t *testing.T) {
		env := sockets("/custom.sock", "/var/run/docker.sock")
		env.Getenv = func(k string) string {
			if k == "DOCKER_HOST" {
				return "unix:///custom.sock"
			}
			return ""
		}
		if got, err := DetectDocker(env); err != nil || got != "/custom.sock" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("stale socket file is an error with a fix", func(t *testing.T) {
		env := sockets()
		env.Exists = func(p string) bool { return p == "/var/run/docker.sock" }
		if _, err := DetectDocker(env); !errors.Is(err, ErrDockerUnreachable) {
			t.Fatalf("err = %v, want ErrDockerUnreachable", err)
		}
	})
	t.Run("nothing found", func(t *testing.T) {
		if _, err := DetectDocker(sockets()); !errors.Is(err, ErrDockerUnreachable) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestDockerFixMentionsPlatform(t *testing.T) {
	for goos, want := range map[string]string{"darwin": "Docker Desktop", "linux": "systemctl"} {
		if got := DockerFix(goos, nil); !bytes.Contains([]byte(got), []byte(want)) {
			t.Errorf("DockerFix(%q) = %q, want mention of %q", goos, got, want)
		}
	}
}
