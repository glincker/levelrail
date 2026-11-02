package lima

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	testHome = "/Users/test"
	testData = "/Users/test/Library/Application Support/levelrail"
	testSHA  = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func testSettings() Settings {
	s := DefaultSettings()
	s.DashboardPort = 18080
	s.IngressPort = 18088
	return s
}

func testLimits() Limits {
	return DefaultLimits(16384, 8, 0, testHome, testData)
}

func renderMap(t *testing.T, s Settings, ro RenderOptions) (map[string]any, string) {
	t.Helper()
	raw, err := Render(s, testLimits(), ro)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatalf("rendered yaml does not parse: %v", err)
	}
	return m, string(raw)
}

func TestRenderIsolationDefaults(t *testing.T) {
	m, raw := renderMap(t, testSettings(), RenderOptions{})

	if m["plain"] != true {
		t.Error("plain mode must be on without shares")
	}
	if mounts, _ := m["mounts"].([]any); mounts == nil || len(mounts) != 0 {
		t.Errorf("mounts must be an explicit empty list, got %v", m["mounts"])
	}
	if strings.Contains(raw, testHome) || strings.Contains(raw, `location: ~`) {
		t.Error("template must not mention the host home directory")
	}
	if strings.Contains(raw, "docker.sock") || strings.Contains(raw, "guestSocket") {
		t.Error("no docker socket forward allowed")
	}
	if a, _ := m["audio"].(map[string]any); a["device"] != "" {
		t.Errorf("audio device must be empty, got %v", a["device"])
	}
	vz, _ := m["vmOpts"].(map[string]any)["vz"].(map[string]any)
	if r, _ := vz["rosetta"].(map[string]any); r["enabled"] != false {
		t.Error("rosetta must stay off")
	}
	if m["propagateProxyEnv"] != false {
		t.Error("host proxy env must not propagate")
	}
	if m["vmType"] != "vz" {
		t.Errorf("vmType = %v", m["vmType"])
	}

	fw, _ := m["portForwards"].([]any)
	if len(fw) != 2 {
		t.Fatalf("want exactly two forwards, got %d", len(fw))
	}
	wantHost := []int{18080, 18088}
	for i, f := range fw {
		fm := f.(map[string]any)
		if fm["hostIP"] != "127.0.0.1" || fm["static"] != true || fm["hostPort"] != wantHost[i] {
			t.Errorf("forward %d not loopback static: %v", i, fm)
		}
	}
	if wildcard.MatchString(raw) {
		t.Error("no wildcard binds allowed")
	}
}

func TestRenderSharesAreReadOnlyAndKeepAutoForwardOff(t *testing.T) {
	s := testSettings()
	s.Shares = []Share{{HostPath: "/Users/test/Documents/notes"}}
	m, _ := renderMap(t, s, RenderOptions{})

	mounts := m["mounts"].([]any)
	if len(mounts) != 1 || mounts[0].(map[string]any)["writable"] != false {
		t.Fatalf("share must be read-only: %v", mounts)
	}
	if m["plain"] != false {
		t.Error("shares need non-plain mode")
	}
	var forwards, ignores int
	for _, f := range m["portForwards"].([]any) {
		if f.(map[string]any)["ignore"] == true {
			ignores++
		} else {
			forwards++
		}
	}
	if forwards != 2 || ignores != 1 {
		t.Errorf("want 2 forwards plus one catch-all ignore, got %d and %d", forwards, ignores)
	}
}

func TestRenderImagePinAndOverride(t *testing.T) {
	m, _ := renderMap(t, testSettings(), RenderOptions{})
	imgs := m["images"].([]any)
	if len(imgs) != 2 {
		t.Fatalf("images = %d", len(imgs))
	}
	for _, i := range imgs {
		d, _ := i.(map[string]any)["digest"].(string)
		if !strings.HasPrefix(d, "sha256:") || len(d) != len("sha256:")+64 {
			t.Errorf("image digest not pinned: %q", d)
		}
	}
	m, _ = renderMap(t, testSettings(), RenderOptions{Arch: "x86_64", ImageSHA256Override: testSHA})
	imgs = m["images"].([]any)
	if imgs[1].(map[string]any)["digest"] != "sha256:"+testSHA {
		t.Error("override must apply to the chosen arch")
	}
	if imgs[0].(map[string]any)["digest"] == "sha256:"+testSHA {
		t.Error("override must not touch the other arch")
	}
	if _, err := Render(testSettings(), testLimits(), RenderOptions{ImageSHA256Override: "xyz"}); err == nil {
		t.Error("bad override digest must be rejected")
	}
}

func TestProvisionContainsFirewallAndOrder(t *testing.T) {
	_, raw := renderMap(t, testSettings(), RenderOptions{})
	for _, want := range []string{"ip daddr 192.168.5.2 drop", "192.168.0.0/16", "10.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "\"ip\": \"127.0.0.1\""} {
		if !strings.Contains(raw, want) {
			t.Errorf("rendered template lacks %q", want)
		}
	}
	fw := strings.Index(raw, "enable --now lr-isolation.service")
	dk := strings.Index(raw, "install -y --no-install-recommends docker.io")
	if fw < 0 || dk < 0 || fw > dk {
		t.Error("firewall unit must be enabled before Docker is installed")
	}
}

func TestSidecarSlot(t *testing.T) {
	s := testSettings()
	s.Sidecar.BinaryPath = "/usr/local/bin/levelrail"
	s.Sidecar.UnitName = "levelrail.service"
	s.Sidecar.UnitContent = "[Service]\nExecStart=/usr/local/bin/levelrail\n"
	_, raw := renderMap(t, s, RenderOptions{})
	if !strings.Contains(raw, "systemctl enable --now levelrail.service") {
		t.Error("sidecar unit not enabled")
	}
	s.Sidecar.UnitName = "bad name.service"
	if _, err := Render(s, testLimits(), RenderOptions{}); err == nil {
		t.Error("bad unit name must be rejected")
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Settings){
		"memory below floor":     func(s *Settings) { s.MemoryMiB = 1024 },
		"memory above cap":       func(s *Settings) { s.MemoryMiB = 9000 },
		"zero cpus":              func(s *Settings) { s.CPUs = 0 },
		"too many cpus":          func(s *Settings) { s.CPUs = 64 },
		"disk too small":         func(s *Settings) { s.DiskGiB = 2 },
		"disk too large":         func(s *Settings) { s.DiskGiB = 100000 },
		"unset host port":        func(s *Settings) { s.DashboardPort = 0 },
		"privileged host port":   func(s *Settings) { s.IngressPort = 80 },
		"same host ports":        func(s *Settings) { s.IngressPort = s.DashboardPort },
		"writable-looking home":  func(s *Settings) { s.Shares = []Share{{HostPath: testHome}} },
		"parent of home":         func(s *Settings) { s.Shares = []Share{{HostPath: "/Users"}} },
		"root":                   func(s *Settings) { s.Shares = []Share{{HostPath: "/"}} },
		"data dir":               func(s *Settings) { s.Shares = []Share{{HostPath: testData}} },
		"inside data dir":        func(s *Settings) { s.Shares = []Share{{HostPath: testData + "/vm"}} },
		"ssh dir":                func(s *Settings) { s.Shares = []Share{{HostPath: testHome + "/.ssh"}} },
		"relative path":          func(s *Settings) { s.Shares = []Share{{HostPath: "notes"}} },
		"guest path off /mnt":    func(s *Settings) { s.Shares = []Share{{HostPath: testHome + "/x", GuestPath: "/etc"}} },
		"public lan docker pool": func(s *Settings) { s.Guest.DockerPool = "10.0.0.0/8" },
		"wildcard publish ip":    func(s *Settings) { s.Guest.PublishIP = "0.0.0.0" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			s := testSettings()
			mut(&s)
			if err := s.Validate(testLimits()); err == nil {
				t.Errorf("expected rejection")
			}
		})
	}
	if err := testSettings().Validate(testLimits()); err != nil {
		t.Errorf("defaults must validate: %v", err)
	}
	tiny := DefaultLimits(2048, 4, 50, testHome, testData)
	if err := testSettings().Validate(tiny); err == nil {
		t.Error("a host too small for the floor must be rejected")
	}
}

func TestLimaValidate(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	bin, err := exec.LookPath("limactl")
	if err != nil {
		t.Skip("limactl not installed")
	}
	for name, s := range map[string]Settings{"plain": testSettings(), "shares": func() Settings {
		s := testSettings()
		s.Shares = []Share{{HostPath: "/Users/test/Documents/notes"}}
		return s
	}()} {
		raw, err := Render(s, testLimits(), RenderOptions{})
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		f := filepath.Join(dir, "lr.yaml")
		if err := os.WriteFile(f, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "validate", f) //nolint:gosec // fixed binary from LookPath, temp file arg
		cmd.Env = append(os.Environ(), "LIMA_HOME="+filepath.Join(dir, "home"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: limactl validate: %v\n%s", name, err, out)
		}
	}
}

// Matches a bare 0.0.0.0 but not 10.0.0.0/8.
var wildcard = regexp.MustCompile(`(^|[^0-9.])0\.0\.0\.0`)
