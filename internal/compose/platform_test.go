package compose

import (
	"strings"
	"testing"
)

func TestToDesiredServices_Platform(t *testing.T) {
	cases := []struct {
		name, platform, warn string
	}{
		{"foreign arch pinned and flagged", "linux/s390x", "emulation"},
		{"malformed value is named and not applied", "windows/amd64", "platform"},
	}
	for _, c := range cases {
		f, err := Parse([]byte("services:\n  web:\n    image: nginx:1\n    platform: " + c.platform + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		got, warns, err := ToDesiredServices("app", f)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !strings.Contains(strings.Join(warns, "\n"), c.warn) {
			t.Fatalf("%s: warnings = %v", c.name, warns)
		}
		pinned := got[0].Runtime != nil && got[0].Runtime.Platform == c.platform
		if pinned != (c.platform == "linux/s390x") {
			t.Fatalf("%s: runtime = %+v", c.name, got[0].Runtime)
		}
	}
}
