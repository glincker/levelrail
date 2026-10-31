package deploy

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestStaticBuildArgsPublicEnvOnly(t *testing.T) {
	env := map[string]spec.EnvVar{
		"VITE_API_URL":     {Value: "https://api.example.test"},
		"PUBLIC_SITE":      {Value: "docs"},
		"DATABASE_URL":     {Value: "postgres://x"},
		"VITE_SECRET":      {Value: "s", Secret: true},
		"NEXT_PUBLIC_REF":  {From: "postgres.main.url"},
		"GATSBY_VAULT":     {Value: "v", Vault: &spec.VaultRef{Path: "a", Key: "b"}},
		"VITE_EXPLICIT":    {Value: "env"},
		"VITE_BAD-NAME":    {Value: "x"},
		"NEXT_PUBLIC_KEEP": {Value: "1"},
	}
	args := map[string]string{"VITE_EXPLICIT": "arg", "NODE_ENV": "production"}
	got := staticBuildArgs(env, args, []string{"VITE_", "NEXT_PUBLIC_", "PUBLIC_", "GATSBY_"})
	want := map[string]string{
		"VITE_API_URL": "https://api.example.test", "PUBLIC_SITE": "docs", "NEXT_PUBLIC_KEEP": "1",
		"VITE_EXPLICIT": "arg", "NODE_ENV": "production",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestStaticBuildEnvPrefixesEnv(t *testing.T) {
	t.Setenv("APP_STATIC_BUILD_ENV_PREFIXES", " ASTRO_PUBLIC_ , ,SITE_")
	got := StaticBuildEnvPrefixes()
	if len(got) != 2 || got[0] != "ASTRO_PUBLIC_" || got[1] != "SITE_" {
		t.Fatalf("got %v", got)
	}
	t.Setenv("APP_STATIC_BUILD_ENV_PREFIXES", "")
	if got := StaticBuildEnvPrefixes(); len(got) != 0 {
		t.Fatalf("empty setting should disable, got %v", got)
	}
}
