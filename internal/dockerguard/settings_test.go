package dockerguard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func envMap(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		settings   Settings
		wantMode   Mode
		wantSource string
		wantErr    bool
	}{
		{name: "default is audit", wantMode: ModeAudit, wantSource: SourceDefault},
		{name: "settings used", settings: Settings{Mode: ModeEnforce}, wantMode: ModeEnforce, wantSource: SourceSettings},
		{name: "env wins over settings", env: map[string]string{EnvMode: "off"}, settings: Settings{Mode: ModeEnforce}, wantMode: ModeOff, wantSource: SourceEnv},
		{name: "env case insensitive", env: map[string]string{EnvMode: " ENFORCE "}, wantMode: ModeEnforce, wantSource: SourceEnv},
		{name: "blank env ignored", env: map[string]string{EnvMode: "  "}, settings: Settings{Mode: ModeOff}, wantMode: ModeOff, wantSource: SourceSettings},
		{name: "typo fails closed", env: map[string]string{EnvMode: "enforcing"}, wantMode: ModeEnforce, wantSource: SourceEnv, wantErr: true},
		{name: "bad settings fail closed", settings: Settings{Mode: "maybe"}, wantMode: ModeEnforce, wantSource: SourceSettings, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, source, err := Resolve(envMap(tt.env), tt.settings)
			if mode != tt.wantMode || source != tt.wantSource || (err != nil) != tt.wantErr {
				t.Fatalf("Resolve = %s, %s, %v; want %s, %s, err %v", mode, source, err, tt.wantMode, tt.wantSource, tt.wantErr)
			}
		})
	}
}

func TestTunablesFromEnv(t *testing.T) {
	tun, err := TunablesFromEnv(envMap(map[string]string{EnvMaxBodyBytes: "2048", EnvAuditDedup: "5m", EnvAllowHostNetwork: "true", EnvSummaryWindow: "nope"}))
	if err == nil {
		t.Fatal("invalid summary window should be reported")
	}
	if tun.MaxBodyBytes != 2048 || tun.AuditDedup != 5*time.Minute || !tun.AllowHostNetwork || tun.SummaryWindow != DefaultSummaryWindow {
		t.Fatalf("tunables = %+v", tun)
	}
}

func TestControllerSetModePersistsAndAppliesLive(t *testing.T) {
	h := startGuard(t, ModeAudit)
	path := filepath.Join(t.TempDir(), SettingsFile)
	c := NewController(path, envMap(nil), h.server, "unix:///var/run/docker.sock", nil)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	st, err := c.SetMode(ModeEnforce, "admin", now)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != ModeEnforce || st.Effective != ModeEnforce || st.Source != SourceSettings || st.RestartRequired {
		t.Fatalf("status = %+v", st)
	}
	saved, err := LoadSettings(path)
	if err != nil || saved.Mode != ModeEnforce || saved.UpdatedBy != "admin" {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("settings file mode: %v %v", info, err)
	}
	st, err = c.SetMode(ModeOff, "admin", now)
	if err != nil || !st.RestartRequired || st.Effective != ModeAudit {
		t.Fatalf("off while running: %+v, %v", st, err)
	}
	if _, err := c.SetMode("sometimes", "admin", now); err == nil {
		t.Fatal("invalid mode accepted")
	}
}

func TestControllerPinnedByEnv(t *testing.T) {
	c := NewController(filepath.Join(t.TempDir(), SettingsFile), envMap(map[string]string{EnvMode: "audit"}), nil, "", nil)
	if _, err := c.SetMode(ModeEnforce, "admin", time.Now()); !errors.Is(err, ErrPinnedByEnv) {
		t.Fatalf("err = %v, want ErrPinnedByEnv", err)
	}
}

func TestBootModes(t *testing.T) {
	d := newFakeDaemon(t)
	tests := []struct {
		name        string
		env         map[string]string
		wantErr     bool
		wantRunning bool
	}{
		{name: "off starts nothing", env: map[string]string{EnvMode: "off"}},
		{name: "audit starts the guard", env: map[string]string{}, wantRunning: true},
		{name: "enforce starts the guard", env: map[string]string{EnvMode: "enforce"}, wantRunning: true},
		{name: "enforce with tcp upstream refuses to boot", env: map[string]string{EnvMode: "enforce", "DOCKER_HOST": "tcp://10.0.0.1:2375"}, wantErr: true},
		{name: "audit with tcp upstream falls back", env: map[string]string{"DOCKER_HOST": "tcp://10.0.0.1:2375"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{"DOCKER_HOST": "unix://" + d.socket}
			for k, v := range tt.env {
				env[k] = v
			}
			dir := shortTempDir(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b, err := Boot(ctx, BootConfig{DataDir: dir, Lookup: envMap(env)})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Boot err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if (b.Server != nil) != tt.wantRunning || (b.Host != "") != tt.wantRunning {
				t.Fatalf("running = %v host = %q", b.Server != nil, b.Host)
			}
			if b.Server != nil {
				_ = b.Server.Close()
			}
		})
	}
}
