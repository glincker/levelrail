package build

import (
	"context"
	"testing"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"
	"github.com/opencontainers/go-digest"
)

func TestWithCgroupParentRewritesExecOpsAndKeepsGraphConsistent(t *testing.T) {
	st := llb.Image("alpine:3.20").Run(llb.Shlex("echo one")).Root()
	st = st.Run(llb.Shlex("echo two")).Root()
	def, err := st.Marshal(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got, err := withCgroupParent(def, "levelrail-builds/web")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Def) != len(def.Def) {
		t.Fatalf("ops = %d, want %d", len(got.Def), len(def.Def))
	}

	known := map[string]bool{}
	execs := 0
	for _, raw := range got.Def {
		var op pb.Op
		if err := op.UnmarshalVT(raw); err != nil {
			t.Fatal(err)
		}
		for _, in := range op.Inputs {
			if !known[in.Digest] {
				t.Fatalf("input %s does not match an earlier op digest", in.Digest)
			}
		}
		if ex := op.GetExec(); ex != nil {
			execs++
			if ex.Meta.CgroupParent != "levelrail-builds/web" {
				t.Fatalf("cgroup parent = %q", ex.Meta.CgroupParent)
			}
		}
		known[string(digest.FromBytes(raw))] = true
	}
	if execs != 2 {
		t.Fatalf("exec ops = %d, want 2", execs)
	}
	for d := range got.Metadata {
		if !known[string(d)] {
			t.Fatalf("metadata digest %s has no op", d)
		}
	}
}

func TestNewSolveOptSetsCgroupParent(t *testing.T) {
	dir := t.TempDir()
	opt, err := newSolveOpt(Request{ContextDir: dir, Tag: "reg/web:abc", CgroupParent: "levelrail-builds/web"}, CacheConfig{}, nopWriteCloser{})
	if err != nil {
		t.Fatal(err)
	}
	if opt.FrontendAttrs["cgroup-parent"] != "levelrail-builds/web" {
		t.Fatalf("attrs = %v", opt.FrontendAttrs)
	}
}
