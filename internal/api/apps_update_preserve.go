package api

import "github.com/GLINCKER/levelrail/internal/store"

// Carries stored fields the PUT body cannot express, since SaveDesiredService rewrites every column.
// secret_env and vault_env apply only when sent: absent keeps them, an empty value clears them.
func preserveUnsentAppFields(desired *store.DesiredService, existing store.DesiredService, req appResource, imageChanged bool) {
	desired.DatabaseEnv = existing.DatabaseEnv
	desired.Volumes = existing.Volumes
	desired.BindMounts = existing.BindMounts
	desired.Entrypoint = existing.Entrypoint
	desired.PullPolicy = existing.PullPolicy
	desired.RegistryCredentialID = existing.RegistryCredentialID
	desired.DependsOn = existing.DependsOn
	desired.Health = store.KeepComposeHealth(desired.Health, existing.Health)
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
