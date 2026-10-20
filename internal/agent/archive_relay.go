package agent

import (
	"context"
	"errors"
	"io"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// attachArchive serves a tar copy over the exec frame stream: copy-out
// relays the container archive as output frames, copy-in feeds arriving
// input frames to the archive API and ends the stream when it returns.
func (r *ExecRelay) attachArchive(ctx context.Context, ex *agentExec, req *agentpb.ExecRequest, stdinR *io.PipeReader) (io.ReadCloser, error) {
	ar, ok := r.rt.(docker.ArchiveRuntime)
	if !ok {
		return nil, docker.ErrArchiveUnsupported
	}
	op := req.GetArchive()
	if op.GetPath() == "" {
		return nil, errors.New("agent: archive copy needs a path")
	}
	if !op.GetCopyIn() {
		rc, st, err := ar.CopyFromContainer(ctx, req.GetContainerId(), op.GetPath(), op.GetMaxBytes())
		if err != nil {
			return nil, err
		}
		ex.stat = &agentpb.ArchiveStat{Name: st.Name, Size: st.Size, Mode: uint32(st.Mode), LinkTarget: st.LinkTarget}
		if !ex.attach(rc) {
			_ = rc.Close()
			return nil, nil
		}
		return rc, nil
	}

	pr, pw := io.Pipe()
	if !ex.attach(pr) {
		_ = pr.Close()
		return nil, nil
	}
	go func() {
		err := ar.CopyToContainer(ctx, req.GetContainerId(), op.GetPath(), stdinR, op.GetMaxBytes())
		_ = pw.CloseWithError(err)
	}()
	return pr, nil
}

// ackResponse is the exec acknowledgment: it carries the path stat for a
// copy-out and the empty result for everything else.
func ackResponse(execID string, ex *agentExec) *agentpb.AgentResponse {
	if ex.stat != nil {
		return &agentpb.AgentResponse{RequestId: execID, Result: &agentpb.AgentResponse_ArchiveStat{ArchiveStat: ex.stat}}
	}
	return &agentpb.AgentResponse{RequestId: execID, Result: emptyResult()}
}
