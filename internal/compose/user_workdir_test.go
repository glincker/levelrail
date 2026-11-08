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
