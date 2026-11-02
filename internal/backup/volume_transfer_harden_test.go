package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestImportRejectsSymlinkChainsAndBadHardLinks(t *testing.T) {
	cases := map[string][]tarEntry{
		"chain through parent link": {
			{name: "d/up", typ: tar.TypeSymlink, link: ".."},
			{name: "e", typ: tar.TypeSymlink, link: "d/up/.."},
		},
		"chain declared in reverse": {
			{name: "e", typ: tar.TypeSymlink, link: "d/up/.."},
			{name: "d/up", typ: tar.TypeSymlink, link: ".."},
		},
		"link loop":                   {{name: "a", typ: tar.TypeSymlink, link: "b"}, {name: "b", typ: tar.TypeSymlink, link: "a"}},
		"hard link to missing target": {{name: "h", typ: tar.TypeLink, link: "nope"}},
		"hard link declared too early": {
			{name: "h", typ: tar.TypeLink, link: "later"},
			{name: "later", body: "x"},
		},
		"hard link to a directory": {{name: "d/", typ: tar.TypeDir}, {name: "h", typ: tar.TypeLink, link: "d"}},
		"hard link through symlink": {
			{name: "l", typ: tar.TypeSymlink, link: "d"},
			{name: "h", typ: tar.TypeLink, link: "l/f"},
		},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			f := &fakeTransferRuntime{listing: "old\n"}
			_, err := (&VolumeTransfer{Runtime: f}).Import(context.Background(), "data", bytes.NewReader(buildArchive(t, entries)), ImportOptions{MaxBytes: 1 << 20, Overwrite: true})
			if !errors.Is(err, ErrUnsafeArchive) {
				t.Fatalf("want ErrUnsafeArchive, got %v", err)
			}
			if wipedOrExtracted(f) || len(f.specs) != 0 {
				t.Fatal("a rejected archive must not touch or mount the volume")
			}
		})
	}
}

func TestImportAcceptsInVolumeLinks(t *testing.T) {
	entries := []tarEntry{
		{name: "d/", typ: tar.TypeDir},
		{name: "d/file", body: "x"},
		{name: "d/up", typ: tar.TypeSymlink, link: ".."},
		{name: "rel", typ: tar.TypeSymlink, link: "d/file"},
		{name: "hard", typ: tar.TypeLink, link: "d/file"},
	}
	f := &fakeTransferRuntime{}
	if _, err := (&VolumeTransfer{Runtime: f}).Import(context.Background(), "data", bytes.NewReader(buildArchive(t, entries)), ImportOptions{MaxBytes: 1 << 20}); err != nil {
		t.Fatalf("safe links rejected: %v", err)
	}
}

// probeReader fails the test if the volume is mounted while the archive is still being read.
type probeReader struct {
	io.Reader
	t *testing.T
	f *fakeTransferRuntime
}

func (p probeReader) Read(b []byte) (int, error) {
	if len(p.f.specs) != 0 {
		p.t.Fatal("the volume was mounted before the upload finished")
	}
	return p.Reader.Read(b)
}

func TestImportStagesInSpoolDirBeforeMountAndRechecks(t *testing.T) {
	arc := buildArchive(t, []tarEntry{{name: "a", body: "x"}})
	spool := filepath.Join(t.TempDir(), "spool")
	f := &fakeTransferRuntime{}
	var sawStaged bool
	opts := ImportOptions{MaxBytes: 1 << 20, SpoolDir: spool, BeforeMount: func(context.Context) error {
		ents, _ := os.ReadDir(spool)
		sawStaged = len(ents) == 1 && strings.HasPrefix(ents[0].Name(), SpoolFilePrefix)
		return nil
	}}
	if _, err := (&VolumeTransfer{Runtime: f}).Import(context.Background(), "data", probeReader{bytes.NewReader(arc), t, f}, opts); err != nil {
		t.Fatal(err)
	}
	if !sawStaged {
		t.Fatal("the archive was not staged in the spool dir")
	}
	if ents, _ := os.ReadDir(spool); len(ents) != 0 {
		t.Fatalf("spool file left behind: %v", ents)
	}

	// An app that started during the upload must stop the import before any mount.
	errStarted := errors.New("app started")
	f2 := &fakeTransferRuntime{listing: "old\n"}
	opts.BeforeMount = func(context.Context) error { return errStarted }
	opts.Overwrite = true
	if _, err := (&VolumeTransfer{Runtime: f2}).Import(context.Background(), "data", bytes.NewReader(arc), opts); !errors.Is(err, errStarted) {
		t.Fatalf("want the recheck error, got %v", err)
	}
	if len(f2.specs) != 0 || wipedOrExtracted(f2) {
		t.Fatal("a failed recheck must not mount, wipe or write")
	}
}

func TestCleanSpoolDirRemovesOnlyStagedArchives(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, SpoolFilePrefix+"123.tar")
	other := filepath.Join(dir, "keep.txt")
	for _, p := range []string{stale, other} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	CleanSpoolDir(dir)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale spool file survived")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("an unrelated file was removed")
	}
}

func TestHelperSpecIsLockedDown(t *testing.T) {
	for _, readOnly := range []bool{true, false} {
		spec := transferHelperSpec("h", "vol", readOnly)
		if spec.NetworkMode != "none" {
			t.Fatalf("network = %q, want none", spec.NetworkMode)
		}
		if !spec.Hardened {
			t.Fatal("helper must use the hardened profile (read-only root, drop all caps, no-new-privileges)")
		}
		if spec.Resources == nil || spec.Resources.MemoryBytes <= 0 || spec.PidsLimit <= 0 {
			t.Fatalf("missing memory or pids limit: %+v", spec)
		}
		if slices.Contains(spec.HardenedCapAdd, "SYS_ADMIN") || slices.Contains(spec.HardenedCapAdd, "NET_RAW") || slices.Contains(spec.HardenedCapAdd, "ALL") {
			t.Fatalf("too many capabilities: %v", spec.HardenedCapAdd)
		}
		if readOnly && (len(spec.HardenedCapAdd) != 1 || spec.HardenedCapAdd[0] != "DAC_READ_SEARCH") {
			t.Fatalf("export caps = %v", spec.HardenedCapAdd)
		}
		if spec.Volumes[0].ReadOnly != readOnly {
			t.Fatal("mount mode does not match the helper kind")
		}
	}
}
