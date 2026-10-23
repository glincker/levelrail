package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func resetDevFixtureRegistry(t *testing.T) {
	t.Helper()
	wipe := func() {
		seededDevFixtures.Lock()
		seededDevFixtures.byID = map[string]DevFixtureToken{}
		seededDevFixtures.Unlock()
	}
	wipe()
	t.Cleanup(wipe)
}

// listDevFixtureFlags seeds one fixture plus a same-name real token with
// different abilities, then returns the dev_fixture flag per token id.
func listDevFixtureFlags(t *testing.T, devMode bool) map[string]bool {
	t.Helper()
	resetDevFixtureRegistry(t)
	if devMode {
		t.Setenv("APP_DEV_MODE", "1")
	} else {
		t.Setenv("APP_DEV_MODE", "")
	}
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	fixtures := &DevFixtures{Tokens: []DevFixtureToken{{Name: "dev-read", Plaintext: "fixture-plain", Abilities: []string{AbilityRead}}}}
	// The seed itself only runs in dev mode (MaybeSeedDevFixturesFromFile
	// gates on it), so outside it the row exists but is never recorded.
	if devMode {
		if err := seedDevFixtures(ctx, db, slog.New(slog.DiscardHandler), fixtures, true); err != nil {
			t.Fatalf("seed: %v", err)
		}
	} else if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: devFixtureIDPrefix + "dev-read", Name: devFixtureNamePrefix + "dev-read", TokenHash: "h-fx",
		Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed row: %v", err)
	}
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_real", Name: devFixtureNamePrefix + "dev-read", TokenHash: "h-real",
		Abilities: []string{AbilityRead, AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed real: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/auth/tokens", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got []tokenResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	flags := map[string]bool{}
	for _, tok := range got {
		flags[tok.ID] = tok.DevFixture
	}
	return flags
}

func TestListTokens_FlagsDevFixtureInDevMode(t *testing.T) {
	flags := listDevFixtureFlags(t, true)
	if !flags[devFixtureIDPrefix+"dev-read"] {
		t.Error("seeded fixture token not flagged dev_fixture")
	}
	if flags["tok_real"] {
		t.Error("real token sharing the fixture name was flagged")
	}
}

func TestListTokens_NoFlagWhenDevModeOff(t *testing.T) {
	flags := listDevFixtureFlags(t, false)
	if len(flags) != 2 {
		t.Fatalf("got %d tokens, want 2", len(flags))
	}
	for id, on := range flags {
		if on {
			t.Errorf("token %s flagged with dev mode off", id)
		}
	}
}

func TestIsDevFixtureToken_RecordedButDevModeTurnedOff(t *testing.T) {
	resetDevFixtureRegistry(t)
	recordDevFixture(DevFixtureToken{Name: "x", Abilities: []string{AbilityRead}})
	tok := store.APIToken{ID: devFixtureIDPrefix + "x", Name: devFixtureNamePrefix + "x", Abilities: []string{AbilityRead}}
	t.Setenv("APP_DEV_MODE", "1")
	if !isDevFixtureToken(tok) {
		t.Fatal("want flagged in dev mode")
	}
	t.Setenv("APP_DEV_MODE", "")
	if isDevFixtureToken(tok) {
		t.Fatal("want not flagged outside dev mode")
	}
}
