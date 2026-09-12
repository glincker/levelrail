package agent

// This file: the control plane's side of a dispatched build. A build is
// the second operation (after exec) that spans many frames in both
// directions on one Session, so it follows the same subscribe-before-call,
// credit-windowed, exactly-one-terminal-frame shape grpc_transport.go's
// exec already establishes: the build context goes up as BuildInput
// frames, progress and the resulting image tar come back as BuildOutput
// frames.

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/build"
)

// RemoteBuilder is the optional capability a Transport advertises when it
// can run a build dispatched to its node. Only *GRPCTransport does: a
// Local transport is this process's own Docker socket, which is what
// "build locally" already means, not somewhere to dispatch to.
type RemoteBuilder interface {
	BuildOnNode(ctx context.Context, req build.RemoteRequest, image io.Writer, progress func(build.ProgressEvent)) (*build.Result, error)
}

// BuildDispatcher resolves a node ID to that node's session and runs a
// build there. Satisfies internal/build.NodeBuilder, which is how
// build.Router reaches a node without knowing anything about transports.
type BuildDispatcher struct {
	registry *Registry
}

// NewBuildDispatcher builds a dispatcher over registry.
func NewBuildDispatcher(registry *Registry) *BuildDispatcher {
	return &BuildDispatcher{registry: registry}
}

// BuildOnNode implements internal/build.NodeBuilder.
func (d *BuildDispatcher) BuildOnNode(ctx context.Context, nodeID string, req build.RemoteRequest, image io.Writer, progress func(build.ProgressEvent)) (*build.Result, error) {
	transport, err := d.registry.Get(nodeID)
	if err != nil {
		return nil, err
	}
	remote, ok := transport.(RemoteBuilder)
	if !ok {
		return nil, fmt.Errorf("agent: node %q is reached through a %T, which cannot run a dispatched build", nodeID, transport)
	}
	return remote.BuildOnNode(ctx, req, image, progress)
}

// BuildOnNode implements RemoteBuilder: it dispatches req to this
// transport's node, streams req.ContextDir up as a tar, relays progress
// through progress, and writes the docker-save image tar the build
// produced into image. The image is never loaded on the build node: the
// control plane that asked for the build is the one that wants it.
func (t *GRPCTransport) BuildOnNode(ctx context.Context, req build.RemoteRequest, image io.Writer, progress func(build.ProgressEvent)) (*build.Result, error) {
	if progress == nil {
		progress = func(build.ProgressEvent) {}
	}

	pbReq, err := buildRequestToPB(req)
	if err != nil {
		return nil, err
	}
	buildID, err := randomRequestID()
	if err != nil {
		return nil, err
	}

	// Subscribe before issuing the call, the same ordering startExec
	// needs: the agent starts sending frames the moment it has accepted
	// the build, which can be before this side sees the acknowledgment.
	sub := t.mux.subscribeBuild(buildID)
	if sub == nil {
		return nil, ErrSessionClosed
	}
	defer t.mux.unsubscribeBuild(buildID)

	if _, callErr := t.mux.CallWithID(ctx, buildID, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Build{Build: pbReq},
	}); callErr != nil {
		_ = t.mux.sendBuildCancel(buildID)
		return nil, callErr
	}

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	go t.streamBuildContext(streamCtx, buildID, sub, req.ContextDir)

	res, err := t.receiveBuildOutput(ctx, buildID, req.Tag, sub, image, progress)
	if err != nil {
		// The build may still be running on the node for a caller that is
		// no longer listening, so stop it rather than leave it burning the
		// build node's CPU for nobody.
		_ = t.mux.sendBuildCancel(buildID)
		return nil, err
	}
	return res, nil
}

// streamBuildContext tars dir and sends it as BuildInput frames for as
// long as the agent's credit allows. A failure producing the tar is sent
// as an explicit error frame rather than an EOF: a half-shipped context
// must fail the build, not silently build the wrong tree.
func (t *GRPCTransport) streamBuildContext(ctx context.Context, buildID string, sub *buildSub, dir string) {
	pipeR, pipeW := io.Pipe()
	go func() { _ = pipeW.CloseWithError(build.TarContext(ctx, dir, pipeW)) }()
	defer func() { _ = pipeR.Close() }()

	buf := make([]byte, buildChunkBytes)
	credit := uint32(buildWindowFrames)

	for {
		if credit == 0 {
			select {
			case refund, ok := <-sub.credit:
				if !ok {
					return
				}
				credit += refund
			case <-ctx.Done():
				return
			}
			continue
		}

		n, readErr := pipeR.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if err := t.mux.sendBuildInput(&agentpb.BuildInput{BuildId: buildID, Chunk: chunk}); err != nil {
				return
			}
			credit--
		}
		if readErr != nil {
			final := &agentpb.BuildInput{BuildId: buildID, Eof: true}
			if !errors.Is(readErr, io.EOF) {
				final = &agentpb.BuildInput{BuildId: buildID, Error: readErr.Error()}
			}
			_ = t.mux.sendBuildInput(final)
			return
		}
	}
}

// receiveBuildOutput consumes the build's frames until its one terminal
// frame arrives, crediting consumed frames back in batches so the agent
// is not stopped by an exhausted window.
func (t *GRPCTransport) receiveBuildOutput(ctx context.Context, buildID, tag string, sub *buildSub, image io.Writer, progress func(build.ProgressEvent)) (*build.Result, error) {
	consumed := uint32(0)
	credit := func() {
		consumed++
		if consumed < buildCreditBatch {
			return
		}
		frames := consumed
		consumed = 0
		// A failed credit means the session is gone, which the next frame
		// read discovers on its own.
		_ = t.mux.sendBuildCredit(buildID, frames)
	}

	for {
		select {
		case frame, ok := <-sub.out:
			if !ok {
				if err := sub.failure(); err != nil {
					return nil, err
				}
				return nil, ErrSessionClosed
			}
			switch f := frame.GetFrame().(type) {
			case *agentpb.BuildOutput_Progress:
				progress(buildProgressFromPB(f.Progress))
				credit()
			case *agentpb.BuildOutput_ImageChunk:
				if _, err := image.Write(f.ImageChunk); err != nil {
					return nil, fmt.Errorf("agent: write dispatched build's image stream: %w", err)
				}
				credit()
			case *agentpb.BuildOutput_Done:
				return buildResultFromPB(tag, f.Done), nil
			case *agentpb.BuildOutput_Failure:
				return nil, buildFailureToError(f.Failure)
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
