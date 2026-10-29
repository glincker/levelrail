package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// resolveFiles turns the stored file list into mounts. A secret file is
// decrypted here and its ${VAR} placeholders expanded from the container's
// resolved env; the plaintext lives only until Create and is never logged.
func (c *Controller) resolveFiles(ctx context.Context, desired *store.DesiredService, env map[string]string) ([]docker.FileMount, error) {
	if len(desired.Files) == 0 {
		return nil, nil
	}
	out := make([]docker.FileMount, 0, len(desired.Files))
	for _, f := range desired.Files {
		mode, err := spec.ParseFileMode(f.Mode)
		if err != nil {
			return nil, fmt.Errorf("file %q: %w", f.Path, err)
		}
		content := f.Content
		if f.Secret {
			if c.secretResolver == nil {
				return nil, fmt.Errorf("file %q is secret but no secret resolver is configured", f.Path)
			}
			plain, err := c.secretResolver.Resolve(ctx, c.serviceName, store.FileSecretKey(f.Path))
			if errors.Is(err, secrets.ErrValueNotFound) {
				return nil, fmt.Errorf("secret file %q has no stored content, set it again", f.Path)
			}
			if err != nil {
				return nil, fmt.Errorf("resolve secret file %q: %w", f.Path, err)
			}
			if content, err = spec.ExpandFilePlaceholders(plain, env); err != nil {
				return nil, fmt.Errorf("file %q: %w", f.Path, err)
			}
		}
		out = append(out, docker.FileMount{Path: f.Path, Mode: uint32(mode), Content: []byte(content)})
	}
	return out, nil
}
