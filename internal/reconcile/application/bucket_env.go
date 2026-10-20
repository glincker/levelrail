package application

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// BucketBindingStore lists the buckets bound to an app. *store.DB satisfies it.
type BucketBindingStore interface {
	ListStorageBucketBindings(ctx context.Context, appName string) ([]store.StorageBucketBinding, error)
}

// BucketEnvSuffixes are the names injected per binding, each behind the binding's env prefix.
var BucketEnvSuffixes = []string{"ENDPOINT", "BUCKET", "PREFIX", "REGION", "ACCESS_KEY_ID", "SECRET_ACCESS_KEY"}

// WithBucketBindings injects the env vars of every bucket bound to the app at container create.
func WithBucketBindings(s BucketBindingStore) Option {
	return func(ctrl *Controller) { ctrl.bucketBindings = s }
}

func (c *Controller) resolveBucketEnv(ctx context.Context) (map[string]string, error) {
	bindings, err := c.bucketBindings.ListStorageBucketBindings(ctx, c.serviceName)
	if err != nil {
		return nil, fmt.Errorf("list bucket bindings: %w", err)
	}
	env := map[string]string{}
	for _, b := range bindings {
		target, err := c.resolveStorageEnv(ctx, b.Bucket.TargetID)
		if err != nil {
			return nil, fmt.Errorf("bucket %q: %w", b.Bucket.Name, err)
		}
		p := b.EnvPrefix + "_"
		env[p+"BUCKET"] = target[envKeyS3Bucket]
		env[p+"PREFIX"] = b.Bucket.Prefix
		env[p+"ACCESS_KEY_ID"] = target[envKeyS3AccessKey]
		env[p+"SECRET_ACCESS_KEY"] = target[envKeyS3SecretKey]
		if v, ok := target[envKeyS3Endpoint]; ok {
			env[p+"ENDPOINT"] = v
		}
		if v, ok := target[envKeyS3Region]; ok {
			env[p+"REGION"] = v
		}
	}
	return env, nil
}
