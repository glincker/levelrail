package build

import (
	"context"
	"strings"
	"testing"
)

func TestSanitizeCacheID(t *testing.T) {
	cases := []struct{ id, tag, want string }{
		{"My_App.v2", "", "my-app-v2"},
		{"", "levelrail/thesvg:abc1234", "thesvg"},
		{"../etc/passwd", "", "etc-passwd"},
		{"", "plain:tag", "plain"},
		{"---", "", ""},
	}
	for _, c := range cases {
		if got := SanitizeCacheID(c.id, c.tag); got != c.want {
			t.Errorf("SanitizeCacheID(%q,%q)=%q want %q", c.id, c.tag, got, c.want)
		}
	}
	if got := SanitizeCacheID(strings.Repeat("a", 80), ""); len(got) > maxCacheIDLen {
		t.Errorf("not capped: %d", len(got))
	}
}

func TestSanitizeCacheIDLongNamesUnique(t *testing.T) {
	prefix := strings.Repeat("a", 50)
	a, b := SanitizeCacheID(prefix+"-one", ""), SanitizeCacheID(prefix+"-two", "")
	if a == b || len(a) > maxCacheIDLen || len(b) > maxCacheIDLen {
		t.Fatalf("collision or too long: %q %q", a, b)
	}
	if SanitizeCacheID(a, "") != a || SanitizeCacheID(prefix+"-one", "") != a {
		t.Errorf("not stable: %q", a)
	}
}

func TestPerAppCacheEnabled(t *testing.T) {
	ctx := context.Background()
	t.Setenv(EnvBuildCachePerApp, "")
	if !perAppCacheEnabled(ctx) {
		t.Fatal("default must be on")
	}
	t.Setenv(EnvBuildCachePerApp, "off")
	if perAppCacheEnabled(ctx) {
		t.Fatal("env off ignored")
	}
	SetCachePerAppResolver(func(context.Context) (bool, error) { return true, nil })
	t.Cleanup(func() { SetCachePerAppResolver(nil) })
	if !perAppCacheEnabled(ctx) {
		t.Fatal("runtime setting must win over env")
	}
}
