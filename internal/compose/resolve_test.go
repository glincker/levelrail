package compose

import (
	"fmt"
	"testing"
)

func TestResolveMagicVars_GeneratedSharedAcrossServices(t *testing.T) {
	f, err := Parse([]byte(`
services:
  app:
    image: app:latest
    environment:
      DB_PASSWORD: $SERVICE_PASSWORD_DB
  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	generateCalls := 0
	generate := func(_, _ string, _ int) (string, error) {
		generateCalls++
		return "generated-value", nil
	}
	var persisted []string
	persist := func(svcKey, envKey, value string) error {
		persisted = append(persisted, fmt.Sprintf("%s/%s=%s", svcKey, envKey, value))
		return nil
	}

	secretEnv, unresolved, err := ResolveMagicVars(f, generate, persist)
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) != 0 {
		t.Errorf("unresolved = %+v, want none", unresolved)
	}
	if generateCalls != 1 {
		t.Errorf("generate called %d times, want 1 (shared key across two services)", generateCalls)
	}
	if len(persisted) != 2 {
		t.Fatalf("persist called %d times, want 2, got %v", len(persisted), persisted)
	}

	if got := secretEnv["app"]; len(got) != 1 || got[0] != "DB_PASSWORD" {
		t.Errorf("secretEnv[app] = %v, want [DB_PASSWORD]", got)
	}
	if got := secretEnv["db"]; len(got) != 1 || got[0] != "POSTGRES_PASSWORD" {
		t.Errorf("secretEnv[db] = %v, want [POSTGRES_PASSWORD]", got)
	}

	if _, ok := f.Services["app"].Environment["DB_PASSWORD"]; ok {
		t.Error("DB_PASSWORD is still in Environment after resolving to a secret, want it removed")
	}
}

func TestResolveMagicVars_DefaultSubstitutedAsLiteral(t *testing.T) {
	f, err := Parse([]byte(`
services:
  db:
    image: postgres:16
    environment:
      POSTGRES_DB: ${SERVICE_DB_NAME:-ente_db}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	secretEnv, unresolved, err := ResolveMagicVars(f, failGenerate(t), failPersist(t))
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) != 0 {
		t.Errorf("unresolved = %+v, want none", unresolved)
	}
	if len(secretEnv) != 0 {
		t.Errorf("secretEnv = %+v, want none (default-substituted values are literal)", secretEnv)
	}
	if got := f.Services["db"].Environment["POSTGRES_DB"]; got != "ente_db" {
		t.Errorf("POSTGRES_DB = %q, want ente_db", got)
	}
}

func TestResolveMagicVars_NoDefaultIsUnresolved(t *testing.T) {
	f, err := Parse([]byte(`
services:
  app:
    image: app:latest
    environment:
      API_KEY: ${SERVICE_OPENAI_API_KEY}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	_, unresolved, err := ResolveMagicVars(f, failGenerate(t), failPersist(t))
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) != 1 {
		t.Fatalf("unresolved = %+v, want exactly one entry", unresolved)
	}
	if unresolved[0].Service != "app" || unresolved[0].EnvKey != "API_KEY" {
		t.Errorf("unresolved[0] = %+v, want service=app envKey=API_KEY", unresolved[0])
	}
}

func TestResolveMagicVars_FQDNIsUnresolved(t *testing.T) {
	f, err := Parse([]byte(`
services:
  app:
    image: app:latest
    environment:
      FRONTEND_URL: ${SERVICE_FQDN_APP}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	_, unresolved, err := ResolveMagicVars(f, failGenerate(t), failPersist(t))
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) != 1 {
		t.Fatalf("unresolved = %+v, want exactly one entry", unresolved)
	}
}

func TestResolveMagicVars_EmbeddedTokenUsesDefault(t *testing.T) {
	f, err := Parse([]byte(`
services:
  app:
    image: app:latest
    environment:
      URL: "https://${SERVICE_DB_HOST:-localhost}:5432/app"
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	_, unresolved, err := ResolveMagicVars(f, failGenerate(t), failPersist(t))
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) != 0 {
		t.Errorf("unresolved = %+v, want none", unresolved)
	}
	if got := f.Services["app"].Environment["URL"]; got != "https://localhost:5432/app" {
		t.Errorf("URL = %q, want https://localhost:5432/app", got)
	}
}

func TestResolveMagicVars_EmbeddedGeneratableTokenIsUnresolved(t *testing.T) {
	f, err := Parse([]byte(`
services:
  app:
    image: app:latest
    environment:
      URL: "postgres://user:$SERVICE_PASSWORD_DB@db/app"
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	_, unresolved, err := ResolveMagicVars(f, failGenerate(t), failPersist(t))
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) != 1 {
		t.Fatalf("unresolved = %+v, want exactly one entry (a generated secret can't be spliced into a literal string)", unresolved)
	}
}

func TestResolveMagicVars_GenerateErrorPropagates(t *testing.T) {
	f, err := Parse([]byte(`
services:
  app:
    image: app:latest
    environment:
      SECRET: $SERVICE_PASSWORD_X
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	_, _, err = ResolveMagicVars(f, func(_, _ string, _ int) (string, error) {
		return "", fmt.Errorf("no secrets manager configured")
	}, failPersist(t))
	if err == nil {
		t.Fatal("ResolveMagicVars() error = nil, want an error propagated from generate")
	}
}

func failGenerate(t *testing.T) func(string, string, int) (string, error) {
	t.Helper()
	return func(kind, key string, _ int) (string, error) {
		t.Fatalf("generate unexpectedly called for %s_%s", kind, key)
		return "", nil
	}
}

func failPersist(t *testing.T) func(string, string, string) error {
	t.Helper()
	return func(svcKey, envKey string, _ string) error {
		t.Fatalf("persist unexpectedly called for %s/%s", svcKey, envKey)
		return nil
	}
}
