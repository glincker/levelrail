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
