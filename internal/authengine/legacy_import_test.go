package authengine_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

func legacyHash(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func TestImportLegacyTokenRules(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	tests := []struct {
		name       string
		tok        store.APIToken
		wantErr    error
		wantReason string
	}{
		{name: "owned read imports", tok: store.APIToken{Abilities: []string{"read"}}},
		{name: "system token stays legacy", tok: store.APIToken{Abilities: []string{"root"}, OwnerUserID: "-"}, wantErr: authengine.ErrLegacyNotOwned},
		{name: "revoked", tok: store.APIToken{Abilities: []string{"read"}, RevokedAt: &past}, wantReason: authengine.LegacySkipRevoked},
		{name: "expired", tok: store.APIToken{Abilities: []string{"read"}, ExpiresAt: &past}, wantReason: authengine.LegacySkipExpired},
		{name: "malformed hash", tok: store.APIToken{Abilities: []string{"read"}, TokenHash: "zz"}, wantReason: authengine.LegacySkipBadHash},
		{name: "unknown ability", tok: store.APIToken{Abilities: []string{"read", "admin"}}, wantReason: authengine.LegacySkipUnknownAbility},
		{name: "no abilities", tok: store.APIToken{Abilities: []string{}}, wantReason: authengine.LegacySkipUnknownAbility},
		{name: "sign-in approval is never imported", tok: store.APIToken{Abilities: []string{"signin:approve"}}, wantReason: authengine.LegacySkipUnknownAbility},
		{name: "owner gone", tok: store.APIToken{Abilities: []string{"read"}, OwnerUserID: "usr_gone"}, wantReason: authengine.LegacySkipOwnerMissing},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			env := newSessEnv(t)
			env.user(t, "usr_owner", "owner@example.com")
			tok := tc.tok
			tok.ID, tok.Name, tok.CreatedAt = "tok_case", "case", time.Now()
			switch tok.OwnerUserID {
			case "":
				tok.OwnerUserID = "usr_owner"
			case "-":
				tok.OwnerUserID = ""
			}
			if tok.TokenHash == "" {
				tok.TokenHash = legacyHash("raw-" + tc.name)
			}
			if err := env.db.SaveAPIToken(ctx, tok); err != nil {
				t.Fatal(err)
			}
			res, err := env.eng.ImportLegacyToken(ctx, tok)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if res.SkipReason != tc.wantReason || res.Migrated != (tc.wantErr == nil && tc.wantReason == "") {
				t.Fatalf("result = %+v, want reason %q", res, tc.wantReason)
			}
			_, lookupErr := env.eng.LookupBearer(ctx, "raw-"+tc.name)
			if res.Migrated != (lookupErr == nil) {
				t.Fatalf("library lookup err = %v after result %+v", lookupErr, res)
			}
			if _, err := env.eng.ImportLegacyToken(ctx, tok); res.Migrated && !errors.Is(err, authengine.ErrLegacyAlreadyLinked) {
				t.Fatalf("second import err = %v, want ErrLegacyAlreadyLinked", err)
			}
		})
	}
}
