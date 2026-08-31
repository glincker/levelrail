package backup

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestContainerVolumeArchiver_Archive(t *testing.T) {
	rt := &fakeVolumeRuntime{execContent: "tar-bytes"}
	a := &ContainerVolumeArchiver{Runtime: rt}

	rc, err := a.Archive(context.Background(), "app-web-data")
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "tar-bytes" {
		t.Errorf("content = %q, want %q", got, "tar-bytes")
	}

	if rt.gotExecID != "helper-"+rt.gotCreateSpec.Name {
		t.Errorf("Exec ran against %q, want the helper container's own id", rt.gotExecID)
	}
	wantCmd := []string{"tar", "-cf", "-", "-C", volumeMountPath, "."}
	if len(rt.gotExecCmd) != len(wantCmd) {
		t.Fatalf("exec cmd = %v, want %v", rt.gotExecCmd, wantCmd)
	}
	for i := range wantCmd {
		if rt.gotExecCmd[i] != wantCmd[i] {
			t.Errorf("exec cmd[%d] = %q, want %q", i, rt.gotExecCmd[i], wantCmd[i])
		}
	}

	if rt.removeCalls != 0 {
		t.Fatalf("Remove called before Close, want 0 calls until the caller is done reading")
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if rt.removeCalls != 1 {
		t.Errorf("Remove calls after Close = %d, want 1 (helper container cleaned up)", rt.removeCalls)
	}
}

func TestContainerVolumeArchiver_Archive_CreateFailure(t *testing.T) {
	rt := &fakeVolumeRuntime{createErr: errors.New("create failed")}
	a := &ContainerVolumeArchiver{Runtime: rt}

	_, err := a.Archive(context.Background(), "app-web-data")
	if err == nil {
		t.Fatal("Archive() error = nil, want the Create failure")
	}
}

func TestContainerVolumeArchiver_Archive_ExecFailure_RemovesHelper(t *testing.T) {
	rt := &fakeVolumeRuntime{execErr: errors.New("exec failed")}
	a := &ContainerVolumeArchiver{Runtime: rt}

	_, err := a.Archive(context.Background(), "app-web-data")
	if err == nil {
		t.Fatal("Archive() error = nil, want the Exec failure")
	}
	if rt.removeCalls != 1 {
		t.Errorf("Remove calls after an Exec failure = %d, want 1 (no leaked helper container)", rt.removeCalls)
	}
}
