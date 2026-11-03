package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/spec"
)

type memNextSecrets struct {
	SecretSetter
	vals map[string]string
}

func (m *memNextSecrets) Exists(_ context.Context, svc, key string) (bool, error) {
	_, ok := m.vals[svc+"/"+key]
	return ok, nil
}

func (m *memNextSecrets) SetValueGuarded(_ context.Context, svc, key, v string, _ bool) error {
	m.vals[svc+"/"+key] = v
	return nil
}

func nextSource(t *testing.T, dep string) string {
	t.Helper()
	dir := t.TempDir()
	pkg := `{"name":"a","dependencies":{"` + dep + `":"15.1.0"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestNextjsActionsKey(t *testing.T) {
	ctx := context.Background()
	newRT := func() (*Router, *memNextSecrets) {
		m := &memNextSecrets{vals: map[string]string{}}
		return &Router{secrets: m, logger: slog.Default()}, m
	}
	t.Run("generated once and stable across deploys", func(t *testing.T) {
		rt, m := newRT()
		src := nextSource(t, "next")
		req := &deploy.Request{SourceDir: src}
		rt.ensureNextActionsKey(ctx, req, "web", "Next.js")
		first := m.vals["web/"+nextActionsKeyEnv]
		raw, err := base64.StdEncoding.DecodeString(first)
		if err != nil || len(raw) != 32 {
			t.Fatalf("key = %q, err %v", first, err)
		}
		if ev := req.Service.Env[nextActionsKeyEnv]; !ev.Secret || ev.Value != "" {
			t.Fatalf("env var must be a value-less secret ref: %+v", ev)
		}
		req2 := &deploy.Request{SourceDir: src}
		rt.ensureNextActionsKey(ctx, req2, "web", "Next.js")
		if m.vals["web/"+nextActionsKeyEnv] != first || !req2.Service.Env[nextActionsKeyEnv].Secret {
			t.Fatal("second deploy rotated or dropped the key")
		}
		for k, v := range deploy.StaticBuildArgsForTest(req.Service.Env) {
			if k == nextActionsKeyEnv || v == first {
				t.Fatal("key reached build args")
			}
		}
	})
	t.Run("not for non-Next apps or old Next", func(t *testing.T) {
		rt, m := newRT()
		rt.ensureNextActionsKey(ctx, &deploy.Request{SourceDir: nextSource(t, "next")}, "a", "Express")
		rt.ensureNextActionsKey(ctx, &deploy.Request{SourceDir: nextSource(t, "express")}, "b", "Next.js")
		old := t.TempDir()
		_ = os.WriteFile(filepath.Join(old, "package.json"), []byte(`{"dependencies":{"next":"13.5.0"}}`), 0o600)
		rt.ensureNextActionsKey(ctx, &deploy.Request{SourceDir: old}, "c", "Next.js")
		if len(m.vals) != 0 {
			t.Fatalf("unexpected secrets: %v", len(m.vals))
		}
	})
	t.Run("user set key wins", func(t *testing.T) {
		rt, m := newRT()
		req := &deploy.Request{SourceDir: nextSource(t, "next")}
		req.Service.Env = map[string]spec.EnvVar{nextActionsKeyEnv: {Value: "mine"}}
		rt.ensureNextActionsKey(ctx, req, "web", "Next.js")
		if len(m.vals) != 0 || req.Service.Env[nextActionsKeyEnv].Value != "mine" {
			t.Fatal("user key was replaced")
		}
	})
	t.Run("setting off disables", func(t *testing.T) {
		db := openTestDB(t)
		svc := runtimesettings.New(db, func(string) (string, bool) { return "", false })
		if _, err := svc.Set(ctx, runtimesettings.KeyNextjsGenerateActionsKey, json.RawMessage("false"), "test"); err != nil {
			t.Fatal(err)
		}
		rt, m := newRT()
		rt.runtimeSettings = svc
		rt.ensureNextActionsKey(ctx, &deploy.Request{SourceDir: nextSource(t, "next")}, "web", "Next.js")
		if len(m.vals) != 0 {
			t.Fatal("generated while the setting is off")
		}
	})
}
