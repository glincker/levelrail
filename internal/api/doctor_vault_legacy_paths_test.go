package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

func TestDoctorCheckVaultLegacyPaths(t *testing.T) {
	ctx := context.Background()
	rt, db := newTestRouter(t)
	save := func(name string, ref store.VaultEnvRef) {
		t.Helper()
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "img:v1", Port: 8080, VaultEnv: map[string]store.VaultEnvRef{"K": ref}}); err != nil {
			t.Fatal(err)
		}
	}
	if got := rt.doctorCheckVaultLegacyPaths(ctx); got.Status != doctorStatusOK {
		t.Fatalf("empty store = %+v, want ok", got)
	}

	save("inside", store.VaultEnvRef{Path: "secret/data/team-a/db", Key: "k", Unvetted: true})
	save("vetted", store.VaultEnvRef{Path: "old/path", Key: "k"})
	save("conn", store.VaultEnvRef{Conn: "main", Path: "old/path", Key: "k", Unvetted: true})
	save("outside", store.VaultEnvRef{Path: "secret/data/team-b/db", Key: "k", Unvetted: true})
	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyVaultLegacyAllowedPrefixes, json.RawMessage(`["secret/data/team-a"]`), "test"); err != nil {
		t.Fatal(err)
	}

	got := rt.doctorCheckVaultLegacyPaths(ctx)
	if got.Status != doctorStatusWarn || got.Code != "vault_legacy_paths" || !strings.Contains(got.Message, "outside") || strings.Contains(got.Message, "inside") ||
		strings.Contains(got.Message, "vetted") || strings.Contains(got.Message, "conn") || got.Fix == "" {
		t.Fatalf("check = %+v, want a warning naming only the outside app with a fix", got)
	}
}
