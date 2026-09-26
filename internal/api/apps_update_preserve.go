package api

import "github.com/GLINCKER/levelrail/internal/store"

// preserveUnsentAppFields carries over stored settings the PUT body cannot
// express, because SaveDesiredService rewrites every column and would
// otherwise reset them. secret_env and vault_env apply only when the body
// carries them: absent keeps the stored set, an empty list or map clears it.
func preserveUnsentAppFields(desired *store.DesiredService, existing store.DesiredService, req appResource, imageChanged bool) {
	desired.DatabaseEnv = existing.DatabaseEnv
	desired.Volumes = existing.Volumes
	desired.BindMounts = existing.BindMounts
	desired.Entrypoint = existing.Entrypoint
	desired.PullPolicy = existing.PullPolicy
	desired.RegistryCredentialID = existing.RegistryCredentialID
	if !imageChanged {
		desired.ImageID, desired.ImageIDRef = existing.ImageID, existing.ImageIDRef
	}
	desired.VaultEnv = existing.VaultEnv
	if req.VaultEnv != nil {
		desired.VaultEnv = make(map[string]store.VaultEnvRef, len(req.VaultEnv))
		for k, v := range req.VaultEnv {
			desired.VaultEnv[k] = store.VaultEnvRef{Path: v.Path, Key: v.Key}
		}
	}
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
