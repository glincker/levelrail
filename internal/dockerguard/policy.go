package dockerguard

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/bindmount"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// GrantableCaps are capabilities a container may add only when its own
// create was declared with them: the egress sidecar's firewall needs these.
var GrantableCaps = []string{"NET_ADMIN", "NET_RAW"}

// Policy is the fixed part of what the body validator allows.
type Policy struct {
	// AllowedCaps may appear in CapAdd on any container: the hardening
	// profile's minimal set plus operator extras.
	AllowedCaps []string
	// SensitivePaths are host paths no bind may cover, on top of
	// internal/bindmount.ForbiddenPaths.
	SensitivePaths []string
	// AllowHostNetwork lets a create declared with host networking through.
	AllowHostNetwork bool
	// ResolveSymlinks checks a bind's resolved host path too. Off when this
	// process cannot see the host filesystem (the agent in a container).
	ResolveSymlinks bool
}

// PolicyFromHardening builds the default policy from the hardening profile
// and the data directories this process owns.
func PolicyFromHardening(h docker.HardeningConfig, t Tunables, dataDirs ...string) Policy {
	p := Policy{AllowHostNetwork: t.AllowHostNetwork, ResolveSymlinks: true}
	p.AllowedCaps = append(append(p.AllowedCaps, docker.MinimalCapabilities...), h.ExtraCaps...)
	for _, d := range dataDirs {
		if d == "" {
			continue
		}
		if abs, err := filepath.Abs(d); err == nil {
			p.SensitivePaths = append(p.SensitivePaths, abs)
		}
	}
	return p
}

// Grants tracks what each in-flight container create was declared with by
// internal/docker's Create. A declaration lives only for the duration of
// that call, so a request that did not come from Create carries no grant.
type Grants struct {
	mu sync.Mutex
	m  map[string]docker.CreateDeclaration
}

// NewGrants returns an empty registry.
func NewGrants() *Grants {
	return &Grants{m: map[string]docker.CreateDeclaration{}}
}

// DeclareCreate implements docker.CreateDeclarer.
func (g *Grants) DeclareCreate(d docker.CreateDeclaration) func() {
	name := normalizeName(d.Name)
	if name == "" {
		return func() {}
	}
	g.mu.Lock()
	g.m[name] = d
	g.mu.Unlock()
	return func() {
		g.mu.Lock()
		delete(g.m, name)
		g.mu.Unlock()
	}
}

func (g *Grants) lookup(name string) docker.CreateDeclaration {
	name = normalizeName(name)
	if g == nil || name == "" {
		return docker.CreateDeclaration{}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.m[name]
}

func normalizeName(n string) string {
	return strings.TrimPrefix(strings.TrimSpace(n), "/")
}

func normalizeCap(c string) string {
	return strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(c)), "CAP_")
}

// sensitiveHostPath reports whether hostPath, or what it resolves to, is
// or contains a protected path (also compared in resolved form, since /etc
// may itself be a symlink).
func (p Policy) sensitiveHostPath(hostPath string) bool {
	candidates := []string{filepath.Clean(hostPath)}
	protected := append(append([]string(nil), bindmount.ForbiddenPaths...), p.SensitivePaths...)
	if p.ResolveSymlinks {
		if resolved, err := filepath.EvalSymlinks(hostPath); err == nil {
			candidates = append(candidates, resolved)
		}
		for _, f := range protected {
			if resolved, err := filepath.EvalSymlinks(f); err == nil && resolved != f {
				protected = append(protected, resolved)
			}
		}
	}
	for _, c := range candidates {
		for _, f := range protected {
			if coversOrWithin(c, f) {
				return true
			}
		}
	}
	return false
}

// coversOrWithin: c is f, lies under f, or is an ancestor that exposes f.
func coversOrWithin(c, f string) bool {
	if c == f {
		return true
	}
	if f == "/" {
		return false
	}
	return strings.HasPrefix(c, f+"/") || strings.HasPrefix(f, strings.TrimSuffix(c, "/")+"/")
}
