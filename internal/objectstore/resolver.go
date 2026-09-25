package objectstore

import (
	"context"
	"fmt"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TargetStore is the storage-destination lookup surface the resolver needs.
type TargetStore interface {
	GetBackupTarget(ctx context.Context, id string) (store.BackupTarget, error)
	GetStorageOptions(ctx context.Context, targetID string) (store.StorageOptions, bool, error)
}

// SecretsResolver reads a stored credential.
type SecretsResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// Resolver builds a Client for a stored storage destination.
type Resolver struct {
	Store   TargetStore
	Secrets SecretsResolver
	// HTTPClient overrides the SSRF-guarded default, for tests.
	HTTPClient *http.Client
	// MaxAttempts bounds SDK retries per request; zero means the default.
	MaxAttempts int
}

// Client resolves targetID's settings and credentials into a Client.
func (r *Resolver) Client(ctx context.Context, targetID string) (*Client, store.BackupTarget, error) {
	target, err := r.Store.GetBackupTarget(ctx, targetID)
	if err != nil {
		return nil, store.BackupTarget{}, fmt.Errorf("objectstore: get destination %q: %w", targetID, err)
	}
	opts, ok, err := r.Store.GetStorageOptions(ctx, targetID)
	if err != nil {
		return nil, store.BackupTarget{}, fmt.Errorf("objectstore: %w", err)
	}
	pathStyle := target.Endpoint != ""
	if ok {
		pathStyle = opts.PathStyle
	}

	key := store.BackupTargetSecretsKey(targetID)
	accessKeyID, err := r.Secrets.Resolve(ctx, key, "access_key_id")
	if err != nil {
		return nil, store.BackupTarget{}, fmt.Errorf("objectstore: resolve access key id for %q: %w", targetID, err)
	}
	secret, err := r.Secrets.Resolve(ctx, key, "secret_access_key")
	if err != nil {
		return nil, store.BackupTarget{}, fmt.Errorf("objectstore: resolve secret access key for %q: %w", targetID, err)
	}

	c, err := New(Config{
		Endpoint: target.Endpoint, Region: target.Region, Bucket: target.Bucket,
		AccessKeyID: accessKeyID, SecretAccessKey: secret, PathStyle: pathStyle, HTTPClient: r.HTTPClient, MaxAttempts: r.MaxAttempts,
	})
	if err != nil {
		return nil, store.BackupTarget{}, err
	}
	return c, target, nil
}
