package build

import (
	"context"
	"sync"
	"testing"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"
	"github.com/opencontainers/go-digest"
)

// namedCgroups mirrors the Linux provisioner's parent path without touching /sys.
type namedCgroups struct {
	mu    sync.Mutex
	names []string
}

func (n *namedCgroups) Provision(name string, _, _ int) (string, func(), error) {
	n.mu.Lock()
	n.names = append(n.names, name)
	n.mu.Unlock()
	return "levelrail-builds/" + name, func() {}, nil
}

// execCacheKeys digests every exec op after clearing the fields BuildKit's
// ExecOp.CacheMap (v0.34.1) clears; CgroupParent is not one of them.
func execCacheKeys(t *testing.T, def *llb.Definition) []digest.Digest {
	t.Helper()
	var keys []digest.Digest
	for _, raw := range def.Def {
		var op pb.Op
		if err := op.UnmarshalVT(raw); err != nil {
			t.Fatal(err)
		}
		ex := op.GetExec()
		if ex == nil {
			continue
		}
		for _, h := range ex.Meta.ExtraHosts {
			h.IP = ""
		}
		for _, m := range ex.Mounts {
			m.Selector = ""
			if m.CacheOpt != nil {
				m.CacheOpt.ID, m.CacheOpt.Sharing = "", 0
			}
		}
		ex.Meta.ProxyEnv = nil
		enc, err := ex.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, digest.FromBytes(enc))
	}
	if len(keys) == 0 {
		t.Fatal("no exec ops")
	}
	return keys
}

func sampleInstallDef(t *testing.T) *llb.Definition {
	t.Helper()
	st := llb.Image("node:22-slim").
		Run(llb.Shlex("npm ci"), llb.AddMount("/root/.npm", llb.Scratch(), llb.AsPersistentCacheDir("web-npm", llb.CacheMountShared))).Root()
	st = st.Run(llb.Shlex("npm run build")).Root()
	def, err := st.Marshal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return def
}

// parentFor runs one build through the Limiter and returns the cgroup parent it handed the builder.
func parentFor(t *testing.T, l *Limiter, tag string) string {
	t.Helper()
	var got string
	l.inner = cgroupSpy{onBuild: func(r Request) { got = r.CgroupParent }}
	if _, err := l.Build(context.Background(), Request{Tag: tag}, nil); err != nil {
		t.Fatal(err)
	}
	return got
}

func keysWithParent(t *testing.T, parent string) []digest.Digest {
	t.Helper()
	def := sampleInstallDef(t)
	if parent != "" {
		var err error
		if def, err = withCgroupParent(def, parent); err != nil {
			t.Fatal(err)
		}
	}
	return execCacheKeys(t, def)
}

func sameKeys(a, b []digest.Digest) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCgroupParentKeepsExecCacheKeysStableAcrossCommits(t *testing.T) {
	l := NewLimiter(nil, fixed(Limits{MemoryMB: 2048, CPUMillis: 1500}), WithCgroupProvisioner(&namedCgroups{}))

	first := parentFor(t, l, "reg/web:1111111")
	second := parentFor(t, l, "reg/web:2222222")
	other := parentFor(t, l, "reg/api:1111111")
	if first != "levelrail-builds/web" || first != second {
		t.Fatalf("parents = %q, %q, want one stable per app name", first, second)
	}
	if !sameKeys(keysWithParent(t, first), keysWithParent(t, second)) {
		t.Fatal("exec cache keys differ between two commits of the same app")
	}
	if sameKeys(keysWithParent(t, first), keysWithParent(t, other)) {
		t.Fatal("exec cache keys of two apps with limits should differ by cgroup parent")
	}
}

func TestBuildWithoutLimitsKeepsPlainCacheKeys(t *testing.T) {
	l := NewLimiter(nil, fixed(Limits{}), WithCgroupProvisioner(&namedCgroups{}))
	if p := parentFor(t, l, "reg/web:abc"); p != "" {
		t.Fatalf("parent = %q without limits", p)
	}
	plain := execCacheKeys(t, sampleInstallDef(t))
	if !sameKeys(plain, keysWithParent(t, "")) {
		t.Fatal("cache keys changed without limits")
	}
}

func TestOverlappingSameAppBuildsGetOwnCgroup(t *testing.T) {
	cg := &namedCgroups{}
	l := NewLimiter(nil, fixed(Limits{MemoryMB: 1024}), WithCgroupProvisioner(cg))
	a, freeA := l.takeCgroupName("web")
	b, freeB := l.takeCgroupName("web")
	if a != "web" || b != "web.slot1" {
		t.Fatalf("names = %q, %q", a, b)
	}
	freeA()
	if c, freeC := l.takeCgroupName("web"); c != "web" {
		t.Fatalf("freed name not reused: %q", c)
	} else {
		freeC()
	}
	freeB()
}
