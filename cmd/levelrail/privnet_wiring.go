package main

import (
	"context"

	"github.com/glincker/levelrail/internal/api"
	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/privnet"
	"github.com/glincker/levelrail/internal/store"
)

func privateNetworkConfig(db *store.DB, rt docker.Runtime, prefix, instanceID string) api.PrivateNetworkConfig {
	return api.PrivateNetworkConfig{
		Store:    db,
		Enforcer: privnet.NewController(db, rt, prefix, instanceID),
		Prefix:   prefix,
		Containers: func(ctx context.Context) ([]docker.ContainerState, error) {
			return rt.ListByPrefix(ctx, "")
		},
		Nodes: func(ctx context.Context) (map[string]string, error) {
			svcs, err := db.ListDesiredServices(ctx)
			if err != nil {
				return nil, err
			}
			out := make(map[string]string, len(svcs))
			for _, s := range svcs {
				if s.AppID != "" {
					out[s.AppID] = s.NodeID
				}
			}
			return out, nil
		},
	}
}

func privateNetworkHiddenApps(db *store.DB) func(context.Context) (map[string]bool, error) {
	return func(ctx context.Context) (map[string]bool, error) {
		nets, rules, _, err := privnet.Load(ctx, db)
		if err != nil {
			return nil, err
		}
		return privnet.MeshHidden(nets, rules), nil
	}
}
