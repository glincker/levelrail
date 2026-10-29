package main

import (
	"context"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/backup"
)

type backupNodeFiles struct{ files *agent.NodeFiles }

func (b backupNodeFiles) Put(ctx context.Context, nodeID, path string, r io.Reader) error {
	return b.files.Put(ctx, nodeID, path, r)
}

func (b backupNodeFiles) Get(ctx context.Context, nodeID, path string) (io.ReadCloser, error) {
	return b.files.Get(ctx, nodeID, path)
}

func (b backupNodeFiles) Delete(ctx context.Context, nodeID, path string) error {
	return b.files.Delete(ctx, nodeID, path)
}

func (b backupNodeFiles) List(ctx context.Context, nodeID, prefix string) ([]backup.RemoteObject, error) {
	entries, err := b.files.List(ctx, nodeID, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]backup.RemoteObject, len(entries))
	for i, e := range entries {
		out[i] = backup.RemoteObject{Key: e.Path, Size: e.Size}
	}
	return out, nil
}

// routedStorage sends S3 destinations to the S3 clients and agent node destinations over registry's sessions.
func routedStorage(registry *agent.Registry) backup.Routed {
	return backup.Routed{Files: backupNodeFiles{files: agent.NewNodeFiles(registry)}, LocalRoot: backup.LocalRootFromEnv(os.LookupEnv)}
}
