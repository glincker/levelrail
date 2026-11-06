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

type memGraceStore struct{ rows map[string]NextAssetGrace }

func (m *memGraceStore) key(g NextAssetGrace) string { return g.Service + "/" + g.DeploymentID }
func (m *memGraceStore) InsertNextAssetGrace(_ context.Context, g NextAssetGrace) error {
	m.rows[m.key(g)] = g
	return nil
}
func (m *memGraceStore) ListActiveNextAssetGrace(_ context.Context, now time.Time) ([]NextAssetGrace, error) {
	var out []NextAssetGrace
	for _, g := range m.rows {
		if g.Until.After(now) {
			out = append(out, g)
		}
	}
	return out, nil
}
func (m *memGraceStore) DeleteExpiredNextAssetGrace(_ context.Context, now time.Time) error {
	for k, g := range m.rows {
		if !g.Until.After(now) {
			delete(m.rows, k)
		}
	}
	return nil
}
func (m *memGraceStore) DeleteNextAssetGraceForService(_ context.Context, s string) error {
	for k, g := range m.rows {
		if g.Service == s {
			delete(m.rows, k)
		}
	}
	return nil
}

func TestNextGraceBookSurvivesRestartAndCapsGenerations(t *testing.T) {
	t.Setenv(EnvNextAssetGraceSecs, "600")
	ctx := context.Background()
	now := time.Now()
	st := &memGraceStore{rows: map[string]NextAssetGrace{}}
	b := NewNextGraceBook().WithStore(st)
	b.Begin(ctx, "web", "d1", "img1", now)
	b.Begin(ctx, "web", "d2", "img2", now.Add(time.Minute))
	// Default cap is 1: only the newest generation stays held.
	if act := b.Active("web", now.Add(2*time.Minute)); len(act) != 1 || act[0].DeploymentID != "d2" {
		t.Fatalf("active = %+v", act)
	}
	restarted := NewNextGraceBook().WithStore(st)
	if err := restarted.Load(ctx, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if act := restarted.Active("web", now.Add(2*time.Minute)); len(act) != 1 || act[0].DeploymentID != "d2" {
		t.Fatalf("after restart = %+v", act)
	}
	if !restarted.HeldUntil("web", now).After(now.Add(10 * time.Minute)) {
		t.Fatal("HeldUntil must be the end of the window")
	}
}
