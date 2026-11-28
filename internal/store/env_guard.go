package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrEnvKeyInvalid is returned when an env writer is given an unusable key name.
var ErrEnvKeyInvalid = errors.New("store: env key is not usable")

// checkEnvKeys is the lowest-layer shape guard every env writer calls, so a
// future writer that forgets the API checks still cannot store an empty key or
// one with = or NUL. It names the key and never the value.
func checkEnvKeys(keys ...string) error {
	for _, k := range keys {
		if k == "" || strings.ContainsAny(k, "=\x00") {
			return fmt.Errorf("%w: %q must be non-empty and contain no = or NUL", ErrEnvKeyInvalid, k)
		}
	}
	return nil
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// checkServiceEnvSave guards a full-record save: key shape for every plain and
// secret key, and no key that the save newly puts in both homes. An overlap the
// stored row already had is left alone so it never blocks an unrelated save.
func checkServiceEnvSave(ctx context.Context, tx *sql.Tx, svc DesiredService) error {
	if err := checkEnvKeys(mapKeys(svc.Env)...); err != nil {
		return err
	}
	if err := checkEnvKeys(SecretEnvNames(svc.SecretEnv)...); err != nil {
		return err
	}
	if len(svc.Env) == 0 || len(svc.SecretEnv) == 0 {
		return nil
	}
	var envRaw, secretRaw string
	err := tx.QueryRowContext(ctx, `SELECT env, secret_env FROM desired_services WHERE name = ?`, svc.Name).Scan(&envRaw, &secretRaw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: read stored env for %q: %w", svc.Name, err)
	}
	oldEnv := map[string]string{}
	var oldRefs []SecretEnvRef
	if err == nil {
		if envRaw != "" {
			_ = json.Unmarshal([]byte(envRaw), &oldEnv)
		}
		if secretRaw != "" {
			_ = json.Unmarshal([]byte(secretRaw), &oldRefs)
		}
	}
	oldNames := SecretEnvNames(oldRefs)
	for _, k := range SecretEnvNames(svc.SecretEnv) {
		if _, plain := svc.Env[k]; !plain {
			continue
		}
		_, hadPlain := oldEnv[k]
		if hadPlain && slices.Contains(oldNames, k) {
			continue
		}
		return fmt.Errorf("%w: %s", ErrEnvDualHome, k)
	}
	return nil
}
