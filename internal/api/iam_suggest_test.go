package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestLeastPrivilegeSuggestions(t *testing.T) {
	now := time.Now()
	since := now.AddDate(0, 0, -30)
	recent := now.Add(-time.Hour)
	old := now.AddDate(0, 0, -90)
	revoked := now.Add(-time.Minute)
	tokens := []store.APIToken{
		{ID: "root-writer", Name: "ci", Abilities: []string{AbilityRoot}},
		{ID: "root-real", Name: "admin", Abilities: []string{AbilityRoot}},
		{ID: "deployer", Name: "deploy", Abilities: []string{AbilityRead, AbilityWrite, AbilityDeploy}},
		{ID: "full", Name: "full", Abilities: []string{AbilityRead, AbilityWrite}},
		{ID: "reader", Name: "reader", Abilities: []string{AbilityRead, AbilityWrite}, LastUsedAt: &recent},
		{ID: "stale", Name: "stale", Abilities: []string{AbilityRead}, LastUsedAt: &old},
		{ID: "gone", Name: "gone", Abilities: []string{AbilityRoot}, RevokedAt: &revoked},
	}
	usage := []store.TokenAbilityUsage{
		{TokenID: "root-writer", Ability: AbilityWrite, Requests: 4},
		{TokenID: "root-writer", Ability: AbilityDeploy, Requests: 2},
		{TokenID: "root-real", Ability: AbilityRoot, Requests: 1},
		{TokenID: "deployer", Ability: AbilityDeploy, Requests: 9},
		{TokenID: "full", Ability: AbilityWrite, Requests: 1},
	}
	got := leastPrivilegeSuggestions(tokens, usage, since, now)
	byID := map[string]tokenSuggestion{}
	for _, s := range got {
		byID[s.TokenID] = s
	}
	if _, ok := byID["gone"]; ok {
		t.Fatal("revoked token must not be listed")
	}
	cases := []struct {
		id        string
		status    string
		suggested []string
	}{
		{"root-writer", suggestNarrow, []string{AbilityRead, AbilityWrite, AbilityDeploy}},
		{"root-real", suggestOK, []string{AbilityRoot}},
		{"deployer", suggestNarrow, []string{AbilityRead, AbilityDeploy}},
		{"full", suggestOK, []string{AbilityRead, AbilityWrite}},
		{"reader", suggestNarrow, []string{AbilityRead}},
		{"stale", suggestIdle, []string{}},
	}
	for _, c := range cases {
		s, ok := byID[c.id]
		if !ok {
			t.Fatalf("%s missing", c.id)
		}
		if s.Status != c.status || !slices.Equal(s.SuggestedAbilities, c.suggested) {
			t.Errorf("%s: status %q suggested %v, want %q %v", c.id, s.Status, s.SuggestedAbilities, c.status, c.suggested)
		}
	}
	if byID["root-writer"].Risk != sevHigh {
		t.Errorf("unused root should be high risk, got %q", byID["root-writer"].Risk)
	}
}

func TestHandleIAMSuggestions(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveAPIToken(ctx, store.APIToken{ID: "t1", Name: "t1", TokenHash: "h1", Abilities: []string{AbilityRead, AbilityWrite}}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/iam/suggestions?days=7", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var res suggestionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.WindowDays != 7 || len(res.Tokens) != 1 || res.Tokens[0].Status != suggestIdle {
		t.Fatalf("unexpected response: %+v", res)
	}

	bad := httptest.NewRecorder()
	rt.Handler().ServeHTTP(bad, authedRequest(t, cookie, http.MethodGet, "/api/v1/iam/suggestions?days=0", ""))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("days=0 status %d", bad.Code)
	}
}
