package cleanup

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/registrycatalog"
)

type fakeRegistry struct {
	tags    map[string][]registrycatalog.TagInfo
	use     map[string]map[string]bool
	deleted []string
	gc      RegistryGCReport
	gcRuns  []bool
}

func (f *fakeRegistry) Repositories(context.Context) ([]string, error) {
	var out []string
	for r := range f.tags {
		out = append(out, r)
	}
	return out, nil
}
func (f *fakeRegistry) Tags(_ context.Context, r string) ([]registrycatalog.TagInfo, error) {
	return f.tags[r], nil
}
func (f *fakeRegistry) Delete(_ context.Context, r, d string) error {
	f.deleted = append(f.deleted, r+"@"+d)
	return nil
}
func (f *fakeRegistry) InUse(context.Context) (map[string]map[string]bool, error) { return f.use, nil }
func (f *fakeRegistry) GarbageCollect(_ context.Context, dry bool) (RegistryGCReport, error) {
	f.gcRuns = append(f.gcRuns, dry)
	return f.gc, nil
}

func tag(name string, age int) registrycatalog.TagInfo {
	return registrycatalog.TagInfo{Tag: name, Digest: "sha256:" + name, Created: time.Unix(1000-int64(age), 0)}
}

func TestRegistryTagsPruneNeverDeletesTagInUse(t *testing.T) {
	f := &fakeRegistry{
		tags: map[string][]registrycatalog.TagInfo{"app": {tag("v1", 40), tag("v2", 30), tag("v3", 20), tag("v4", 10)}},
		use:  map[string]map[string]bool{"app": {"v1": true}},
	}
	tg := &registryTagsTarget{src: f}
	if imp, _ := tg.Preview(context.Background(), 1); imp.Rows != 2 {
		t.Fatalf("preview rows = %d, want 2 (v2 and v3)", imp.Rows)
	}
	if res := tg.Run(context.Background(), 1, RunOptions{DryRun: true}); res.Rows != 2 || len(f.deleted) != 0 {
		t.Fatalf("dry run rows=%d deleted=%v, want 2 and nothing deleted", res.Rows, f.deleted)
	}
	res := tg.Run(context.Background(), 1, RunOptions{})
	if res.Err != nil || res.Rows != 2 {
		t.Fatalf("run = %+v", res)
	}
	for _, d := range f.deleted {
		if d == "app@sha256:v1" || d == "app@sha256:v4" {
			t.Fatalf("deleted a tag in use or newest: %s", d)
		}
	}
}

func TestRegistryTagsPruneSkipsDigestPinnedRepo(t *testing.T) {
	f := &fakeRegistry{
		tags: map[string][]registrycatalog.TagInfo{"app": {tag("v1", 40), tag("v2", 30), tag("v3", 20)}},
		use:  map[string]map[string]bool{"app": {"*": true}},
	}
	tg := &registryTagsTarget{src: f}
	if res := tg.Run(context.Background(), 1, RunOptions{}); res.Rows != 0 || len(f.deleted) != 0 {
		t.Fatalf("pinned repo pruned: %+v %v", res, f.deleted)
	}
}

func TestRegistryGCDryRunFirstAndThreshold(t *testing.T) {
	f := &fakeRegistry{gc: RegistryGCReport{Blobs: 3, Bytes: 50 * bytesPerMB}}
	tg := &registryGCTarget{src: f}
	if res := tg.Run(context.Background(), 100, RunOptions{}); res.Rows != 0 {
		t.Fatalf("below threshold collected: %+v", res)
	}
	for _, dry := range f.gcRuns {
		if !dry {
			t.Fatal("a real collection ran below the threshold")
		}
	}
	f.gcRuns = nil
	if res := tg.Run(context.Background(), 10, RunOptions{}); res.Rows != 3 {
		t.Fatalf("run = %+v", res)
	}
	if len(f.gcRuns) != 2 || !f.gcRuns[0] || f.gcRuns[1] {
		t.Fatalf("gc runs = %v, want dry run then real", f.gcRuns)
	}
}

func TestSplitImageRef(t *testing.T) {
	cases := map[string][2]string{
		"127.0.0.1:5540/org/app:v2":     {"org/app", "v2"},
		"reg.example.com/app":           {"app", "latest"},
		"org/app:1":                     {"org/app", "1"},
		"reg.example.com/app@sha256:ab": {"app", "*"},
	}
	for in, want := range cases {
		if r, tg := SplitImageRef(in); r != want[0] || tg != want[1] {
			t.Errorf("SplitImageRef(%q) = %q,%q want %q,%q", in, r, tg, want[0], want[1])
		}
	}
}
