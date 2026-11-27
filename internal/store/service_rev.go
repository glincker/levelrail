package store

import (
	"context"
	"errors"
	"fmt"
)

// ErrStaleDesiredService means a CheckRev save lost to a write made after the snapshot was read.
var ErrStaleDesiredService = errors.New("store: desired service changed since it was read")

// maxStaleSaveAttempts bounds the re-read loop; an internal limit, not a tunable.
const maxStaleSaveAttempts = 5

// SaveRebased saves svc with a revision check. On a stale rejection it re-reads
// the row, lets rebase re-apply the caller's own fields onto the fresh row, and
// retries, so columns the caller does not own (env above all) are never rolled back.
func SaveRebased(
	ctx context.Context,
	svc DesiredService,
	get func(ctx context.Context, name string) (*DesiredService, error),
	save func(ctx context.Context, svc DesiredService) error,
	rebase func(fresh DesiredService) DesiredService,
) (DesiredService, error) {
	for attempt := 0; ; attempt++ {
		toSave := svc
		toSave.CheckRev = true
		err := save(ctx, toSave)
		if err == nil {
			return svc, nil
		}
		if !errors.Is(err, ErrStaleDesiredService) || attempt+1 >= maxStaleSaveAttempts {
			return DesiredService{}, err
		}
		fresh, gerr := get(ctx, svc.Name)
		if gerr != nil {
			return DesiredService{}, fmt.Errorf("store: rebase desired service %q: %w", svc.Name, gerr)
		}
		svc = rebase(*fresh)
	}
}

// EnvColumnsEqual reports whether two snapshots carry the same env inputs.
func EnvColumnsEqual(a, b DesiredService) bool {
	return fmt.Sprint(a.Env) == fmt.Sprint(b.Env) &&
		fmt.Sprint(a.SecretEnv) == fmt.Sprint(b.SecretEnv) &&
		fmt.Sprint(a.DatabaseEnv) == fmt.Sprint(b.DatabaseEnv) &&
		fmt.Sprint(a.VaultEnv) == fmt.Sprint(b.VaultEnv)
}

// MergeEnvMap applies the changes mine made relative to base onto fresh, so keys
// another writer added after base survive and only mine's own edits are re-applied.
func MergeEnvMap(base, mine, fresh map[string]string) map[string]string {
	out := make(map[string]string, len(fresh)+len(mine))
	for k, v := range fresh {
		out[k] = v
	}
	for k, v := range mine {
		if old, ok := base[k]; !ok || old != v {
			out[k] = v
		}
	}
	for k := range base {
		if _, ok := mine[k]; !ok {
			delete(out, k)
		}
	}
	return out
}

// RebaseEnvColumns returns mine with its env edits (relative to base) layered onto
// fresh's env columns and fresh's revision, for writers that own env but must not
// drop what a concurrent env write added.
func RebaseEnvColumns(base, mine, fresh DesiredService) DesiredService {
	out := mine
	out.Rev = fresh.Rev
	out.Env = MergeEnvMap(base.Env, mine.Env, fresh.Env)
	if fmt.Sprint(mine.SecretEnv) == fmt.Sprint(base.SecretEnv) {
		out.SecretEnv = fresh.SecretEnv
	}
	if fmt.Sprint(mine.DatabaseEnv) == fmt.Sprint(base.DatabaseEnv) {
		out.DatabaseEnv = fresh.DatabaseEnv
	}
	if fmt.Sprint(mine.VaultEnv) == fmt.Sprint(base.VaultEnv) {
		out.VaultEnv = fresh.VaultEnv
	}
	out.EnvDirty = mine.EnvDirty || fresh.EnvDirty || !EnvColumnsEqual(base, out)
	return out
}
