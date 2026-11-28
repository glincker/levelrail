package api

import (
	"strings"

	"github.com/GLINCKER/levelrail/internal/envredact"
)

// envApplyRequest is the body of POST /api/v1/apps/{name}/env/apply.
type envApplyRequest struct {
	ExpectedVersion *int                             `json:"expected_version"`
	Set             map[string]string                `json:"set"`
	Unset           []string                         `json:"unset"`
	SecretSet       map[string]string                `json:"secret_set"`
	SecretDelete    []string                         `json:"secret_delete"`
	Convert         []string                         `json:"convert"`
	VaultSet        map[string]setAppVaultEnvRequest `json:"vault_set"`
	VaultClear      []string                         `json:"vault_clear"`
	OverwriteLocked bool                             `json:"overwrite_locked"`
	DryRun          bool                             `json:"dry_run"`
}

// touchesSensitive is true when the plan writes secrets, conversions or vault references.
func (p envApplyRequest) touchesSensitive() bool {
	return len(p.SecretSet) > 0 || len(p.SecretDelete) > 0 || len(p.Convert) > 0 || len(p.VaultSet) > 0 || len(p.VaultClear) > 0
}

func (p envApplyRequest) empty() bool {
	return len(p.Set) == 0 && len(p.Unset) == 0 && !p.touchesSensitive()
}

func validEnvKey(k string) bool {
	return k != "" && !strings.ContainsAny(k, "=\x00")
}

// validateEnvPlan applies the same key and value rules as the single-key
// endpoints and refuses a plan that gives one key two homes. It returns an
// empty string when the plan is acceptable, else the reason.
func validateEnvPlan(p envApplyRequest) string {
	if p.empty() {
		return "send at least one change"
	}
	home := map[string]string{}
	claim := func(k, kind string) string {
		if !validEnvKey(k) {
			return "env keys must be non-empty and contain no = or NUL"
		}
		if prev, dup := home[k]; dup {
			return "key " + k + " appears twice in the plan (" + prev + " and " + kind + ")"
		}
		home[k] = kind
		return ""
	}
	for k, v := range p.Set {
		if msg := claim(k, "set"); msg != "" {
			return msg
		}
		if envredact.IsPlaceholder(v) {
			return "a masked placeholder is not a value; send the real value or leave the key out"
		}
	}
	for k, v := range p.SecretSet {
		if msg := claim(k, "secret_set"); msg != "" {
			return msg
		}
		if v == "" || envredact.IsPlaceholder(v) {
			return "a secret needs a real, non-empty value"
		}
	}
	for k := range p.VaultSet {
		if msg := claim(k, "vault_set"); msg != "" {
			return msg
		}
	}
	for _, group := range []struct {
		keys []string
		kind string
	}{{p.Unset, "unset"}, {p.SecretDelete, "secret_delete"}, {p.Convert, "convert"}, {p.VaultClear, "vault_clear"}} {
		for _, k := range group.keys {
			if msg := claim(k, group.kind); msg != "" {
				return msg
			}
		}
	}
	if len(p.Convert) > 0 && p.ExpectedVersion == nil {
		return "convert needs expected_version so the plain value read matches what is stored"
	}
	return ""
}
