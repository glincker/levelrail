package backup

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TargetGetter is the one-method surface resolveTargetDestination
// needs, shared by BaseBackupHistoryStore and PITRHistoryStore so both
// can resolve a target's live Destination the identical way without each
// repeating the lookup-plus-two-secret-resolves sequence Runner.
// ResolveDestination (runner.go) already establishes for the logical-dump
// direction.
type TargetGetter interface {
	GetBackupTarget(ctx context.Context, id string) (store.BackupTarget, error)
}

// resolveTargetDestination resolves targetID's stored config and live
// secrets into a Destination: the base-backup and PITR-restore
// counterpart of Runner.ResolveDestination, sharing its exact logic
// instead of each of BaseBackupRunner and PITRRunner repeating it.
func resolveTargetDestination(ctx context.Context, targets TargetGetter, secrets SecretsResolver, targetID string) (Destination, error) {
	target, err := targets.GetBackupTarget(ctx, targetID)
	if err != nil {
		return Destination{}, fmt.Errorf("get backup target %q: %w", targetID, err)
	}

	secretsKey := store.BackupTargetSecretsKey(targetID)
	accessKeyID, err := secrets.Resolve(ctx, secretsKey, "access_key_id")
	if err != nil {
		return Destination{}, fmt.Errorf("resolve access key id for target %q: %w", targetID, err)
	}
	secretAccessKey, err := secrets.Resolve(ctx, secretsKey, "secret_access_key")
	if err != nil {
		return Destination{}, fmt.Errorf("resolve secret access key for target %q: %w", targetID, err)
	}

	return Destination{
		Provider:        target.Provider,
		Endpoint:        target.Endpoint,
		Region:          target.Region,
		Bucket:          target.Bucket,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}, nil
}
