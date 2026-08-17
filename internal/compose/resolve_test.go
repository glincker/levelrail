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

func TestResolveMagicVars_FQDNResolvesToOwnDomain(t *testing.T) {
	f, err := Parse([]byte(`
x-levelrail-domains:
  app: app.example.com
services:
  app:
    image: app:latest
    environment:
      FRONTEND_URL: ${SERVICE_FQDN_APP}
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
		t.Errorf("secretEnv = %+v, want none (FQDN is a literal URL, not a secret)", secretEnv)
	}
	if got := f.Services["app"].Environment["FRONTEND_URL"]; got != "https://app.example.com" {
		t.Errorf("FRONTEND_URL = %q, want https://app.example.com", got)
	}
}

func TestResolveMagicVars_FQDNEmbeddedResolvesToOwnDomain(t *testing.T) {
	f, err := Parse([]byte(`
x-levelrail-domains:
  app: app.example.com
services:
  app:
    image: app:latest
    environment:
      CALLBACK_URL: "${SERVICE_FQDN_APP}/oauth/callback"
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
	if got := f.Services["app"].Environment["CALLBACK_URL"]; got != "https://app.example.com/oauth/callback" {
		t.Errorf("CALLBACK_URL = %q, want https://app.example.com/oauth/callback", got)
	}
}

func TestResolveMagicVars_FQDNIgnoresOtherServicesDomain(t *testing.T) {
	f, err := Parse([]byte(`
x-levelrail-domains:
  web: web.example.com
services:
  web:
    image: web:latest
  worker:
    image: worker:latest
    environment:
      FRONTEND_URL: ${SERVICE_FQDN_WEB}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	_, unresolved, err := ResolveMagicVars(f, failGenerate(t), failPersist(t))
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) != 1 {
		t.Fatalf("unresolved = %+v, want exactly one entry (worker has no domain of its own, the FQDN key naming web is not what resolves it)", unresolved)
	}
}

func TestValidate_UnknownDomainServiceRejected(t *testing.T) {
	f, err := Parse([]byte(`
x-levelrail-domains:
  typo-service: app.example.com
services:
  app:
    image: app:latest
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := f.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want an error for a domain mapped to an unknown service")
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

func TestResolveMagicVars_EmbeddedGeneratableTokenIsSpliced(t *testing.T) {
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

	var persisted string
	persist := func(_, _, value string) error {
		persisted = value
		return nil
	}
	secretEnv, unresolved, err := ResolveMagicVars(f, func(_, _ string, _ int) (string, error) {
		return "generated-pw", nil //nolint:gosec // test fixture value, not a real credential
	}, persist)
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) != 0 {
		t.Errorf("unresolved = %+v, want none", unresolved)
	}
	want := "postgres://user:generated-pw@db/app" //nolint:gosec // test fixture value, not a real credential
	if persisted != want {
		t.Errorf("persisted = %q, want %q", persisted, want)
	}
	if got := secretEnv["app"]; len(got) != 1 || got[0] != "URL" {
		t.Errorf("secretEnv[app] = %v, want [URL]", got)
	}
	if _, ok := f.Services["app"].Environment["URL"]; ok {
		t.Error("URL is still in Environment after resolving to a secret, want it removed")
	}
}

func TestResolveMagicVars_EmbeddedSharesGeneratedValueWithWholeValueReference(t *testing.T) {
	f, err := Parse([]byte(`
services:
  app:
    image: app:latest
    environment:
      DB_PASSWORD: $SERVICE_PASSWORD_DB
      DATABASE_URL: "postgres://user:$SERVICE_PASSWORD_DB@db/app"
  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	calls := 0
	generate := func(_, _ string, _ int) (string, error) {
		calls++
		return "shared-pw", nil //nolint:gosec // test fixture value, not a real credential
	}
	persisted := map[string]string{}
	persist := func(svcKey, envKey, value string) error {
		persisted[svcKey+"/"+envKey] = value
		return nil
	}

	_, unresolved, err := ResolveMagicVars(f, generate, persist)
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) != 0 {
		t.Errorf("unresolved = %+v, want none", unresolved)
	}
	if calls != 1 {
		t.Errorf("generate called %d times, want 1 (one shared key across a whole-value and an embedded reference)", calls)
	}
	if persisted["app/DB_PASSWORD"] != "shared-pw" {
		t.Errorf("app/DB_PASSWORD = %q, want shared-pw", persisted["app/DB_PASSWORD"])
	}
	wantURL := "postgres://user:shared-pw@db/app" //nolint:gosec // test fixture value, not a real credential
	if persisted["app/DATABASE_URL"] != wantURL {
		t.Errorf("app/DATABASE_URL = %q, want %q", persisted["app/DATABASE_URL"], wantURL)
	}
	if persisted["db/POSTGRES_PASSWORD"] != "shared-pw" {
		t.Errorf("db/POSTGRES_PASSWORD = %q, want shared-pw", persisted["db/POSTGRES_PASSWORD"])
	}
}

func TestResolveMagicVars_EmbeddedDefaultlessNonGeneratableTokenIsUnresolved(t *testing.T) {
	f, err := Parse([]byte(`
services:
  app:
    image: app:latest
    environment:
      URL: "postgres://user@db/${SERVICE_DB_NAME}"
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	_, unresolved, err := ResolveMagicVars(f, failGenerate(t), failPersist(t))
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) != 1 {
		t.Fatalf("unresolved = %+v, want exactly one entry (no default, not generatable)", unresolved)
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
