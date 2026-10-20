package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/orphans"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newOrphanReaper builds the reaper shared by the reconcile loop and the API.
// Remote nodes are scanned for containers through their agents; volumes are
// only scanned on the control plane's own node.
func newOrphanReaper(db *store.DB, local *docker.Client, registry *agent.Registry, localNodeID, instanceID string, logger *slog.Logger) *orphans.Reaper {
	normalize := func(id string) string {
		if id == "" || id == localNodeID {
			return ""
		}
		return id
	}
	return orphans.New(orphans.Deps{
		Store: db,
		NodeIDs: func(ctx context.Context) ([]string, error) {
			nodes, err := db.ListNodes(ctx)
			if err != nil {
				return nil, fmt.Errorf("list nodes: %w", err)
			}
			ids := []string{""}
			for _, n := range nodes {
				if normalize(n.ID) != "" {
					ids = append(ids, n.ID)
				}
			}
			return ids, nil
		},
		NormNode: normalize,
		Resolve: func(nodeID string) (docker.Runtime, error) {
			return resolveNodeTransport(local, registry, nodeID)
		},
		Volumes:     local,
		Certs:       db,
		ServedHosts: db.ServedHosts,
		InstanceID:  instanceID,
		Config:      orphans.ConfigFromEnv(os.LookupEnv),
		Logger:      logger,
	})
}
