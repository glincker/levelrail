package backup

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

const (
	testVolumeCloneSourceName = "levelrail-test-volclonerestore-source"
	testVolumeCloneNewName    = "levelrail-test-volclonerestore-new"
)

func runInNamedVolume(ctx context.Context, t *testing.T, rt docker.Runtime, volumeName string, cmd []string) string {
	t.Helper()
	id, err := createVolumeHelper(ctx, rt, volumeName, "volclonerestore-live-helper", false)
	if err != nil {
		t.Fatalf("createVolumeHelper(%q) error = %v", volumeName, err)
	}
	defer func() { _ = rt.Remove(context.Background(), id, true) }()

	rc, err := rt.Exec(ctx, id, cmd)
	if err != nil {
		t.Fatalf("Exec(%v) on %q error = %v", cmd, volumeName, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Exec(%v) on %q read output error = %v", cmd, volumeName, err)
	}
	return string(out)
}

// TestVolumeCloneRestore_Live_NewVolumeGetsData_SourceUntouched is the
// real-Docker proof VolumeCloneRestoreRunner's own unit tests (fakes only)
// can't give: EnsureVolume creates a brand-new, genuinely empty volume,
// ContainerVolumeArchiver/ContainerVolumeRestorer round-trip real tar
// bytes into it, and the source volume's own contents are read back
// afterward to confirm restoring into the new volume never touched it.
// S3 upload/download is skipped in favor of holding the archived bytes
// directly, the same simplification TestContainerVolumeArchiver_
// Restorer_RoundTrip_Live (volume_live_test.go) already makes: the
// runner's own resolve-download-restore sequence is exercised with fakes
// in volume_clone_restore_runner_test.go, this test's job is only to
// prove the real Docker mechanics (EnsureVolume, Archive, Restore)
// actually do what those fakes assume.
func TestVolumeCloneRestore_Live_NewVolumeGetsData_SourceUntouched(t *testing.T) {
	if testing.Short() {
		t.Skip("real Docker test, skipped in short mode; see nightly.yml for the full run")
	}
	rt := liveRuntime(t)
	ctx := context.Background()

	if err := rt.EnsureVolume(ctx, testVolumeCloneSourceName); err != nil {
		t.Fatalf("EnsureVolume(source) error = %v", err)
	}
	runInNamedVolume(ctx, t, rt, testVolumeCloneSourceName, []string{"sh", "-c",
		"rm -rf " + volumeMountPath + "/* " + volumeMountPath + "/.[!.]* 2>/dev/null; mkdir -p " + volumeMountPath + "/sub && echo source-file > " + volumeMountPath + "/file.txt && echo source-nested > " + volumeMountPath + "/sub/nested.txt",
	})

	a := &ContainerVolumeArchiver{Runtime: rt}
	archiveStream, err := a.Archive(ctx, testVolumeCloneSourceName)
	if err != nil {
		t.Fatalf("Archive(source) error = %v", err)
	}
	var archiveBuf bytes.Buffer
	if _, err := io.Copy(&archiveBuf, archiveStream); err != nil {
		t.Fatalf("reading Archive() stream error = %v", err)
	}
	_ = archiveStream.Close()
	if archiveBuf.Len() == 0 {
		t.Fatal("Archive() produced an empty tar, test setup is broken")
	}

	// The exact two calls VolumeCloneRestoreRunner.createAndRestore makes,
	// against a volume name distinct from the source: EnsureVolume first
	// (a brand-new, empty volume), then Restore into it.
	if err := rt.EnsureVolume(ctx, testVolumeCloneNewName); err != nil {
		t.Fatalf("EnsureVolume(new) error = %v", err)
	}
	r := &ContainerVolumeRestorer{Runtime: rt}
	if err := r.Restore(ctx, testVolumeCloneNewName, bytes.NewReader(archiveBuf.Bytes())); err != nil {
		t.Fatalf("Restore(new) error = %v", err)
	}

	gotNewFile := strings.TrimSpace(runInNamedVolume(ctx, t, rt, testVolumeCloneNewName, []string{"cat", volumeMountPath + "/file.txt"}))
	if gotNewFile != "source-file" {
		t.Errorf("new volume file.txt = %q, want %q", gotNewFile, "source-file")
	}
	gotNewNested := strings.TrimSpace(runInNamedVolume(ctx, t, rt, testVolumeCloneNewName, []string{"cat", volumeMountPath + "/sub/nested.txt"}))
	if gotNewNested != "source-nested" {
		t.Errorf("new volume sub/nested.txt = %q, want %q", gotNewNested, "source-nested")
	}

	// The source volume was never passed to EnsureVolume or Restore for
	// the "new" side of this test: read it back directly to confirm it is
	// still exactly what was written before Archive ran.
	gotSourceFile := strings.TrimSpace(runInNamedVolume(ctx, t, rt, testVolumeCloneSourceName, []string{"cat", volumeMountPath + "/file.txt"}))
	if gotSourceFile != "source-file" {
		t.Errorf("source volume file.txt after clone-restore = %q, want %q (untouched)", gotSourceFile, "source-file")
	}
	gotSourceNested := strings.TrimSpace(runInNamedVolume(ctx, t, rt, testVolumeCloneSourceName, []string{"cat", volumeMountPath + "/sub/nested.txt"}))
	if gotSourceNested != "source-nested" {
		t.Errorf("source volume sub/nested.txt after clone-restore = %q, want %q (untouched)", gotSourceNested, "source-nested")
	}
}
