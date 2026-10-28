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

// persistFileSecrets stores each secret file's content encrypted and blanks
// it in desired state, so the database row holds only the hash.
func (p *Pipeline) persistFileSecrets(ctx context.Context, desired *store.DesiredService) error {
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
