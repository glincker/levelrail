package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"sort"
)

// ServiceFile is one inline file written into a service's container before
// it starts. A secret file's content lives in the secrets store under
// FileSecretKey(Path), so Content is empty for it and Hash alone tells a
// change apart without exposing the text.
type ServiceFile struct {
	Path    string `json:"path"`
	Mode    string `json:"mode,omitempty"`
	Secret  bool   `json:"secret,omitempty"`
	Content string `json:"content,omitempty"`
	Hash    string `json:"hash,omitempty"`
}

// AppliedFieldFiles hashes the file list, which is fixed at container create.
const AppliedFieldFiles = "files"

// secretFileKeyPrefix marks secrets-store slots that hold file content, so
// the env secret listing can leave them out.
const secretFileKeyPrefix = "__file:"

// FileSecretKey is the secrets-store key holding a secret file's content.
func FileSecretKey(filePath string) string {
	return secretFileKeyPrefix + path.Clean(filePath)
}

// IsFileSecretKey reports whether key names a secret file slot.
func IsFileSecretKey(key string) bool {
	return len(key) > len(secretFileKeyPrefix) && key[:len(secretFileKeyPrefix)] == secretFileKeyPrefix
}

// FileContentHash is a short service and path scoped digest of a file's
// content: enough to notice a change, not to recover the text.
func FileContentHash(serviceName, filePath, content string) string {
	sum := sha256.Sum256([]byte(serviceName + "\x00" + path.Clean(filePath) + "\x00" + content))
	return hex.EncodeToString(sum[:])[:appliedHashLen]
}

func nonNilFiles(f []ServiceFile) []ServiceFile {
	if f == nil {
		return []ServiceFile{}
	}
	return f
}

// filesAppliedHash folds path, mode and content hash of every file, in path
// order, so a reorder alone is not a change.
func filesAppliedHash(files []ServiceFile) string {
	if len(files) == 0 {
		return ""
	}
	type entry struct{ P, M, H string }
	out := make([]entry, 0, len(files))
	for _, f := range files {
		h := f.Hash
		if h == "" {
			h = FileContentHash("", f.Path, f.Content)
		}
		out = append(out, entry{path.Clean(f.Path), f.Mode, h})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].P < out[j].P })
	return shortJSONHash(out)
}

// UpdateServiceFiles replaces svc's file list as a whole without
// SaveDesiredService's full-record replace.
func (db *DB) UpdateServiceFiles(ctx context.Context, name string, files []ServiceFile) error {
	return db.updateServiceJSONColumn(ctx, name, "files", `
		UPDATE desired_services SET files = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, nonNilFiles(files))
}
