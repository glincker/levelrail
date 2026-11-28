package deploy

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/glincker/levelrail/internal/spec"
	"github.com/glincker/levelrail/internal/store"
)

// secretValueSetter is what the secrets manager adds beside SecretChecker;
// secret files need it to move their content out of desired state.
type secretValueSetter interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
}

// toServiceFiles is never nil: an app.yaml without files: clears the list,
// the same declarative replace the other spec blocks follow. A secret
// file keeps its content here only until persistFileSecrets runs.
func toServiceFiles(name string, files []spec.File) []store.ServiceFile {
	out := make([]store.ServiceFile, 0, len(files))
	for _, f := range files {
		out = append(out, store.ServiceFile{
			Path: path.Clean(f.Path), Mode: f.Mode, Secret: f.Secret,
			Content: f.Content, Hash: store.FileContentHash(name, f.Path, f.Content),
		})
	}
	return out
}

// secretValueDeleter removes one stored secret value; optional like the setter.
type secretValueDeleter interface {
	DeleteValue(ctx context.Context, serviceName, envKey string) error
}

// desiredLoader reads the previously saved record, to find dropped files.
type desiredLoader interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
}

// dropStaleFileSecrets deletes the encrypted value of every secret file the
// saved record had and the new one lacks, so no orphan row survives and a
// later spec reusing the path must supply the content again. Snapshots hold
// only hashes, so no earlier release depends on the value.
func (p *Pipeline) dropStaleFileSecrets(ctx context.Context, desired *store.DesiredService) error {
	loader, lok := p.store.(desiredLoader)
	deleter, dok := p.secrets.(secretValueDeleter)
	if !lok || !dok {
		return nil
	}
	prev, err := loader.GetDesiredService(ctx, desired.Name)
	if err != nil || prev == nil {
		return nil //nolint:nilerr // a first deploy has no previous record
	}
	keep := make(map[string]bool, len(desired.Files))
	for _, f := range desired.Files {
		if f.Secret {
			keep[path.Clean(f.Path)] = true
		}
	}
	for _, f := range prev.Files {
		if !f.Secret || keep[path.Clean(f.Path)] {
			continue
		}
		if err := deleter.DeleteValue(ctx, desired.Name, store.FileSecretKey(f.Path)); err != nil {
			return fmt.Errorf("remove secret file %q: %w", f.Path, err)
		}
	}
	return nil
}

// persistFileSecrets stores each secret file's content encrypted and blanks
// it in desired state, so the database row holds only the hash. It returns the
// slots it wrote, and admits the name first so a refused name leaves nothing.
func (p *Pipeline) persistFileSecrets(ctx context.Context, desired *store.DesiredService) ([]string, error) {
	if err := p.dropStaleFileSecrets(ctx, desired); err != nil {
		return nil, err
	}
	var written []string
	admitted := false
	for i, f := range desired.Files {
		if !f.Secret || f.Content == "" {
			continue
		}
		setter, ok := p.secrets.(secretValueSetter)
		if !ok {
			return written, fmt.Errorf("file %q is secret but no secrets manager is configured", f.Path)
		}
		if !admitted && p.names != nil {
			if err := p.names.AdmitNewServiceName(ctx, desired.Name); err != nil {
				return written, fmt.Errorf("service name %q: %w", desired.Name, err)
			}
			admitted = true
		}
		key := store.FileSecretKey(f.Path)
		if err := setter.SetValue(ctx, desired.Name, key, f.Content); err != nil {
			p.dropUnsavedFileSecrets(ctx, desired.Name, written)
			return nil, fmt.Errorf("store secret file %q: %w", f.Path, err)
		}
		written = append(written, key)
		desired.Files[i].Content = ""
	}
	return written, nil
}

// dropUnsavedFileSecrets removes slots a failed save left behind, but only for a service
// with no saved record, so a redeploy's earlier values are never lost.
func (p *Pipeline) dropUnsavedFileSecrets(ctx context.Context, name string, keys []string) {
	deleter, dok := p.secrets.(secretValueDeleter)
	loader, lok := p.store.(desiredLoader)
	if len(keys) == 0 || !dok || !lok {
		return
	}
	if _, err := loader.GetDesiredService(ctx, name); !errors.Is(err, store.ErrServiceNotFound) {
		return
	}
	for _, k := range keys {
		if err := deleter.DeleteValue(context.WithoutCancel(ctx), name, k); err != nil {
			p.logger.Warn("deploy: drop unsaved file secret failed", "name", name, "error", err.Error())
		}
	}
}
