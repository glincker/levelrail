package fixit

import (
	"context"
	"errors"
	"testing"
)

func detectNextjs(t *testing.T, id string, f NextjsFacts, found bool) Applicability {
	t.Helper()
	d := NextjsDeps{Facts: func(context.Context, string) (NextjsFacts, bool, error) { return f, found, nil }}
	for _, fx := range NextjsFixes("web", d) {
		if fx.ID == id {
			a, err := fx.Detect(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			return a
		}
	}
	t.Fatalf("fix %s not found", id)
	return Applicability{}
}

func TestNextjsFixes(t *testing.T) {
	healthy := NextjsFacts{Standalone: true, StaticCopied: true, SharpAvailable: true, ActionsKeySet: true,
		MemoryMB: 512, MinMemoryMB: 256, Replicas: 1}
	cases := []struct {
		id   string
		mut  func(*NextjsFacts)
		fail bool
	}{
		{"nextjs.output.standalone", func(f *NextjsFacts) { f.Standalone = false }, true},
		{"nextjs.static.copy", func(f *NextjsFacts) { f.StaticCopied = false }, true},
		{"nextjs.sharp.available", func(f *NextjsFacts) { f.SharpAvailable = false }, true},
		{"nextjs.actions.key", func(f *NextjsFacts) { f.ActionsKeySet = false }, true},
		{"nextjs.memory.floor", func(f *NextjsFacts) { f.MemoryMB = 128 }, true},
		{"nextjs.memory.floor", func(f *NextjsFacts) { f.MemoryMB = 0 }, false},
		{"nextjs.cache.shared", func(f *NextjsFacts) { f.Replicas = 3 }, true},
		{"nextjs.cache.shared", func(f *NextjsFacts) { f.Replicas = 3; f.SharedCache = true }, false},
		{"nextjs.cache.shared", func(*NextjsFacts) {}, false},
	}
	for _, c := range cases {
		f := healthy
		c.mut(&f)
		if got := detectNextjs(t, c.id, f, true); got.Failing != c.fail {
			t.Errorf("%s: failing=%v want %v", c.id, got.Failing, c.fail)
		}
	}
}

func TestNextjsFixesSkipNonNextjsAndSurfaceErrors(t *testing.T) {
	if a := detectNextjs(t, "nextjs.output.standalone", NextjsFacts{}, false); a.Failing {
		t.Error("a non-Next.js app must not fail")
	}
	d := NextjsDeps{Facts: func(context.Context, string) (NextjsFacts, bool, error) {
		return NextjsFacts{}, false, errors.New("boom")
	}}
	if _, err := NextjsFixes("web", d)[0].Detect(context.Background()); err == nil {
		t.Error("expected the error to surface")
	}
}
