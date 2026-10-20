package backup

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestContainerVolumeRestorer_Restore(t *testing.T) {
	rt := &fakeVolumeRuntime{}
	r := &ContainerVolumeRestorer{Runtime: rt}

	err := r.Restore(context.Background(), "app-web-data", strings.NewReader("tar-bytes"))
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if rt.gotStdin != "tar-bytes" {
		t.Errorf("stdin fed to ExecWithInput = %q, want %q", rt.gotStdin, "tar-bytes")
	}
	if !strings.Contains(rt.gotInputCmd[2], "rm -rf") || !strings.Contains(rt.gotInputCmd[2], "tar -xf") {
		t.Errorf("restore cmd = %q, want it to wipe the volume then extract the tar", rt.gotInputCmd[2])
	}
	if rt.removeCalls != 1 {
		t.Errorf("Remove calls = %d, want 1 (helper container always cleaned up)", rt.removeCalls)
	}
	if len(rt.gotCreateSpec.Volumes) != 1 || rt.gotCreateSpec.Volumes[0].ReadOnly {
		t.Errorf("restore mount = %+v, want read-write (not ReadOnly)", rt.gotCreateSpec.Volumes)
	}
}

func TestContainerVolumeRestorer_Restore_CreateFailure(t *testing.T) {
	rt := &fakeVolumeRuntime{createErr: errors.New("create failed")}
	r := &ContainerVolumeRestorer{Runtime: rt}

	err := r.Restore(context.Background(), "app-web-data", strings.NewReader("x"))
	if err == nil {
		t.Fatal("Restore() error = nil, want the Create failure")
	}
}

func TestContainerVolumeRestorer_Restore_ExecFailure_StillRemovesHelper(t *testing.T) {
	rt := &fakeVolumeRuntime{execInputErr: errors.New("exec failed")}
	r := &ContainerVolumeRestorer{Runtime: rt}

	err := r.Restore(context.Background(), "app-web-data", strings.NewReader("x"))
	if err == nil {
		t.Fatal("Restore() error = nil, want the ExecWithInput failure")
	}
	if rt.removeCalls != 1 {
		t.Errorf("Remove calls after an exec failure = %d, want 1 (no leaked helper container)", rt.removeCalls)
	}
}
