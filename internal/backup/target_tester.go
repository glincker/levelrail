package backup

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// resolve builds targetID's live Destination.
func (t *TargetTester) resolve(ctx context.Context, targetID string) (Destination, error) {
	target, err := t.Store.GetBackupTarget(ctx, targetID)
	if err != nil {
		return Destination{}, fmt.Errorf("get backup target %q: %w", targetID, err)
	}
	// Agent and local targets carry no S3 keys, so skip credential resolution.
	if strings.HasPrefix(target.Endpoint, AgentEndpointScheme) || strings.HasPrefix(target.Endpoint, LocalEndpointScheme) {
		return Destination{Provider: target.Provider, Endpoint: target.Endpoint, Bucket: target.Bucket}, nil
	}
	secretsKey := store.BackupTargetSecretsKey(targetID)
	accessKeyID, err := t.Secrets.Resolve(ctx, secretsKey, "access_key_id")
	if err != nil {
		return Destination{}, fmt.Errorf("resolve access key id for target %q: %w", targetID, err)
	}
	secretAccessKey, err := t.Secrets.Resolve(ctx, secretsKey, "secret_access_key")
	if err != nil {
		return Destination{}, fmt.Errorf("resolve secret access key for target %q: %w", targetID, err)
	}
	return withWebDAVExtras(ctx, t.Secrets, secretsKey, Destination{
		Provider:        target.Provider,
		Endpoint:        target.Endpoint,
		Region:          target.Region,
		Bucket:          target.Bucket,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}), nil
}

// TestTarget probes targetID's configured destination over its stored credentials.
func (t *TargetTester) TestTarget(ctx context.Context, targetID string) error {
	dest, err := t.resolve(ctx, targetID)
	if err != nil {
		return err
	}
	if err := t.Tester.Test(ctx, dest); err != nil {
		return fmt.Errorf("test backup target %q: %w", targetID, err)
	}
	return nil
}

// ProbeTarget is TestTarget plus the measured latency and free space when known.
func (t *TargetTester) ProbeTarget(ctx context.Context, targetID string) (TestReport, error) {
	dest, err := t.resolve(ctx, targetID)
	if err != nil {
		return TestReport{}, err
	}
	if p, ok := t.Tester.(Prober); ok {
		rep, err := p.Probe(ctx, dest)
		if err != nil {
			return TestReport{}, fmt.Errorf("test backup target %q: %w", targetID, err)
		}
		return rep, nil
	}
	start := time.Now()
	if err := t.Tester.Test(ctx, dest); err != nil {
		return TestReport{}, fmt.Errorf("test backup target %q: %w", targetID, err)
	}
	return TestReport{OK: true, LatencyMs: time.Since(start).Milliseconds()}, nil
}
