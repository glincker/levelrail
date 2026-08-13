package api

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
)

func TestParseDevFixtures_Valid(t *testing.T) {
	data := []byte(`
tokens:
  - name: dev-read-only
    plaintext: dev-read-token
    abilities: [read]
  - name: dev-root
    plaintext: dev-root-token
    abilities: [root]
`)
	f, err := ParseDevFixtures(data)
	if err != nil {
		t.Fatalf("ParseDevFixtures() error = %v", err)
	}
	if len(f.Tokens) != 2 {
		t.Fatalf("len(Tokens) = %d, want 2", len(f.Tokens))
	}
	if f.Tokens[0].Name != "dev-read-only" || f.Tokens[0].Plaintext != "dev-read-token" {
		t.Errorf("Tokens[0] = %+v", f.Tokens[0])
	}
}

func TestParseDevFixtures_RejectsMissingName(t *testing.T) {
	data := []byte(`
tokens:
  - plaintext: dev-read-token
    abilities: [read]
`)
	if _, err := ParseDevFixtures(data); err == nil {
		t.Fatal("ParseDevFixtures() error = nil, want error for missing name")
	}
}

func TestParseDevFixtures_RejectsMissingPlaintext(t *testing.T) {
	data := []byte(`
tokens:
  - name: dev-read-only
    abilities: [read]
`)
	if _, err := ParseDevFixtures(data); err == nil {
		t.Fatal("ParseDevFixtures() error = nil, want error for missing plaintext")
	}
}

func TestParseDevFixtures_RejectsInvalidAbilities(t *testing.T) {
	data := []byte(`
tokens:
  - name: dev-bogus
    plaintext: dev-bogus-token
    abilities: [not-a-real-ability]
`)
	if _, err := ParseDevFixtures(data); err == nil {
		t.Fatal("ParseDevFixtures() error = nil, want error for an unknown ability")
	}
}

func TestParseDevFixtures_RejectsDuplicateName(t *testing.T) {
	data := []byte(`
tokens:
  - name: dev-dup
    plaintext: token-a
    abilities: [read]
  - name: dev-dup
    plaintext: token-b
    abilities: [write]
`)
	if _, err := ParseDevFixtures(data); err == nil {
		t.Fatal("ParseDevFixtures() error = nil, want error for duplicate token name")
	}
}

func TestParseDevFixtures_RejectsDuplicatePlaintext(t *testing.T) {
	data := []byte(`
tokens:
  - name: dev-a
    plaintext: same-token
    abilities: [read]
  - name: dev-b
    plaintext: same-token
    abilities: [write]
`)
	if _, err := ParseDevFixtures(data); err == nil {
		t.Fatal("ParseDevFixtures() error = nil, want error for duplicate plaintext")
	}
}

func TestParseDevFixtures_EmptyFileIsValidNoTokens(t *testing.T) {
	f, err := ParseDevFixtures([]byte(``))
	if err != nil {
		t.Fatalf("ParseDevFixtures() error = %v", err)
	}
	if len(f.Tokens) != 0 {
		t.Errorf("len(Tokens) = %d, want 0", len(f.Tokens))
	}
}

func TestSeedDevFixtures_Disabled_NoOp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	fixtures := &DevFixtures{Tokens: []DevFixtureToken{
		{Name: "dev-read", Plaintext: "dev-read-token", Abilities: []string{AbilityRead}},
	}}

	if err := seedDevFixtures(ctx, db, logger, fixtures, false); err != nil {
		t.Fatalf("seedDevFixtures() error = %v", err)
	}

	if _, err := db.GetAPITokenByHash(ctx, hashToken("dev-read-token")); err == nil {
		t.Error("token was seeded despite enabled=false")
	}
}

func TestSeedDevFixtures_NilFixtures_NoOp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))

	if err := seedDevFixtures(ctx, db, logger, nil, true); err != nil {
		t.Fatalf("seedDevFixtures() error = %v", err)
	}
}

func TestSeedDevFixtures_Enabled_SeedsWorkingToken(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	fixtures := &DevFixtures{Tokens: []DevFixtureToken{
		{Name: "dev-read", Plaintext: "dev-read-token", Abilities: []string{AbilityRead}},
		{Name: "dev-root", Plaintext: "dev-root-token", Abilities: []string{AbilityRoot}},
	}}

	if err := seedDevFixtures(ctx, db, logger, fixtures, true); err != nil {
		t.Fatalf("seedDevFixtures() error = %v", err)
	}

	got, err := db.GetAPITokenByHash(ctx, hashToken("dev-read-token"))
	if err != nil {
		t.Fatalf("GetAPITokenByHash(dev-read-token) error = %v", err)
	}
	if len(got.Abilities) != 1 || got.Abilities[0] != AbilityRead {
		t.Errorf("Abilities = %v, want [read]", got.Abilities)
	}

	got2, err := db.GetAPITokenByHash(ctx, hashToken("dev-root-token"))
	if err != nil {
		t.Fatalf("GetAPITokenByHash(dev-root-token) error = %v", err)
	}
	if len(got2.Abilities) != 1 || got2.Abilities[0] != AbilityRoot {
		t.Errorf("Abilities = %v, want [root]", got2.Abilities)
	}
}

func TestSeedDevFixtures_Idempotent_SecondCallDoesNotError(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	fixtures := &DevFixtures{Tokens: []DevFixtureToken{
		{Name: "dev-read", Plaintext: "dev-read-token", Abilities: []string{AbilityRead}},
	}}

	if err := seedDevFixtures(ctx, db, logger, fixtures, true); err != nil {
		t.Fatalf("first seedDevFixtures() error = %v", err)
	}
	if err := seedDevFixtures(ctx, db, logger, fixtures, true); err != nil {
		t.Fatalf("second seedDevFixtures() error = %v, want idempotent no-op like a server restart", err)
	}

	tokens, err := db.ListAPITokens(ctx)
	if err != nil {
		t.Fatalf("ListAPITokens() error = %v", err)
	}
	count := 0
	for _, tok := range tokens {
		if tok.Name == "[dev fixture] dev-read" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("dev-read fixture token row count = %d, want exactly 1 after two seed calls", count)
	}
}

func TestSeedDevFixtures_LogsPlaintextForVisibility(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	fixtures := &DevFixtures{Tokens: []DevFixtureToken{
		{Name: "dev-read", Plaintext: "dev-read-token", Abilities: []string{AbilityRead}},
	}}

	if err := seedDevFixtures(ctx, db, logger, fixtures, true); err != nil {
		t.Fatalf("seedDevFixtures() error = %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("dev-read-token")) {
		t.Errorf("log output = %q, want it to mention the fixture plaintext so it's discoverable from startup logs", buf.String())
	}
}

func TestMaybeSeedDevFixturesFromFile_MissingFileIsNotAnError(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))

	// Fixtures are optional: a fresh checkout without a dev-fixtures.yml
	// must not fail startup, dev mode's admin bootstrap already works
	// without one.
	err := MaybeSeedDevFixturesFromFile(ctx, db, logger, "/nonexistent/dev-fixtures.yml")
	if err != nil {
		t.Fatalf("MaybeSeedDevFixturesFromFile() error = %v, want nil for a missing file", err)
	}
}
