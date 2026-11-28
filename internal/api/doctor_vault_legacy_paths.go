package api

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/vault"
)

// doctorCheckVaultLegacyPaths lists apps with an unvetted legacy Vault path outside
// vault.legacy_allowed_prefixes, which the reconciler refuses (read only).
func (rt *Router) doctorCheckVaultLegacyPaths(ctx context.Context) doctorCheckResource {
	const code, name = "vault_legacy_paths", "Legacy Vault paths"
	if rt.apps == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "Apps could not be read on this store."}
	}
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("Apps could not be read: %s", err)}
	}
	var prefixes []string
	if rt.runtimeSettings != nil {
		prefixes = rt.runtimeSettings.List(ctx, runtimesettings.KeyVaultLegacyAllowedPrefixes)
	}
	var bad []string
	for _, svc := range svcs {
		for _, ref := range svc.VaultEnv {
			if ref.Conn == "" && ref.Unvetted && !vault.LegacyPathAllowed(ref.Path, prefixes) {
				bad = append(bad, svc.Name)
				break
			}
		}
	}
	if len(bad) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "No app uses a legacy Vault path outside the allowed prefixes."}
	}
	slices.Sort(bad)
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message: fmt.Sprintf("%d app(s) use a legacy Vault path outside vault.legacy_allowed_prefixes and will not start: %s", len(bad), strings.Join(bad, ", ")),
		Fix:     "Set the allowed prefix in vault.legacy_allowed_prefixes, or migrate the variable to a secret connection.",
	}
}
