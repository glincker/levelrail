package backup

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type moveTestArchiver struct {
	gotVolumeName string
	content       string
	err           error
	closed        bool
}

func (a *moveTestArchiver) Archive(_ context.Context, volumeName string) (io.ReadCloser, error) {
	a.gotVolumeName = volumeName
	if a.err != nil {
		return nil, a.err
	}
	return &closeTrackingReader{Reader: strings.NewReader(a.content), onClose: func() { a.closed = true }}, nil
}

type closeTrackingReader struct {
	io.Reader
	onClose func()
}

func (r *closeTrackingReader) Close() error {
	r.onClose()
	return nil
}

type moveTestRestorer struct {
	gotVolumeName string
	gotBody       string
	err           error
}

func (r *moveTestRestorer) Restore(_ context.Context, volumeName string, archive io.Reader) error {
	r.gotVolumeName = volumeName
	if r.err != nil {
		return r.err
	}
	b, err := io.ReadAll(archive)
	if err != nil {
		return err
	}
	r.gotBody = string(b)
	return nil
}

func TestMoveVolume_Success(t *testing.T) {
	archiver := &moveTestArchiver{content: "tar-bytes"}
	restorer := &moveTestRestorer{}

	if err := MoveVolume(context.Background(), archiver, restorer, "app-web-data"); err != nil {
		t.Fatalf("MoveVolume() error = %v", err)
	}

	if archiver.gotVolumeName != "app-web-data" {
		t.Errorf("archiver received volume %q, want %q", archiver.gotVolumeName, "app-web-data")
	}
	if restorer.gotVolumeName != "app-web-data" {
		t.Errorf("restorer received volume %q, want %q", restorer.gotVolumeName, "app-web-data")
	}
	if restorer.gotBody != "tar-bytes" {
		t.Errorf("restored body = %q, want %q", restorer.gotBody, "tar-bytes")
	}
	if !archiver.closed {
		t.Error("archive stream was never closed")
	}
}

func TestMoveVolume_ArchiveFailure(t *testing.T) {
	archiver := &moveTestArchiver{err: errors.New("archive failed")}
	restorer := &moveTestRestorer{}

	err := MoveVolume(context.Background(), archiver, restorer, "app-web-data")
	if err == nil {
		t.Fatal("MoveVolume() error = nil, want the archive failure")
	}
	if restorer.gotVolumeName != "" {
		t.Error("restorer should never be called when archiving fails")
	}
}

func TestMoveVolume_RestoreFailure(t *testing.T) {
	archiver := &moveTestArchiver{content: "tar-bytes"}
	restorer := &moveTestRestorer{err: errors.New("restore failed")}

	err := MoveVolume(context.Background(), archiver, restorer, "app-web-data")
	if err == nil {
		t.Fatal("MoveVolume() error = nil, want the restore failure")
	}
	if !archiver.closed {
		t.Error("archive stream should still be closed when restore fails")
	}
}
