package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"

	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// LocalContainerLister is implemented by the real Docker client. A runtime
// that is not one cannot discover local source containers.
type LocalContainerLister interface {
	ListLocalContainers(ctx context.Context) ([]docker.LocalContainer, error)
}

func (rt *Router) localSources(ctx context.Context, runtime docker.Runtime) ([]datamigrate.LocalSource, error) {
	lister, ok := runtime.(LocalContainerLister)
	if !ok {
		return nil, errors.New("this node cannot list its containers")
	}
	all, err := lister.ListLocalContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	out := make([]datamigrate.LocalSource, 0)
	for _, c := range all {
		if datamigrate.EngineFromImage(c.Image) == "" {
			continue
		}
		out = append(out, datamigrate.SelectLocalSource(c))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Container < out[j].Container })
	return out, nil
}

func (rt *Router) resolveLocalSource(ctx context.Context, runtime docker.Runtime, name string) (datamigrate.LocalSource, error) {
	list, err := rt.localSources(ctx, runtime)
	if err != nil {
		return datamigrate.LocalSource{}, err
	}
	for _, s := range list {
		if s.Container != name {
			continue
		}
		if s.Problem != "" {
			return datamigrate.LocalSource{}, fmt.Errorf("container %q cannot be used: %s", name, s.Problem)
		}
		return s, nil
	}
	return datamigrate.LocalSource{}, fmt.Errorf("no database container named %q on this node", name)
}

// handleHubLocalSources handles GET /api/v1/migration/hub/local-sources: the
// database containers on a node's Docker daemon, each with the network the
// helper must join to reach it.
func (rt *Router) handleHubLocalSources(w http.ResponseWriter, r *http.Request) {
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "migrating a server is not available on this control plane")
		return
	}
	nodeID := r.URL.Query().Get("node_id")
	runtime, err := rt.execRuntime(nodeID)
	if err != nil {
		rt.logger.Error("api: migration hub: resolve node runtime failed", slog.String("error", err.Error()), slog.String("node_id", nodeID))
		writeError(w, http.StatusBadGateway, "the node is not currently reachable")
		return
	}
	list, err := rt.localSources(r.Context(), runtime)
	if err != nil {
		writeJSON(w, http.StatusOK, []datamigrate.LocalSource{})
		return
	}
	writeJSON(w, http.StatusOK, list)
}
