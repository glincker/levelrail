package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/glincker/levelrail/internal/build"
)

// reuseSource reads the live build.reuse_unchanged setting, nil means on.
var reuseSource atomic.Pointer[func() bool]

// SetBuildReuseSource points build reuse at the runtime setting.
func SetBuildReuseSource(f func() bool) { reuseSource.Store(&f) }

func buildReuseEnabled() bool {
	if f := reuseSource.Load(); f != nil && *f != nil {
		return (*f)()
	}
	return true
}

type reusedBuild struct{ settings, imageID string }

// reuseLedger remembers the settings hash and image ID of the last local build per tag.
// It is process local: after a restart the first deploy of a commit builds once.
type reuseLedger struct {
	mu   sync.Mutex
	seen map[string]reusedBuild
}

func (l *reuseLedger) get(tag string) (reusedBuild, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.seen[tag]
	return b, ok
}

func (l *reuseLedger) put(tag string, b reusedBuild) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil {
		l.seen = map[string]reusedBuild{}
	}
	l.seen[tag] = b
}

// buildSettingsHash fingerprints everything besides the commit that shapes the image.
func buildSettingsHash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sortedPairs(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + m[k] + "\n")
	}
	return b.String()
}

// reusableBuild returns the earlier result for tag when the commit and build settings are unchanged
// and the image is still on this node. Attested and secret-mounting builds always rebuild.
func (p *Pipeline) reusableBuild(ctx context.Context, tag, settings string, noCache, mounts bool) (*build.Result, bool) {
	if noCache || mounts || p.attest || p.inspector == nil || !buildReuseEnabled() {
		return nil, false
	}
	rec, ok := p.reuse.get(tag)
	if !ok || rec.settings != settings {
		return nil, false
	}
	id, err := p.inspector.InspectImageID(ctx, tag)
	if err != nil || id == "" || id != rec.imageID {
		return nil, false
	}
	return &build.Result{Tag: tag}, true
}

// rememberBuild records a finished local build so an identical redeploy can skip it.
func (p *Pipeline) rememberBuild(ctx context.Context, res *build.Result, settings string) {
	if res == nil || res.BuilderNodeID != "" || p.inspector == nil {
		return
	}
	if id, err := p.inspector.InspectImageID(ctx, res.Tag); err == nil && id != "" {
		p.reuse.put(res.Tag, reusedBuild{settings: settings, imageID: id})
	}
}
