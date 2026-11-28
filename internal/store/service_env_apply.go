package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
)

// ErrEnvVersionConflict is returned when expected_version no longer matches the app.
var ErrEnvVersionConflict = errors.New("store: env version conflict")

// ErrEnvLockedSecret is returned when a plan touches a locked secret without overwrite_locked.
var ErrEnvLockedSecret = errors.New("store: env secret is locked")

// ErrEnvDualHome is returned when a plan would leave one key with two homes.
var ErrEnvDualHome = errors.New("store: env key would have two homes")

// EnvVersionConflict carries the version a stale writer lost to.
type EnvVersionConflict struct{ Current int }

func (e *EnvVersionConflict) Error() string {
	return fmt.Sprintf("store: env version conflict: current version is %d", e.Current)
}

// Is makes errors.Is match ErrEnvVersionConflict.
func (e *EnvVersionConflict) Is(target error) bool { return target == ErrEnvVersionConflict }

// EnvPlan is one all-or-nothing env change. Secret values arrive already
// sealed, so plaintext never reaches the store.
type EnvPlan struct {
	ExpectedVersion *int
	Set             map[string]string
	Unset           []string
	SecretSet       map[string][]byte
	SecretDelete    []string
	// Convert keys move from plain to secret; their sealed value is in SecretSet.
	Convert         []string
	VaultSet        map[string]VaultEnvRef
	VaultClear      []string
	OverwriteLocked bool
	DryRun          bool
}

// EnvPlanChange is one planned change, names only.
type EnvPlanChange struct {
	Key    string `json:"key"`
	Action string `json:"action"`
}

// EnvApplyResult reports the plan and the version after it; a dry run changes nothing.
type EnvApplyResult struct {
	Changes  []EnvPlanChange
	Version  int
	Changed  bool
	EnvDirty bool
}

// envApplyFault lets tests fail a named stage of the transaction.
var envApplyFault func(stage string) error

func envFault(stage string) error {
	if envApplyFault == nil {
		return nil
	}
	return envApplyFault(stage)
}

// EnvVersion returns the current env version of an app.
func (db *DB) EnvVersion(ctx context.Context, name string) (int, error) {
	var v int
	err := db.QueryRowContext(ctx, `SELECT env_version FROM desired_services WHERE name = ?`, name).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrServiceNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("store: read env version for %q: %w", name, err)
	}
	return v, nil
}

// ApplyEnvPlan applies plan in one BEGIN IMMEDIATE transaction: plain env,
// secret declarations, ciphertext rows and vault references move together
// or not at all, and the locked-secret check runs inside that transaction.
func (db *DB) ApplyEnvPlan(ctx context.Context, name string, plan EnvPlan) (EnvApplyResult, error) {
	if err := checkEnvKeys(slices.Concat(mapKeys(plan.Set), mapKeys(plan.SecretSet), mapKeys(plan.VaultSet), plan.Unset, plan.SecretDelete, plan.Convert)...); err != nil {
		return EnvApplyResult{}, err
	}
	res, err := db.applyEnvPlanTx(ctx, name, plan)
	if err != nil {
		return res, err
	}
	if res.Changed && !plan.DryRun {
		if err := db.noteAppChange(ctx, name); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (db *DB) applyEnvPlanTx(ctx context.Context, name string, plan EnvPlan) (EnvApplyResult, error) {
	wrap := func(step string, err error) error {
		return fmt.Errorf("store: apply env plan for %q: %s: %w", name, step, err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return EnvApplyResult{}, wrap("conn", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return EnvApplyResult{}, wrap("begin", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `ROLLBACK`)
		}
	}()

	var envRaw, secretRaw, vaultRaw string
	var version int
	var dirty bool
	err = conn.QueryRowContext(ctx, `SELECT env, secret_env, vault_env, env_version, env_dirty FROM desired_services WHERE name = ?`, name).
		Scan(&envRaw, &secretRaw, &vaultRaw, &version, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return EnvApplyResult{}, ErrServiceNotFound
	}
	if err != nil {
		return EnvApplyResult{}, wrap("read", err)
	}
	if plan.ExpectedVersion != nil && *plan.ExpectedVersion != version {
		return EnvApplyResult{Version: version}, &EnvVersionConflict{Current: version}
	}
	var env map[string]string
	if err := decodeJSONCol(envRaw, &env); err != nil {
		return EnvApplyResult{}, wrap("decode env", err)
	}
	refs, err := unmarshalSecretEnv(secretRaw)
	if err != nil {
		return EnvApplyResult{}, wrap("decode secret_env", err)
	}
	vault := map[string]VaultEnvRef{}
	if vaultRaw != "" {
		if err := json.Unmarshal([]byte(vaultRaw), &vault); err != nil {
			return EnvApplyResult{}, wrap("decode vault_env", err)
		}
	}

	nextEnv := maps.Clone(nonNilMap(env))
	nextVault := maps.Clone(vault)
	declared := map[string]SecretEnvRef{}
	var order []string
	for _, r := range refs {
		declared[r.Name] = r
		order = append(order, r.Name)
	}
	var changes []EnvPlanChange
	add := func(k, action string) { changes = append(changes, EnvPlanChange{Key: k, Action: action}) }

	for _, k := range plan.Unset {
		if _, ok := nextEnv[k]; ok {
			delete(nextEnv, k)
			add(k, "unset")
		}
	}
	for _, k := range plan.Convert {
		delete(nextEnv, k)
		add(k, "convert")
	}
	for k, v := range plan.Set {
		if _, isSecret := declared[k]; isSecret {
			return EnvApplyResult{}, fmt.Errorf("%w: %s", ErrEnvKeyIsSecret, k)
		}
		if old, ok := nextEnv[k]; !ok || old != v {
			add(k, "set")
		}
		nextEnv[k] = v
	}

	secretWrites := sortedKeysBytes(plan.SecretSet)
	deleteKeys := append([]string(nil), plan.SecretDelete...)
	sort.Strings(deleteKeys)
	for _, k := range append(append([]string(nil), secretWrites...), deleteKeys...) {
		exists, locked, err := txSecretLocked(ctx, conn, name, k)
		if err != nil {
			return EnvApplyResult{}, wrap("locked check", err)
		}
		if exists && locked && !plan.OverwriteLocked {
			return EnvApplyResult{}, fmt.Errorf("%w: %s", ErrEnvLockedSecret, k)
		}
	}
	for _, k := range secretWrites {
		_, plain := nextEnv[k]
		_, isVault := nextVault[k]
		if plain || isVault {
			return EnvApplyResult{}, fmt.Errorf("%w: %s", ErrEnvDualHome, k)
		}
		if _, ok := declared[k]; !ok {
			declared[k] = SecretEnvRef{Name: k}
			order = append(order, k)
		}
		add(k, "secret_set")
	}
	for _, k := range deleteKeys {
		delete(declared, k)
		add(k, "secret_delete")
	}
	for _, k := range plan.VaultClear {
		if _, ok := nextVault[k]; ok {
			delete(nextVault, k)
			add(k, "vault_clear")
		}
	}
	for k, ref := range plan.VaultSet {
		_, isSecret := declared[k]
		_, plain := nextEnv[k]
		if isSecret || plain {
			return EnvApplyResult{}, fmt.Errorf("%w: %s", ErrEnvDualHome, k)
		}
		nextVault[k] = ref
		add(k, "vault_set")
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Key < changes[j].Key })

	res := EnvApplyResult{Changes: changes, Version: version, Changed: len(changes) > 0, EnvDirty: dirty}
	if plan.DryRun || !res.Changed {
		return res, nil
	}
	if err := envFault("before-secrets"); err != nil {
		return EnvApplyResult{}, wrap("fault", err)
	}
	for _, k := range secretWrites {
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO service_secret_values (service_name, env_key, ciphertext, updated_at)
			VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
			ON CONFLICT (service_name, env_key) DO UPDATE SET ciphertext = excluded.ciphertext, updated_at = excluded.updated_at`,
			name, k, plan.SecretSet[k]); err != nil {
			return EnvApplyResult{}, wrap("save secret "+k, err)
		}
	}
	for _, k := range deleteKeys {
		if _, err := conn.ExecContext(ctx, `DELETE FROM service_secret_values WHERE service_name = ? AND env_key = ?`, name, k); err != nil {
			return EnvApplyResult{}, wrap("delete secret "+k, err)
		}
	}
	if err := envFault("before-columns"); err != nil {
		return EnvApplyResult{}, wrap("fault", err)
	}
	nextRefs := make([]SecretEnvRef, 0, len(declared))
	for _, k := range order {
		if r, ok := declared[k]; ok {
			nextRefs = append(nextRefs, r)
		}
	}
	envJSON, err := json.Marshal(nextEnv)
	if err != nil {
		return EnvApplyResult{}, wrap("marshal env", err)
	}
	secretJSON, err := json.Marshal(nonNilSecretEnv(nextRefs))
	if err != nil {
		return EnvApplyResult{}, wrap("marshal secret_env", err)
	}
	vaultJSON, err := json.Marshal(nextVault)
	if err != nil {
		return EnvApplyResult{}, wrap("marshal vault_env", err)
	}
	if _, err := conn.ExecContext(ctx, `
		UPDATE desired_services SET env = ?, secret_env = ?, vault_env = ?, env_dirty = 1, env_version = env_version + 1,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`,
		string(envJSON), string(secretJSON), string(vaultJSON), name); err != nil {
		return EnvApplyResult{}, wrap("write", err)
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return EnvApplyResult{}, wrap("commit", err)
	}
	committed = true
	res.Version = version + 1
	res.EnvDirty = true
	return res, nil
}

func txSecretLocked(ctx context.Context, conn *sql.Conn, name, key string) (exists, locked bool, err error) {
	var l int
	err = conn.QueryRowContext(ctx, `SELECT locked FROM service_secret_values WHERE service_name = ? AND env_key = ?`, name, key).Scan(&l)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, l != 0, nil
}

func sortedKeysBytes(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
