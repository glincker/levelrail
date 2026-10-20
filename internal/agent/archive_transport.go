package agent

import (
	"context"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// CopyFromContainer implements docker.ArchiveRuntime over the exec frame
// stream: the same window, credit and cancel as any exec, no new RPC shape.
func (t *GRPCTransport) CopyFromContainer(ctx context.Context, containerID, path string, maxBytes int64) (io.ReadCloser, docker.PathStat, error) {
	rc, ack, err := t.startExecAck(ctx, containerID, nil, nil, execAttach{
		archive: &agentpb.ArchiveOp{Path: path, MaxBytes: maxBytes},
	})
	if err != nil {
		return nil, docker.PathStat{}, err
	}
	st := ack.GetArchiveStat()
	return rc, docker.PathStat{Name: st.GetName(), Size: st.GetSize(), Mode: os.FileMode(st.GetMode()), LinkTarget: st.GetLinkTarget()}, nil
}

// CopyToContainer implements docker.ArchiveRuntime. It returns once the
// agent's archive API has consumed the whole stream, or with its error.
func (t *GRPCTransport) CopyToContainer(ctx context.Context, containerID, dir string, content io.Reader, maxBytes int64) error {
	rc, _, err := t.startExecAck(ctx, containerID, nil, content, execAttach{
		stdin:   true,
		archive: &agentpb.ArchiveOp{CopyIn: true, Path: dir, MaxBytes: maxBytes},
	})
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(io.Discard, rc)
	return err
}

// CopyFromContainer implements docker.ArchiveRuntime by forwarding to the
// wrapped runtime, which the embedded interface would otherwise hide.
func (l Local) CopyFromContainer(ctx context.Context, containerID, path string, maxBytes int64) (io.ReadCloser, docker.PathStat, error) {
	ar, ok := l.Runtime.(docker.ArchiveRuntime)
	if !ok {
		return nil, docker.PathStat{}, docker.ErrArchiveUnsupported
	}
	return ar.CopyFromContainer(ctx, containerID, path, maxBytes)
}

// CopyToContainer implements docker.ArchiveRuntime by forwarding.
func (l Local) CopyToContainer(ctx context.Context, containerID, dir string, content io.Reader, maxBytes int64) error {
	ar, ok := l.Runtime.(docker.ArchiveRuntime)
	if !ok {
		return docker.ErrArchiveUnsupported
	}
	return ar.CopyToContainer(ctx, containerID, dir, content, maxBytes)
}
