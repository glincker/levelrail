package deploy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func nextSource(t *testing.T, next bool) string {
	t.Helper()
	dir := t.TempDir()
	pkg := `{"dependencies":{"react":"19.0.0"}}`
	if next {
		pkg = `{"dependencies":{"next":"15.1.0"}}`
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestNextDeploymentID(t *testing.T) {
	cases := []struct {
		name string
		next bool
		off  bool
		env  map[string]spec.EnvVar
		id   string
		want string
	}{
		{"next app gets id", true, false, nil, "att1", "att1"},
		{"user override wins", true, false, map[string]spec.EnvVar{"NEXT_DEPLOYMENT_ID": {Value: "mine"}}, "att1", ""},
		{"setting off", true, true, nil, "att1", ""},
		{"not next", false, false, nil, "att1", ""},
		{"no deploy id", true, false, nil, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvNextDeploymentID, map[bool]string{true: "false", false: "true"}[tc.off])
			got := nextDeploymentID(context.Background(), nextSource(t, tc.next), tc.id, tc.env)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestNextGraceBook(t *testing.T) {
	now := time.Now()
	ctx := context.Background()
	cases := []struct {
		name  string
		grace string
		old   string
		want  bool
	}{
		{"default window records", "", "d1", true},
		{"zero disables", "0", "d1", false},
		{"old release without id", "60", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.grace != "" {
				t.Setenv(EnvNextAssetGraceSecs, tc.grace)
			}
			b := NewNextGraceBook()
			if got := b.Begin(ctx, "web", tc.old, "img", now); got != tc.want {
				t.Fatalf("Begin = %v want %v", got, tc.want)
			}
		})
	}
	t.Run("expires and dedupes", func(t *testing.T) {
		t.Setenv(EnvNextAssetGraceSecs, "60")
		b := NewNextGraceBook()
		b.Begin(ctx, "web", "d1", "img", now)
		if b.Begin(ctx, "web", "d1", "img", now) {
			t.Fatal("duplicate recorded")
		}
		if n := len(b.Active("web", now.Add(30*time.Second))); n != 1 {
			t.Fatalf("active = %d", n)
		}
		if n := len(b.Active("web", now.Add(61*time.Second))); n != 0 {
			t.Fatalf("expired still active = %d", n)
		}
	})
}
