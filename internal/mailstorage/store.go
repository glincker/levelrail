package mailstorage

import (
	"context"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore"
)

// TimeoutEnv bounds each storage operation, in seconds.
const TimeoutEnv = "APP_MAIL_STORAGE_TIMEOUT_SECONDS"

func opTimeout() time.Duration {
	if n, err := strconv.Atoi(os.Getenv(TimeoutEnv)); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 15 * time.Second
}

// Store is a bucket client that refuses every key outside its service prefix.
type Store struct {
	serviceID string
	c         *objectstore.Client
}

// Open resolves the sealed secret and returns a prefix-guarded store.
func Open(ctx context.Context, conn Connection) (*Store, error) {
	cfg, err := conn.config(ctx)
	if err != nil {
		return nil, err
	}
	c, err := objectstore.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Store{serviceID: conn.ServiceID, c: c}, nil
}

// Put writes an object under the service prefix.
func (s *Store) Put(ctx context.Context, key string, body io.ReadSeeker) error {
	if err := Guard(s.serviceID, key); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout())
	defer cancel()
	return s.c.Put(ctx, key, body, "application/octet-stream", "")
}

// Get opens an object; the caller closes it and owns its own timeout.
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	if err := Guard(s.serviceID, key); err != nil {
		return nil, 0, err
	}
	return s.c.Get(ctx, key)
}

// Delete removes an object under the service prefix.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := Guard(s.serviceID, key); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout())
	defer cancel()
	return s.c.Delete(ctx, key)
}

// List lists keys under prefix, which must be inside the service prefix.
func (s *Store) List(ctx context.Context, prefix, token string, limit int32) (objectstore.ListPage, error) {
	if err := guardPrefix(s.serviceID, prefix); err != nil {
		return objectstore.ListPage{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout())
	defer cancel()
	return s.c.List(ctx, prefix, token, limit)
}
