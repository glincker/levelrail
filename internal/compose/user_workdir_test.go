package compose

import (
	"strings"
	"testing"
)

func TestToDesiredServices_UserAndWorkingDir(t *testing.T) {
	f, err := Parse([]byte("services:\n  web:\n    image: nginx:1\n    user: \"1000:1000\"\n    working_dir: /srv\n  bad:\n    image: nginx:1\n    user: \"${UID}\"\n    working_dir: rel\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	for _, u := range f.Unsupported {
		if u.Key == "user" || u.Key == "working_dir" {
			t.Fatalf("%s is still reported as unsupported", u.Key)
		}
	}
	got, warnings, err := ToDesiredServices("app", f)
	if err != nil {
		t.Fatalf("ToDesiredServices() error = %v", err)
	}
	byName := map[string]int{}
	for i, d := range got {
		byName[d.Name] = i
	}
	if web := got[byName["app-web"]]; web.User != "1000:1000" || web.WorkingDir != "/srv" {
		t.Fatalf("web user/working_dir = %q/%q", web.User, web.WorkingDir)
	}
	if bad := got[byName["app-bad"]]; bad.User != "" || bad.WorkingDir != "" {
		t.Fatalf("invalid values must fall back to the image defaults: %q/%q", bad.User, bad.WorkingDir)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "user:") || !strings.Contains(joined, "working_dir:") || strings.Contains(joined, "${UID}") {
		t.Fatalf("warnings = %q", joined)
	}
}
