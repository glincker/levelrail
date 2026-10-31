package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/volumefiles"
)

// ErrVolumeFilesUnsupported means the runtime cannot map a volume to a host directory.
var ErrVolumeFilesUnsupported = errors.New("agent: this runtime cannot resolve a volume's host directory")

type volumeMounter interface {
	VolumeMountpointIfExists(ctx context.Context, name string) (string, error)
}

// VolumeFileEntry is one item of a volume directory listing.
type VolumeFileEntry = volumefiles.Entry

// openVolumeStore resolves the volume on rt and confines a Store to it.
func openVolumeStore(ctx context.Context, rt docker.Runtime, volume string) (*volumefiles.Store, error) {
	vm, ok := rt.(volumeMounter)
	if !ok {
		return nil, ErrVolumeFilesUnsupported
	}
	if volume == "" {
		return nil, errors.New("agent: volume file operation needs a volume name")
	}
	dir, err := vm.VolumeMountpointIfExists(ctx, volume)
	if err != nil {
		return nil, err
	}
	return volumefiles.Open(dir)
}

// runVolumeFile executes one op against the volume and returns its output stream.
// Caps come from the agent's own env, so a control plane cannot raise them.
func runVolumeFile(ctx context.Context, rt docker.Runtime, op *agentpb.VolumeFileOp, stdin io.Reader) (io.ReadCloser, error) {
	st, err := openVolumeStore(ctx, rt, op.GetVolume())
	if err != nil {
		return nil, err
	}
	switch op.GetKind() {
	case agentpb.VolumeFileOp_KIND_LIST:
		defer func() { _ = st.Close() }()
		entries, truncated, err := st.List(op.GetPath(), volumefiles.EffectiveLimit(int(op.GetLimit())))
		if err != nil {
			return nil, err
		}
		pr, pw := io.Pipe()
		go func() { _ = pw.CloseWithError(volumefiles.EncodeEntries(pw, entries, truncated)) }()
		return pr, nil
	case agentpb.VolumeFileOp_KIND_GET:
		rc, _, err := st.Read(op.GetPath())
		if err != nil {
			_ = st.Close()
			return nil, err
		}
		return &closeBoth{ReadCloser: rc, extra: st}, nil
	case agentpb.VolumeFileOp_KIND_PUT:
		pr, pw := io.Pipe()
		go func() {
			defer func() { _ = st.Close() }()
			var mode fs.FileMode
			if op.GetPrivate() {
				mode = 0o600
			}
			_, err := st.WriteMode(op.GetPath(), stdin, volumefiles.EffectiveMax(op.GetMaxBytes()), mode)
			_ = pw.CloseWithError(err)
		}()
		return pr, nil
	case agentpb.VolumeFileOp_KIND_DELETE:
		defer func() { _ = st.Close() }()
		if err := st.Remove(op.GetPath()); err != nil {
			return nil, err
		}
		return io.NopCloser(eofReader{}), nil
	default:
		_ = st.Close()
		return nil, errors.New("agent: unsupported volume file operation")
	}
}

type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }

type closeBoth struct {
	io.ReadCloser
	extra io.Closer
}

func (c *closeBoth) Close() error {
	err := c.ReadCloser.Close()
	_ = c.extra.Close()
	return err
}

// attachVolumeFile serves one VolumeFileOp over the exec frame stream.
func (r *ExecRelay) attachVolumeFile(ctx context.Context, ex *agentExec, req *agentpb.ExecRequest, stdinR *io.PipeReader) (io.ReadCloser, error) {
	var stdin io.Reader
	if stdinR != nil {
		stdin = stdinR
	}
	rc, err := runVolumeFile(ctx, r.rt, req.GetVolumeFile(), stdin)
	if err != nil {
		return nil, err
	}
	if !ex.attach(rc) {
		_ = rc.Close()
		return nil, nil
	}
	return rc, nil
}

// VolumeFileOps is the node-side surface the API uses for volume file browsing.
type VolumeFileOps interface {
	VolumeList(ctx context.Context, volume, dir string, limit int) ([]VolumeFileEntry, bool, error)
	VolumeRead(ctx context.Context, volume, path string) (io.ReadCloser, error)
	VolumeWrite(ctx context.Context, volume, path string, content io.Reader, maxBytes int64) error
	VolumeRemove(ctx context.Context, volume, path string) error
}

func decodeList(rc io.ReadCloser) ([]VolumeFileEntry, bool, error) {
	defer func() { _ = rc.Close() }()
	entries, truncated, err := volumefiles.DecodeEntries(rc)
	if err != nil {
		return nil, false, fmt.Errorf("agent: volume list: %w", err)
	}
	return entries, truncated, nil
}

func drain(rc io.ReadCloser) error {
	defer func() { _ = rc.Close() }()
	_, err := io.Copy(io.Discard, rc)
	return err
}

// VolumeList implements VolumeFileOps over the exec frame stream.
func (t *GRPCTransport) VolumeList(ctx context.Context, volume, dir string, limit int) ([]VolumeFileEntry, bool, error) {
	rc, _, err := t.startExecAck(ctx, "", nil, nil, execAttach{
		volumeFile: &agentpb.VolumeFileOp{Kind: agentpb.VolumeFileOp_KIND_LIST, Volume: volume, Path: dir, Limit: int32(limit)}, //nolint:gosec // limit is bounded by volumefiles.ListLimit
	})
	if err != nil {
		return nil, false, err
	}
	return decodeList(rc)
}

// VolumeRead implements VolumeFileOps; the caller closes the stream.
func (t *GRPCTransport) VolumeRead(ctx context.Context, volume, path string) (io.ReadCloser, error) {
	rc, _, err := t.startExecAck(ctx, "", nil, nil, execAttach{
		volumeFile: &agentpb.VolumeFileOp{Kind: agentpb.VolumeFileOp_KIND_GET, Volume: volume, Path: path},
	})
	return rc, err
}

// VolumeWrite implements VolumeFileOps.
func (t *GRPCTransport) VolumeWrite(ctx context.Context, volume, path string, content io.Reader, maxBytes int64) error {
	return t.volumePut(ctx, &agentpb.VolumeFileOp{Kind: agentpb.VolumeFileOp_KIND_PUT, Volume: volume, Path: path, MaxBytes: maxBytes}, content)
}

// VolumeWritePrivate is VolumeWrite for a file created 0600, such as a private key.
func (t *GRPCTransport) VolumeWritePrivate(ctx context.Context, volume, path string, content io.Reader, maxBytes int64) error {
	return t.volumePut(ctx, &agentpb.VolumeFileOp{Kind: agentpb.VolumeFileOp_KIND_PUT, Volume: volume, Path: path, MaxBytes: maxBytes, Private: true}, content)
}

func (t *GRPCTransport) volumePut(ctx context.Context, op *agentpb.VolumeFileOp, content io.Reader) error {
	rc, _, err := t.startExecAck(ctx, "", nil, content, execAttach{stdin: true, volumeFile: op})
	if err != nil {
		return err
	}
	return drain(rc)
}

// VolumeRemove implements VolumeFileOps.
func (t *GRPCTransport) VolumeRemove(ctx context.Context, volume, path string) error {
	rc, _, err := t.startExecAck(ctx, "", nil, nil, execAttach{
		volumeFile: &agentpb.VolumeFileOp{Kind: agentpb.VolumeFileOp_KIND_DELETE, Volume: volume, Path: path},
	})
	if err != nil {
		return err
	}
	return drain(rc)
}

// VolumeList implements VolumeFileOps for the control plane's own node.
func (l Local) VolumeList(ctx context.Context, volume, dir string, limit int) ([]VolumeFileEntry, bool, error) {
	rc, err := runVolumeFile(ctx, l.Runtime, &agentpb.VolumeFileOp{Kind: agentpb.VolumeFileOp_KIND_LIST, Volume: volume, Path: dir, Limit: int32(limit)}, nil) //nolint:gosec // limit is bounded by volumefiles.ListLimit
	if err != nil {
		return nil, false, err
	}
	return decodeList(rc)
}

// VolumeRead implements VolumeFileOps for the control plane's own node.
func (l Local) VolumeRead(ctx context.Context, volume, path string) (io.ReadCloser, error) {
	return runVolumeFile(ctx, l.Runtime, &agentpb.VolumeFileOp{Kind: agentpb.VolumeFileOp_KIND_GET, Volume: volume, Path: path}, nil)
}

// VolumeWrite implements VolumeFileOps for the control plane's own node.
func (l Local) VolumeWrite(ctx context.Context, volume, path string, content io.Reader, maxBytes int64) error {
	return l.volumePut(ctx, &agentpb.VolumeFileOp{Kind: agentpb.VolumeFileOp_KIND_PUT, Volume: volume, Path: path, MaxBytes: maxBytes}, content)
}

// VolumeWritePrivate is VolumeWrite for a file created 0600 on the control plane's own node.
func (l Local) VolumeWritePrivate(ctx context.Context, volume, path string, content io.Reader, maxBytes int64) error {
	return l.volumePut(ctx, &agentpb.VolumeFileOp{Kind: agentpb.VolumeFileOp_KIND_PUT, Volume: volume, Path: path, MaxBytes: maxBytes, Private: true}, content)
}

func (l Local) volumePut(ctx context.Context, op *agentpb.VolumeFileOp, content io.Reader) error {
	rc, err := runVolumeFile(ctx, l.Runtime, op, content)
	if err != nil {
		return err
	}
	return drain(rc)
}

// VolumeRemove implements VolumeFileOps for the control plane's own node.
func (l Local) VolumeRemove(ctx context.Context, volume, path string) error {
	rc, err := runVolumeFile(ctx, l.Runtime, &agentpb.VolumeFileOp{Kind: agentpb.VolumeFileOp_KIND_DELETE, Volume: volume, Path: path}, nil)
	if err != nil {
		return err
	}
	return drain(rc)
}
