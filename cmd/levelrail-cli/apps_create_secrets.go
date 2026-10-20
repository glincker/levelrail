package main

import (
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// checkRequiredSecrets fails when a { secret: true, required: true } var has
// no --secret or --vault-secret value: the create API keeps only secret
// names, so the first deploy would otherwise start without it.
func checkRequiredSecrets(f createFlags, key string, svc spec.Service) error {
	var missing []string
	for name, v := range svc.Env {
		if !v.Secret || !v.Required {
			continue
		}
		if _, ok := f.secrets[name]; ok {
			continue
		}
		if _, ok := f.vaultSecrets[name]; ok {
			continue
		}
		missing = append(missing, name)
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return newValidationError("service %q declares %s as { secret: true, required: true } but no value was given; pass --secret %s=VALUE (repeatable), since the first deploy starts as soon as the app is created",
		key, strings.Join(missing, ", "), missing[0])
}
