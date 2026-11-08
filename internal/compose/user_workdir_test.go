package compose

import (
	"strings"
	"testing"
)

func TestToDesiredServices_UserAndWorkingDir(t *testing.T) {
	f, err := Parse([]byte("services:\n  web:\n    image: nginx:1\n    user: \"${APP_UID:-1000}:1000\"\n    working_dir: /srv\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	gen := func(string, string, int) (string, error) { return "x", nil }
	if _, _, err := ResolveMagicVars(f, gen, func(string, string, string) error { return nil }); err != nil {
		t.Fatalf("ResolveMagicVars() error = %v", err)
	}
	for _, u := range f.Unsupported {
		if u.Key == "user" || u.Key == "working_dir" {
			t.Fatalf("%s is still reported as unsupported", u.Key)
		}
	}
	got, _, err := ToDesiredServices("app", f)
	if err != nil {
		t.Fatalf("ToDesiredServices() error = %v", err)
	}
	if got[0].User != "1000:1000" || got[0].WorkingDir != "/srv" {
		t.Fatalf("user/working_dir = %q/%q", got[0].User, got[0].WorkingDir)
	}
}

func TestToDesiredServices_InvalidUserFailsWithoutEcho(t *testing.T) {
	for _, body := range []string{"    user: \"evil user\"\n", "    working_dir: rel/dir\n"} {
		f, err := Parse([]byte("services:\n  web:\n    image: nginx:1\n" + body))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		_, _, err = ToDesiredServices("app", f)
		if err == nil || !strings.Contains(err.Error(), `service "web"`) {
			t.Fatalf("ToDesiredServices() error = %v, want a failure naming the service", err)
		}
		if strings.Contains(err.Error(), "evil user") || strings.Contains(err.Error(), "rel/dir") {
			t.Fatalf("error echoes the value: %v", err)
		}
	}
}

func TestToDesiredServices_RuntimeOptions(t *testing.T) {
	doc := "services:\n  web:\n    image: nginx:1\n    read_only: true\n    tmpfs: [/tmp]\n    cap_drop: [cap_net_raw, ALL]\n    stop_grace_period: 500ms\n    sysctls: {kernel.core_pattern: x}\n"
	f, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	got, warns, err := ToDesiredServices("app", f)
	if err != nil {
		t.Fatal(err)
	}
	d := got[0]
	if d.Resources == nil || d.Resources.ReadOnlyRootfs == nil || !*d.Resources.ReadOnlyRootfs {
		t.Fatal("read_only must reach Resources.ReadOnlyRootfs")
	}
	if d.Runtime.CapDrop[0] != "NET_RAW" || !strings.Contains(d.Runtime.Tmpfs[0], "size=") || *d.Runtime.StopTimeout != 1 || d.Runtime.Sysctls != nil {
		t.Fatalf("runtime = %+v", d.Runtime)
	}
	if !strings.Contains(strings.Join(warns, "\n"), "sysctls") {
		t.Fatalf("skipped option not named: %v", warns)
	}
	bad, _ := Parse([]byte("services:\n  web:\n    image: nginx:1\n    cap_drop: [BOGUS]\n"))
	if _, _, err := ToDesiredServices("app", bad); err == nil {
		t.Fatal("an invalid cap_drop must fail the deploy")
	}
}
