package backup

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TargetTestStore is the store surface TargetTester needs, narrowed the
// same way HistoryStore/VerificationHistoryStore already are: only the
// one lookup a connection test actually requires.
type TargetTestStore interface {
	GetBackupTarget(ctx context.Context, id string) (store.BackupTarget, error)
}

// TargetTester resolves targetID's stored fields plus its live
// credentials from internal/secrets and runs Tester.Test against them,
// the "verify a stored credential by actually using it, once, on
// demand" shape this codebase's own RegistryAuthTester established first
// for registry credentials.
type TargetTester struct {
	Store   TargetTestStore
	Secrets SecretsResolver
	Tester  Tester
}

// TestTarget probes targetID's configured bucket over its stored
// credentials, without uploading or deleting anything.
func (t *TargetTester) TestTarget(ctx context.Context, targetID string) error {
	target, err := t.Store.GetBackupTarget(ctx, targetID)
	if err != nil {
		return fmt.Errorf("get backup target %q: %w", targetID, err)
	}

	secretsKey := store.BackupTargetSecretsKey(targetID)
	accessKeyID, err := t.Secrets.Resolve(ctx, secretsKey, "access_key_id")
	if err != nil {
		return fmt.Errorf("resolve access key id for target %q: %w", targetID, err)
	}
	secretAccessKey, err := t.Secrets.Resolve(ctx, secretsKey, "secret_access_key")
	if err != nil {
		return fmt.Errorf("resolve secret access key for target %q: %w", targetID, err)
	}

	dest := Destination{
		Provider:        target.Provider,
		Endpoint:        target.Endpoint,
		Region:          target.Region,
		Bucket:          target.Bucket,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}

	if err := t.Tester.Test(ctx, dest); err != nil {
		return fmt.Errorf("test backup target %q: %w", targetID, err)
	}
	return nil
}
