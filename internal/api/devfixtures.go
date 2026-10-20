package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DevFixtureToken is one entry in a dev-fixtures.yml file: a fixed
// plaintext API token, minted with a known ability set, so local
// testing of the auth flow (does a read-only token get 403'd on a
// write route, does a write:sensitive token reach the secrets handler,
// and so on) doesn't require registering, logging in, and minting a
// real token by hand every time. Only ever seeded under dev mode, see
// devmode_debug.go/devmode_release.go's devModeEnabled gate.
type DevFixtureToken struct {
	Name      string   `yaml:"name"`
	Plaintext string   `yaml:"plaintext"`
	Abilities []string `yaml:"abilities"`
}

// DevFixtures is the top-level shape of dev-fixtures.yml.
type DevFixtures struct {
	Tokens []DevFixtureToken `yaml:"tokens"`
}

// ParseDevFixtures validates as much as validateAbilities already
// validates a real token mint request: an empty or unrecognized
// ability list is rejected here too, so a typo in dev-fixtures.yml
// fails loudly at startup instead of silently minting a token nobody
// can use as intended.
func ParseDevFixtures(data []byte) (*DevFixtures, error) {
	var f DevFixtures
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("api: parse dev fixtures: %w", err)
	}

	seenNames := make(map[string]bool, len(f.Tokens))
	seenPlaintexts := make(map[string]bool, len(f.Tokens))
	for i, tok := range f.Tokens {
		if tok.Name == "" {
			return nil, fmt.Errorf("api: dev fixtures: token[%d]: name is required", i)
		}
		if tok.Plaintext == "" {
			return nil, fmt.Errorf("api: dev fixtures: token %q: plaintext is required", tok.Name)
		}
		if seenNames[tok.Name] {
			return nil, fmt.Errorf("api: dev fixtures: duplicate token name %q", tok.Name)
		}
		seenNames[tok.Name] = true
		if seenPlaintexts[tok.Plaintext] {
			return nil, fmt.Errorf("api: dev fixtures: token %q: duplicate plaintext", tok.Name)
		}
		seenPlaintexts[tok.Plaintext] = true
		if err := validateAbilities(tok.Abilities); err != nil {
			return nil, fmt.Errorf("api: dev fixtures: token %q: %w", tok.Name, err)
		}
	}
	return &f, nil
}

// seedDevFixtures is MaybeSeedDevFixturesFromFile's testable core: a
// pure no-op when enabled is false or fixtures is nil, matching
// maybeBootstrapDevAdmin's shape in devmode.go. Idempotent by design,
// the same property BootstrapAdmin already relies on for the dev admin
// account: seeding twice (a server restart against the same data
// directory) skips any fixture token whose hash already exists rather
// than erroring on a primary-key conflict.
func seedDevFixtures(ctx context.Context, s TokenStore, logger *slog.Logger, fixtures *DevFixtures, enabled bool) error {
	if !enabled || fixtures == nil {
		return nil
	}
	for _, tok := range fixtures.Tokens {
		hash := hashToken(tok.Plaintext)
		if _, err := s.GetAPITokenByHash(ctx, hash); err == nil {
			continue // already seeded by a previous startup
		} else if !errors.Is(err, store.ErrAPITokenNotFound) {
			return fmt.Errorf("api: dev fixtures: check existing token %q: %w", tok.Name, err)
		}

		if err := s.SaveAPIToken(ctx, store.APIToken{
			ID:        "devfixture-" + tok.Name,
			Name:      "[dev fixture] " + tok.Name,
			TokenHash: hash,
			Abilities: tok.Abilities,
			CreatedAt: time.Now(),
		}); err != nil {
			return fmt.Errorf("api: dev fixtures: seed token %q: %w", tok.Name, err)
		}
		logger.Warn("dev fixture token seeded",
			slog.String("name", tok.Name),
			slog.String("plaintext", tok.Plaintext),
			slog.Any("abilities", tok.Abilities),
		)
	}
	return nil
}

// MaybeSeedDevFixturesFromFile is cmd/levelrail/main.go's entry point,
// called unconditionally on every startup the same way
// MaybeBootstrapDevAdmin already is. A missing fixtures file is not an
// error: dev mode works without one (only the fixed dev/dev admin
// account), a dev-fixtures.yml is an opt-in convenience layered on top.
func MaybeSeedDevFixturesFromFile(ctx context.Context, s TokenStore, logger *slog.Logger, path string) error {
	if !devModeEnabled() {
		return nil
	}

	// path is operator-supplied local dev tooling, not attacker-controlled
	// request input, the same reasoning openStore's own gosec exemption
	// in cmd/levelrail/main.go already documents.
	data, err := os.ReadFile(path) //nolint:gosec // caller-controlled dev-only config path, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("api: dev fixtures: read %s: %w", path, err)
	}

	fixtures, err := ParseDevFixtures(data)
	if err != nil {
		return err
	}
	return seedDevFixtures(ctx, s, logger, fixtures, true)
}
