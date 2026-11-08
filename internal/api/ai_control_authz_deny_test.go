package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

func aiControlDenyFeature() authzDenyFeature {
	return authzDenyFeature{
		name:  "ai-control",
		match: func(_, p string) bool { return strings.HasPrefix(p, "/api/v1/settings/ai-control") },
		routes: []authzDenyRoute{
			{http.MethodGet, "/api/v1/settings/ai-control", "", AbilityRead, false},
			{http.MethodPut, "/api/v1/settings/ai-control", `{"mode":"observe","allowed_env_kinds":["dev"]}`, AbilityRoot, false},
			{http.MethodPost, "/api/v1/settings/ai-control/revoke-agent-tokens", "", AbilityRoot, false},
		},
	}
}

func TestAuthzDeny_AIControl(t *testing.T) {
	runAuthzDeny(t, aiControlDenyFeature())
}

// An agent token never changes or revokes AI control, whatever its abilities or the current mode.
func TestAuthzDeny_AIControlAgentTokens(t *testing.T) {
	experimental.Set(experimental.AIControl)
	t.Cleanup(enableAllExperimental)
	for _, mode := range []string{store.AIControlModeOff, store.AIControlModeObserve, store.AIControlModeOperate, store.AIControlModeAdmin} {
		t.Run(mode, func(t *testing.T) {
			rt, db, tokens := gateFixture(t, mode, []string{"dev"})
			for label, tok := range tokens {
				agent := label != "human-root"
				for _, r := range aiControlDenyFeature().routes {
					if r.method == http.MethodGet {
						continue
					}
					rec := gateDo(rt, r.method, r.path(), tok, r.body)
					if agent && rec.Code != http.StatusForbidden {
						t.Errorf("mode %s: %s as %s = %d %s, want 403", mode, r.key(), label, rec.Code, rec.Body.String())
					}
					if !agent && (rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden) {
						t.Errorf("mode %s: %s as human root = %d, want it to reach the handler", mode, r.key(), rec.Code)
					}
				}
			}
			// Audit rows must never carry a token value, refused or not.
			entries, err := db.ListAuditEntries(context.Background(), 1000, nil, store.AuditEntryFilter{})
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				for _, tok := range tokens {
					if strings.Contains(e.Path, tok) || strings.Contains(e.Method, tok) {
						t.Fatalf("audit entry carries a token value: %+v", e)
					}
				}
			}
			for _, tok := range tokens {
				if rec := gateDo(rt, http.MethodGet, "/api/v1/settings/ai-control", tok, ""); strings.Contains(rec.Body.String(), tok) {
					t.Fatalf("response echoes a token value")
				}
			}
		})
	}
}

// A read-only ordinary token reads the settings but cannot write them.
func TestAuthzDeny_AIControlReadOnlyToken(t *testing.T) {
	a := newAuthzDenyActors(t)
	for _, r := range aiControlDenyFeature().routes {
		rec := a.do(r.method, r.path(), r.body, nil, a.readTok, "")
		want := http.StatusForbidden
		if r.ability == AbilityRead {
			want = http.StatusOK
		}
		if rec.Code != want {
			t.Errorf("%s with a read token = %d, want %d", r.key(), rec.Code, want)
		}
	}
	a.assertNoSecrets(t)
}
