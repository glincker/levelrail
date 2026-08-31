package backup

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// testVolumeName is the one real Docker volume this file's live test
// round-trips against. A plain const, not a runInVolume parameter: this
// file has exactly one live test, so there is nothing for that parameter
// to actually vary across, and threading it through would only add an
// argument every call site has to repeat identically.
const testVolumeName = "levelrail-test-volbackup-roundtrip"

// runInVolume execs cmd inside a short-lived helper container mounting
// testVolumeName read-write at volumeMountPath, the same shape
// createVolumeHelper (volume_helper.go) establishes, used here only to
// seed/inspect test data directly rather than through the archiver/
// restorer under test.
func runInVolume(ctx context.Context, t *testing.T, rt docker.Runtime, cmd []string) string {
	t.Helper()
	id, err := createVolumeHelper(ctx, rt, testVolumeName, "volbackup-live-helper", false)
	if err != nil {
		t.Fatalf("createVolumeHelper() error = %v", err)
	}
	defer func() { _ = rt.Remove(context.Background(), id, true) }()

	rc, err := rt.Exec(ctx, id, cmd)
	if err != nil {
		t.Fatalf("Exec(%v) error = %v", cmd, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Exec(%v) read output error = %v", cmd, err)
	}
	return string(out)
}

// TestContainerVolumeArchiver_Restorer_RoundTrip_Live is the strongest
// proof available that a volume backup actually round-trips, run
// against a real Docker volume and a real Alpine helper container, no
// mock or fake anywhere in the path: write known files (including a
// nested directory and a dotfile, to prove the tar captures the whole
// tree, not just top-level entries) into a real named volume, archive it
// (ContainerVolumeArchiver), corrupt the volume's contents to prove a
// real, distinguishable change happened, restore from the captured
// archive (ContainerVolumeRestorer), then read the volume's contents
// back directly and confirm the original data is there and the
// corruption is gone.
func TestContainerVolumeArchiver_Restorer_RoundTrip_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	if err := rt.EnsureVolume(ctx, testVolumeName); err != nil {
		t.Fatalf("EnsureVolume() error = %v", err)
	}
	// No RemoveVolume on docker.Runtime today (only docker.Client's own
	// unexported SDK handle can remove a volume, internal/docker's own
	// live tests do this via c.cli.VolumeRemove directly, not available
	// from this package): the volume is left behind after this test, the
	// same small, honest gap left open rather than reaching for a raw
	// docker CLI shell-out this codebase's own rules forbid.
	runInVolume(ctx, t, rt, []string{"sh", "-c",
		"rm -rf " + volumeMountPath + "/* " + volumeMountPath + "/.[!.]* 2>/dev/null; mkdir -p " + volumeMountPath + "/sub && echo original-file > " + volumeMountPath + "/file.txt && echo original-nested > " + volumeMountPath + "/sub/nested.txt && echo original-dotfile > " + volumeMountPath + "/.hidden",
	})

	a := &ContainerVolumeArchiver{Runtime: rt}
	archiveStream, err := a.Archive(ctx, testVolumeName)
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	var archiveBuf bytes.Buffer
	if _, err := io.Copy(&archiveBuf, archiveStream); err != nil {
		t.Fatalf("reading Archive() stream error = %v", err)
	}
	_ = archiveStream.Close()
	if archiveBuf.Len() == 0 {
		t.Fatal("Archive() produced an empty tar, test setup is broken")
	}

	// Corrupt: overwrite file.txt, delete the nested file and the
	// dotfile entirely, proving a real, distinguishable change happened
	// before restore runs, and that restore must recreate deleted
	// entries, not just overwrite existing ones.
	runInVolume(ctx, t, rt, []string{"sh", "-c",
		"echo corrupted > " + volumeMountPath + "/file.txt && rm -rf " + volumeMountPath + "/sub " + volumeMountPath + "/.hidden",
	})
	corrupted := runInVolume(ctx, t, rt, []string{"cat", volumeMountPath + "/file.txt"})
	if strings.TrimSpace(corrupted) != "corrupted" {
		t.Fatalf("corruption step did not take effect, got %q", corrupted)
	}

	r := &ContainerVolumeRestorer{Runtime: rt}
	if err := r.Restore(ctx, testVolumeName, bytes.NewReader(archiveBuf.Bytes())); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	gotFile := strings.TrimSpace(runInVolume(ctx, t, rt, []string{"cat", volumeMountPath + "/file.txt"}))
	if gotFile != "original-file" {
		t.Errorf("file.txt after restore = %q, want %q", gotFile, "original-file")
	}
	gotNested := strings.TrimSpace(runInVolume(ctx, t, rt, []string{"cat", volumeMountPath + "/sub/nested.txt"}))
	if gotNested != "original-nested" {
		t.Errorf("sub/nested.txt after restore = %q, want %q", gotNested, "original-nested")
	}
	gotDotfile := strings.TrimSpace(runInVolume(ctx, t, rt, []string{"cat", volumeMountPath + "/.hidden"}))
	if gotDotfile != "original-dotfile" {
		t.Errorf(".hidden after restore = %q, want %q", gotDotfile, "original-dotfile")
	}
}
