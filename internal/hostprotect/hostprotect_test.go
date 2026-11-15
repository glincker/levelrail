package hostprotect

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func noEnv(string) string { return "" }

// The installer heredoc is the other writer of this file; expanding it with the
// defaults must give exactly what Render returns.
func TestRenderMatchesInstallScript(t *testing.T) {
	src, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)render_protection\(\) \{.*?cat <<EOF\n(.*?)\nEOF\n`).FindSubmatch(src)
	if m == nil {
		t.Fatal("render_protection heredoc not found in install.sh")
	}
	got := strings.NewReplacer(
		"${PROTECT_MARK}", Marker, "${DROPIN_DIR}", "/etc/systemd/system/levelrail.service.d", "${SERVICE_NAME}", "levelrail",
		"${window}", "600", "${burst}", "20", "${oom}", "-900", "${rsec}", "5", "${mlow}", "256M",
	).Replace(string(m[1])) + "\n"
	if strings.Contains(got, "${") {
		t.Fatalf("install.sh heredoc uses a variable Render does not know:\n%s", got)
	}
	want, err := Render("levelrail", noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("drop-in drifted from install.sh\n--- install.sh\n%s\n--- Render\n%s", got, want)
	}
}

func TestRenderRejectsBadValues(t *testing.T) {
	env := func(k string) string {
		if k == "LEVELRAIL_RESTART_SEC" {
			return "5;rm"
		}
		return ""
	}
	if _, err := Render("levelrail", env); err == nil {
		t.Fatal("want an error for a non integer value")
	}
}

func testHost(t *testing.T, uid int, env map[string]string) (Host, *int) {
	t.Helper()
	root := t.TempDir()
	run := filepath.Join(root, "run")
	units := filepath.Join(root, "units")
	for _, d := range []string{run, units} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(units, "levelrail.service"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	reloads := 0
	return Host{Service: "levelrail", UnitDir: units, SystemdRun: run, Geteuid: func() int { return uid },
		Getenv: func(k string) string { return env[k] }, Reload: func(context.Context) error { reloads++; return nil }}, &reloads
}

func TestApplyIdempotentAndReloadOnly(t *testing.T) {
	h, reloads := testHost(t, 0, nil)
	if r, err := h.Apply(context.Background()); err != nil || r != Wrote {
		t.Fatalf("first apply: %v %v", r, err)
	}
	if r, _ := h.Apply(context.Background()); r != Current || *reloads != 1 {
		t.Fatalf("second apply: %v reloads=%d", r, *reloads)
	}
}

func TestApplySkipsAndKeeps(t *testing.T) {
	cases := map[string]func() (Host, *int){
		"not root":  func() (Host, *int) { return testHost(t, 1000, nil) },
		"opted out": func() (Host, *int) { return testHost(t, 0, map[string]string{EnvOptOut: "0"}) },
		"no systemd": func() (Host, *int) {
			h, n := testHost(t, 0, nil)
			h.SystemdRun = filepath.Join(h.UnitDir, "missing")
			return h, n
		},
		"no unit": func() (Host, *int) {
			h, n := testHost(t, 0, nil)
			_ = os.Remove(filepath.Join(h.UnitDir, "levelrail.service"))
			return h, n
		},
	}
	for name, mk := range cases {
		h, reloads := mk()
		if r, err := h.Apply(context.Background()); err != nil || r != Skipped || *reloads != 0 {
			t.Fatalf("%s: %v %v reloads=%d", name, r, err, *reloads)
		}
	}
	h, _ := testHost(t, 0, nil)
	dir := filepath.Join(h.UnitDir, "levelrail.service.d")
	_ = os.MkdirAll(dir, 0o750)
	_ = os.WriteFile(filepath.Join(dir, "30-protection.conf"), []byte("[Service]\nOOMScoreAdjust=-100\n"), 0o600)
	if r, _ := h.Apply(context.Background()); r != Kept {
		t.Fatalf("hand edited file must be kept, got %v", r)
	}
}
