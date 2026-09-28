package cpbackup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const (
	listPageSize    = 1000
	maxManifestSize = 64 << 10
)

// Remote is one backup found at the destination.
type Remote struct {
	Key         string    `json:"key"`
	ManifestKey string    `json:"manifest_key"`
	CreatedAt   time.Time `json:"created_at"`
	SizeBytes   int64     `json:"size_bytes"`
	// Complete is true when both the ciphertext and its manifest exist.
	Complete bool `json:"complete"`
	// Modified is the newest object modification time, used to age out orphans.
	Modified time.Time `json:"-"`
}

func newInstallID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate install id: %w", err)
	}
	return "inst-" + hex.EncodeToString(b), nil
}

func installPrefix(installID string) string { return Prefix + "/" + installID + "/" }

// listRemotes pages through prefix and pairs ciphertexts with manifests, newest first.
func listRemotes(ctx context.Context, b Bucket, prefix string) ([]Remote, error) {
	byBase := map[string]*Remote{}
	token := ""
	for {
		page, err := b.List(ctx, prefix, token, listPageSize)
		if err != nil {
			return nil, fmt.Errorf("list backups: %w", err)
		}
		for _, o := range page.Objects {
			var base string
			isData := strings.HasSuffix(o.Key, dataSuffix)
			switch {
			case isData:
				base = strings.TrimSuffix(o.Key, dataSuffix)
			case strings.HasSuffix(o.Key, manifestSuffix):
				base = strings.TrimSuffix(o.Key, manifestSuffix)
			default:
				continue
			}
			r := byBase[base]
			if r == nil {
				r = &Remote{}
				byBase[base] = r
			}
			if isData {
				r.Key, r.SizeBytes = o.Key, o.Size
			} else {
				r.ManifestKey = o.Key
			}
			if o.LastModified.After(r.Modified) {
				r.Modified = o.LastModified
			}
		}
		if page.Next == "" {
			break
		}
		token = page.Next
	}
	out := make([]Remote, 0, len(byBase))
	for base, r := range byBase {
		key := r.Key
		if key == "" {
			key = base + dataSuffix
		}
		if _, ts, err := ParseKey(key); err == nil {
			r.CreatedAt = ts
		} else {
			continue
		}
		r.Complete = r.Key != "" && r.ManifestKey != ""
		if r.Key == "" {
			r.Key = key
		}
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// ListRemote lists the backups one install has at the destination, newest first.
func (s *Service) ListRemote(ctx context.Context) ([]Remote, error) {
	cfg, err := s.Store.GetCPDRSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	if cfg.TargetID == "" {
		return nil, ErrNotConfigured
	}
	b, _, err := s.Dest.Open(ctx, cfg.TargetID)
	if err != nil {
		return nil, fmt.Errorf("open destination: %w", err)
	}
	if cfg.InstallID == "" {
		return []Remote{}, nil
	}
	return listRemotes(ctx, b, installPrefix(cfg.InstallID))
}

// FetchManifest downloads and parses the manifest for a ciphertext key.
func FetchManifest(ctx context.Context, b Bucket, dataKey string) (Manifest, error) {
	rc, _, err := b.Get(ctx, ManifestKey(dataKey))
	if err != nil {
		return Manifest{}, fmt.Errorf("download manifest: %w", err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, maxManifestSize))
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	return ParseManifest(data)
}

// pruneRemote removes backups outside the retention policy and stale orphans,
// oldest first, at most limit objects per call. Manifest goes before its
// ciphertext so an interrupted prune leaves an orphan, never a manifest that
// points at nothing.
func pruneRemote(ctx context.Context, b Bucket, prefix string, ret Retention, limit int, orphanAge time.Duration, now time.Time) (int, error) {
	remotes, err := listRemotes(ctx, b, prefix)
	if err != nil {
		return 0, err
	}
	var entries []Entry
	for _, r := range remotes {
		if r.Complete {
			entries = append(entries, Entry{Key: r.Key, CreatedAt: r.CreatedAt})
		}
	}
	keep := ret.Select(entries)
	var doomed []Remote
	for i := len(remotes) - 1; i >= 0; i-- {
		r := remotes[i]
		switch {
		case r.Complete && !keep[r.Key]:
			doomed = append(doomed, r)
		case !r.Complete && now.Sub(r.Modified) > orphanAge:
			doomed = append(doomed, r)
		}
	}
	removed := 0
	for _, r := range doomed {
		if removed >= limit {
			break
		}
		if r.ManifestKey != "" {
			if err := b.Delete(ctx, r.ManifestKey); err != nil {
				return removed, err
			}
		}
		if r.SizeBytes > 0 || r.Complete {
			if err := b.Delete(ctx, r.Key); err != nil {
				return removed, err
			}
		}
		removed++
	}
	return removed, nil
}
