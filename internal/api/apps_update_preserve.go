package api

import "github.com/GLINCKER/levelrail/internal/store"

// preserveUnsentAppFields carries over stored settings the PUT body cannot
// express, because SaveDesiredService rewrites every column and would
// otherwise reset them. secret_env is the one exception: when the body
// names it, the list is authoritative, keeping each kept name's Required flag.
func preserveUnsentAppFields(desired *store.DesiredService, existing store.DesiredService, req appResource) {
	desired.VaultEnv = existing.VaultEnv
	desired.DatabaseEnv = existing.DatabaseEnv
	desired.Volumes = existing.Volumes
	desired.BindMounts = existing.BindMounts
	desired.Entrypoint = existing.Entrypoint
	desired.PullPolicy = existing.PullPolicy
	desired.RegistryCredentialID = existing.RegistryCredentialID
	if req.SecretEnv == nil {
		desired.SecretEnv = existing.SecretEnv
		return
	}
	required := make(map[string]bool, len(existing.SecretEnv))
	for _, ref := range existing.SecretEnv {
		required[ref.Name] = ref.Required
	}
	refs := store.SecretEnvRefsFromNames(unionSecretEnvNames(req.SecretEnv, nil))
	for i := range refs {
		refs[i].Required = required[refs[i].Name]
	}
	desired.SecretEnv = refs
}
