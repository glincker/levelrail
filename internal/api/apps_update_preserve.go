package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// decodeAppUpdate decodes a PUT body into req and reports which top-level
// keys it carried, so an omitted field keeps its stored value while an
// explicit empty value or null clears it.
func decodeAppUpdate(w http.ResponseWriter, r *http.Request, req *appResource) (map[string]bool, bool) {
	var raw json.RawMessage
	if !decodeReq(w, r, &raw) {
		return nil, false
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) != nil || json.Unmarshal(raw, req) != nil {
		writeError(w, http.StatusBadRequest, msgInvalidRequestBody)
		return nil, false
	}
	sent := make(map[string]bool, len(keys))
	for k := range keys {
		// encoding/json matches field names case-insensitively, so presence must too.
		sent[strings.ToLower(k)] = true
	}
	return sent, true
}

// mergeOmittedAppFields fills every settable field the body left out with
// the stored value, the one place PUT's omitted-versus-empty rule lives.
func mergeOmittedAppFields(req *appResource, existing store.DesiredService, sent map[string]bool) {
	keep := func(key string, apply func()) {
		if !sent[key] {
			apply()
		}
	}
	keep("image", func() { req.Image = existing.Image })
	keep("port", func() { req.Port = existing.Port })
	keep("host_port", func() { req.HostPort = existing.HostPort })
	keep("published_ports", func() { req.PublishedPorts = existing.PublishedPorts })
	keep("bind_address", func() { req.BindAddress = existing.BindAddress })
	keep("domains", func() { req.Domains = existing.Domains })
	keep("env", func() { req.Env = existing.Env })
	keep("resources", func() { req.Resources = existing.Resources })
	keep("health", func() { req.Health = existing.Health })
	keep("hooks", func() { req.Hooks = existing.Hooks })
	keep("strategy", func() { req.Strategy = existing.Strategy })
	keep("replicas", func() { req.Replicas = existing.Replicas })
	keep("labels", func() { req.Labels = existing.Labels })
	keep("command", func() { req.Command = existing.Command })
}

// Carries stored fields the PUT body cannot express, since SaveDesiredService rewrites every column.
// secret_env and vault_env clear only on [] or {}: absent or null keeps them, so no client drops a secret binding by accident.
func preserveUnsentAppFields(desired *store.DesiredService, existing store.DesiredService, req appResource, imageChanged bool) {
	desired.DatabaseEnv = existing.DatabaseEnv
	desired.TemplateID = existing.TemplateID
	desired.Volumes = existing.Volumes
	desired.BindMounts = existing.BindMounts
	desired.Entrypoint = existing.Entrypoint
	desired.PullPolicy = existing.PullPolicy
	desired.RegistryCredentialID = existing.RegistryCredentialID
	desired.DependsOn = existing.DependsOn
	desired.DependsOnHealthy = existing.DependsOnHealthy
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
