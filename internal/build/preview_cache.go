package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"sync/atomic"
)

// EnvPreviewCacheMode seeds the preview build cache mode; unset means isolated.
const EnvPreviewCacheMode = "APP_DEPLOY_PREVIEW_CACHE_MODE"

// KeyPreviewCacheMode is the runtime setting key.
const KeyPreviewCacheMode = "deploy.preview_cache_mode"

// Preview cache modes.
const (
	PreviewCacheIsolated      = "isolated"
	PreviewCacheSharedWithApp = "shared_with_app"
	PreviewCacheOff           = "off"
)

var previewCacheResolver atomic.Pointer[func(context.Context) string]

// SetPreviewCacheModeResolver wires the saved setting in at boot so changes apply live.
func SetPreviewCacheModeResolver(fn func(context.Context) string) {
	if fn == nil {
		previewCacheResolver.Store(nil)
		return
	}
	previewCacheResolver.Store(&fn)
}

// PreviewCacheMode returns the active mode; anything unknown falls back to isolated.
func PreviewCacheMode(ctx context.Context) string {
	raw := os.Getenv(EnvPreviewCacheMode)
	if fn := previewCacheResolver.Load(); fn != nil {
		raw = (*fn)(ctx)
	}
	switch m := strings.ToLower(strings.TrimSpace(raw)); m {
	case PreviewCacheSharedWithApp, PreviewCacheOff:
		return m
	}
	return PreviewCacheIsolated
}

// FitCacheID sanitizes id without losing its tail: past the max length it keeps
// a head and a hash of the whole id, so two long names never collide.
func FitCacheID(id string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(id) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == '_' || r == '.' || r == '/' || r == ' ':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) <= maxCacheIDLen {
		return out
	}
	sum := sha256.Sum256([]byte(out))
	head := strings.Trim(out[:maxCacheIDLen-9], "-")
	return head + "-" + hex.EncodeToString(sum[:4])
}

// isPreviewOf reports whether name is "<parent>-pr-<digits>" or one of its service keys.
func isPreviewOf(name, parent string) bool {
	rest, ok := strings.CutPrefix(name, parent+"-pr-")
	if !ok {
		return false
	}
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	return digits > 0 && (digits == len(rest) || rest[digits] == '-')
}

// PreviewCacheID derives the cache id of a preview build. parent and parentID
// come from the preview record: shared mode reuses parentID only when the
// preview is named after that parent, never by cutting the name. unique only
// matters for mode off, where every build gets a fresh id.
func PreviewCacheID(mode, serviceName, parent, parentID, unique string) string {
	switch mode {
	case PreviewCacheSharedWithApp:
		if parent != "" && parentID != "" && isPreviewOf(serviceName, parent) {
			return parentID
		}
	case PreviewCacheOff:
		base := FitCacheID(serviceName)
		if len(base) > maxCacheIDLen-9 {
			base = strings.Trim(base[:maxCacheIDLen-9], "-")
		}
		sum := sha256.Sum256([]byte(unique))
		return base + "-" + hex.EncodeToString(sum[:4])
	}
	return FitCacheID(serviceName)
}
