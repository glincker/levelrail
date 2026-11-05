package deploy

import (
	"context"
	"fmt"
	"path"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
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
// it in desired state, so the database row holds only the hash.
func (p *Pipeline) persistFileSecrets(ctx context.Context, desired *store.DesiredService) error {
	if err := p.dropStaleFileSecrets(ctx, desired); err != nil {
		return err
	}
	for i, f := range desired.Files {
		if !f.Secret || f.Content == "" {
			continue
		}
		setter, ok := p.secrets.(secretValueSetter)
		if !ok {
			return fmt.Errorf("file %q is secret but no secrets manager is configured", f.Path)
		}
		if err := setter.SetValue(ctx, desired.Name, store.FileSecretKey(f.Path), f.Content); err != nil {
			return fmt.Errorf("store secret file %q: %w", f.Path, err)
		}
		desired.Files[i].Content = ""
	}
	return nil
}
