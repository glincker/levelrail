package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// AgentEndpointScheme marks a destination whose bytes live in an enrolled node's confined storage directory instead of an S3 bucket.
const AgentEndpointScheme = store.AgentEndpointScheme

// AgentEndpoint returns the destination endpoint that points at nodeID.
func AgentEndpoint(nodeID string) string { return AgentEndpointScheme + nodeID }

// NodeFiles is the control plane view of a node's confined storage directory, reached over the existing agent session.
type NodeFiles interface {
	Put(ctx context.Context, nodeID, path string, r io.Reader) error
	Get(ctx context.Context, nodeID, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, nodeID, path string) error
	List(ctx context.Context, nodeID, prefix string) ([]RemoteObject, error)
}

// ErrNodeFilesUnavailable is returned when an agent destination is used but no node transport is wired.
var ErrNodeFilesUnavailable = errors.New("backup: agent node destinations are not available on this control plane")

// ErrAgentDestinationUnsupported is returned by flows that only speak S3 (WAL shipping, base backups, point in time restore).
var ErrAgentDestinationUnsupported = errors.New("backup: this feature needs an S3 compatible destination, not an agent node directory")

func (d Destination) agentNode() (string, bool) {
	if !strings.HasPrefix(d.Endpoint, AgentEndpointScheme) {
		return "", false
	}
	return strings.TrimPrefix(d.Endpoint, AgentEndpointScheme), true
}

// IsAgent reports whether d is an agent node directory destination.
func (d Destination) IsAgent() bool {
	_, ok := d.agentNode()
	return ok
}

func (d Destination) agentPath(key string) string {
	return path.Join(d.Bucket, key)
}

// Routed sends operations for an agent destination to the node and everything else to the S3 implementations.
type Routed struct {
	Files NodeFiles
	// LocalRoot confines local:// targets; empty disables them.
	LocalRoot string
}

func (r Routed) files(d Destination) (NodeFiles, string, error) {
	node, _ := d.agentNode()
	if r.Files == nil {
		return nil, "", ErrNodeFilesUnavailable
	}
	return r.Files, node, nil
}

// Upload implements Uploader.
func (r Routed) Upload(ctx context.Context, dest Destination, key string, rd io.Reader, size int64) error {
	if dest.IsLocal() {
		return r.localUpload(dest, key, rd)
	}
	if !dest.IsAgent() {
		return S3Uploader{}.Upload(ctx, dest, key, rd, size)
	}
	f, node, err := r.files(dest)
	if err != nil {
		return err
	}
	if err := f.Put(ctx, node, dest.agentPath(key), rd); err != nil {
		return fmt.Errorf("backup: upload to node %q: %w", node, err)
	}
	return nil
}

// Download implements Downloader.
func (r Routed) Download(ctx context.Context, dest Destination, key string) (io.ReadCloser, error) {
	if dest.IsLocal() {
		return r.localDownload(dest, key)
	}
	if !dest.IsAgent() {
		return S3Downloader{}.Download(ctx, dest, key)
	}
	f, node, err := r.files(dest)
	if err != nil {
		return nil, err
	}
	rc, err := f.Get(ctx, node, dest.agentPath(key))
	if err != nil {
		return nil, fmt.Errorf("backup: download from node %q: %w", node, err)
	}
	return rc, nil
}

// Delete implements Deleter.
func (r Routed) Delete(ctx context.Context, dest Destination, key string) error {
	if dest.IsLocal() {
		return r.localDelete(dest, key)
	}
	if !dest.IsAgent() {
		return S3Deleter{}.Delete(ctx, dest, key)
	}
	f, node, err := r.files(dest)
	if err != nil {
		return err
	}
	if err := f.Delete(ctx, node, dest.agentPath(key)); err != nil {
		return fmt.Errorf("backup: delete on node %q: %w", node, err)
	}
	return nil
}

// List implements Lister; keys come back relative to the destination directory, like S3 keys.
func (r Routed) List(ctx context.Context, dest Destination, prefix string) ([]RemoteObject, error) {
	if dest.IsLocal() {
		return r.localList(dest, prefix)
	}
	if !dest.IsAgent() {
		return S3Lister{}.List(ctx, dest, prefix)
	}
	f, node, err := r.files(dest)
	if err != nil {
		return nil, err
	}
	base := dest.agentPath("") + "/"
	objs, err := f.List(ctx, node, base+prefix)
	if err != nil {
		return nil, fmt.Errorf("backup: list on node %q: %w", node, err)
	}
	for i := range objs {
		objs[i].Key = strings.TrimPrefix(objs[i].Key, base)
	}
	return objs, nil
}

// Test implements Tester with a write, read back and delete of a tiny object.
func (r Routed) Test(ctx context.Context, dest Destination) error {
	if dest.IsLocal() {
		return r.localTest(dest)
	}
	if !dest.IsAgent() {
		return S3Tester{}.Test(ctx, dest)
	}
	f, node, err := r.files(dest)
	if err != nil {
		return err
	}
	probe := dest.agentPath(".probe-" + node)
	if err := f.Put(ctx, node, probe, strings.NewReader("ok")); err != nil {
		return fmt.Errorf("backup: test node %q: %w", node, err)
	}
	rc, err := f.Get(ctx, node, probe)
	if err != nil {
		return fmt.Errorf("backup: test node %q: %w", node, err)
	}
	got, readErr := io.ReadAll(rc)
	_ = rc.Close()
	if readErr != nil || string(got) != "ok" {
		return fmt.Errorf("backup: test node %q: object read back did not match what was written", node)
	}
	if err := f.Delete(ctx, node, probe); err != nil {
		return fmt.Errorf("backup: test node %q: %w", node, err)
	}
	return nil
}
