package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

// FileEntry is one object found by ListFiles.
type FileEntry struct {
	Path string
	Size int64
}

// PutFile streams content to path inside the node's confined storage directory.
func (t *GRPCTransport) PutFile(ctx context.Context, path string, content io.Reader, maxBytes int64) error {
	rc, _, err := t.startExecAck(ctx, "", nil, content, execAttach{
		stdin: true,
		file:  &agentpb.FileOp{Kind: agentpb.FileOp_KIND_PUT, Path: path, MaxBytes: maxBytes},
	})
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(io.Discard, rc)
	return err
}

// GetFile streams the object at path; the caller closes it.
func (t *GRPCTransport) GetFile(ctx context.Context, path string) (io.ReadCloser, error) {
	rc, _, err := t.startExecAck(ctx, "", nil, nil, execAttach{
		file: &agentpb.FileOp{Kind: agentpb.FileOp_KIND_GET, Path: path},
	})
	return rc, err
}

// DeleteFile removes path; a missing object is success.
func (t *GRPCTransport) DeleteFile(ctx context.Context, path string) error {
	rc, _, err := t.startExecAck(ctx, "", nil, nil, execAttach{
		file: &agentpb.FileOp{Kind: agentpb.FileOp_KIND_DELETE, Path: path},
	})
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(io.Discard, rc)
	return err
}

// ListFiles returns every object whose path starts with prefix.
func (t *GRPCTransport) ListFiles(ctx context.Context, prefix string) ([]FileEntry, error) {
	rc, _, err := t.startExecAck(ctx, "", nil, nil, execAttach{
		file: &agentpb.FileOp{Kind: agentpb.FileOp_KIND_LIST, Path: prefix},
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	var out []FileEntry
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		size, path, ok := strings.Cut(sc.Text(), "\t")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("agent: list files: bad size %q: %w", size, err)
		}
		out = append(out, FileEntry{Path: path, Size: n})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("agent: list files: %w", err)
	}
	return out, nil
}

// NodeFiles resolves a node ID to its session and runs file operations there.
type NodeFiles struct{ registry *Registry }

// NewNodeFiles builds the control plane side of agent file destinations over registry.
func NewNodeFiles(registry *Registry) *NodeFiles { return &NodeFiles{registry: registry} }

type fileTransport interface {
	PutFile(ctx context.Context, path string, content io.Reader, maxBytes int64) error
	GetFile(ctx context.Context, path string) (io.ReadCloser, error)
	DeleteFile(ctx context.Context, path string) error
	ListFiles(ctx context.Context, prefix string) ([]FileEntry, error)
}

func (n *NodeFiles) transport(nodeID string) (fileTransport, error) {
	tr, err := n.registry.Get(nodeID)
	if err != nil {
		return nil, fmt.Errorf("node %q is not connected: %w", nodeID, err)
	}
	ft, ok := tr.(fileTransport)
	if !ok {
		return nil, fmt.Errorf("node %q is reached through a %T, which has no file storage", nodeID, tr)
	}
	return ft, nil
}

// Put implements backup.NodeFiles.
func (n *NodeFiles) Put(ctx context.Context, nodeID, path string, r io.Reader) error {
	ft, err := n.transport(nodeID)
	if err != nil {
		return err
	}
	return ft.PutFile(ctx, path, r, 0)
}

// Get implements backup.NodeFiles.
func (n *NodeFiles) Get(ctx context.Context, nodeID, path string) (io.ReadCloser, error) {
	ft, err := n.transport(nodeID)
	if err != nil {
		return nil, err
	}
	return ft.GetFile(ctx, path)
}

// Delete implements backup.NodeFiles.
func (n *NodeFiles) Delete(ctx context.Context, nodeID, path string) error {
	ft, err := n.transport(nodeID)
	if err != nil {
		return err
	}
	return ft.DeleteFile(ctx, path)
}

// List implements backup.NodeFiles.
func (n *NodeFiles) List(ctx context.Context, nodeID, prefix string) ([]FileEntry, error) {
	ft, err := n.transport(nodeID)
	if err != nil {
		return nil, err
	}
	return ft.ListFiles(ctx, prefix)
}
