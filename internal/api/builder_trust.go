package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// KeyBuildPersonalInProduction lets personal builders build production environments without an explicit allow-list.
const (
	KeyBuildPersonalInProduction = "build.personal_builders_in_production"
	envBuildPersonalInProduction = "APP_BUILD_PERSONAL_IN_PRODUCTION"

	// KeyBuildPersonalBuilderScope limits which apps a personal builder may receive.
	KeyBuildPersonalBuilderScope = "build.personal_builder_scope"
	envBuildPersonalBuilderScope = "APP_BUILD_PERSONAL_BUILDER_SCOPE"

	// Node labels that record who a personal builder belongs to.
	NodeLabelBuilderOwner = "builder.owner"
	NodeLabelBuilderTeam  = "builder.team"

	maxEnvironmentBuilders = 32
)

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyBuildPersonalBuilderScope, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindString,
		Default: build.ScopeOwner, Env: envBuildPersonalBuilderScope, Apply: runtimesettings.ApplyLive,
		AllowedStrings: []string{build.ScopeOwner, build.ScopeTeam, build.ScopeAll},
		Title:          "Personal builder scope",
		Description:    "Whose apps a personal builder may receive. owner: only apps of teams its owner belongs to (the builder.owner node label holds the user id). team: also apps of the team in its builder.team label. all: any team's non-production apps. A personal builder with no recorded owner receives nothing under owner or team. An environment allow-list that names the builder still decides alone. Read on every build; the environment variable only seeds the default.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyBuildPersonalInProduction, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindBool,
		Default: "false", Env: envBuildPersonalInProduction, Apply: runtimesettings.ApplyLive,
		Title:       "Personal builders may build production",
		Description: "When off, a personal builder (a laptop or home machine) is never chosen for a production environment unless that environment lists it in its builder allow-list. Server and managed builders are unaffected. Read on every build; the environment variable only seeds the default.",
	})
}

// BuilderTrustStore is the node tier and environment allow-list store. *store.DB satisfies it.
type BuilderTrustStore interface {
	SetNodeBuildTrust(ctx context.Context, id, tier string) error
	GetEnvironmentBuilders(ctx context.Context, envID string) ([]string, error)
	SetEnvironmentBuilders(ctx context.Context, envID string, allowed []string, now time.Time) error
}

// nodeBuildTrusts maps node ID to tier, empty when the store cannot report it.
func (rt *Router) nodeBuildTrusts(ctx context.Context) map[string]string {
	l, ok := rt.nodes.(interface {
		ListNodeBuildTrust(ctx context.Context) (map[string]string, error)
	})
	if !ok {
		return nil
	}
	m, err := l.ListNodeBuildTrust(ctx)
	if err != nil {
		return nil
	}
	return m
}

type setNodeBuildTrustRequest struct {
	BuildTrust string `json:"build_trust"`
}

type nodeBuildTrustResource struct {
	ID         string `json:"id"`
	BuildTrust string `json:"build_trust"`
}

// handleSetNodeBuildTrust handles PUT /api/v1/nodes/{id}/build-trust.
func (rt *Router) handleSetNodeBuildTrust(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.nodes.(BuilderTrustStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "builder trust is not available on this control plane")
		return
	}
	id := r.PathValue("id")
	var req setNodeBuildTrustRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if !store.ValidBuildTrust(req.BuildTrust) {
		writeError(w, http.StatusBadRequest, "build_trust must be server, managed or personal")
		return
	}
	if err := s.SetNodeBuildTrust(r.Context(), id, req.BuildTrust); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set node build trust failed", err, slog.String("node_id", id))
		return
	}
	writeJSON(w, http.StatusOK, nodeBuildTrustResource{ID: id, BuildTrust: req.BuildTrust})
}

type environmentBuildersResource struct {
	EnvironmentID   string   `json:"environment_id"`
	Kind            string   `json:"kind"`
	AllowedBuilders []string `json:"allowed_builders"`
	// PersonalAllowed is whether a personal builder may build here under the current policy.
	PersonalAllowed bool `json:"personal_allowed"`
}

type setEnvironmentBuildersRequest struct {
	AllowedBuilders []string `json:"allowed_builders"`
}

func (rt *Router) environmentBuilders(w http.ResponseWriter, r *http.Request) (BuilderTrustStore, store.Environment, bool) {
	s, ok := rt.environments.(BuilderTrustStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "builder allow-lists are not available on this control plane")
		return nil, store.Environment{}, false
	}
	env, err := rt.environments.GetEnvironment(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, "environment not found")
		return nil, store.Environment{}, false
	} else if err != nil {
		rt.internalError(w, "api: load environment for builders failed", err)
		return nil, store.Environment{}, false
	}
	return s, env, true
}

func (rt *Router) environmentBuildersView(ctx context.Context, env store.Environment, allowed []string) environmentBuildersResource {
	if allowed == nil {
		allowed = []string{}
	}
	personal := len(allowed) == 0 && env.Kind != store.EnvironmentKindProduction
	if len(allowed) == 0 && !personal {
		if rs := rt.RuntimeSettings(); rs != nil {
			personal = rs.Bool(ctx, KeyBuildPersonalInProduction)
		}
	}
	for _, a := range allowed {
		personal = personal || a == store.BuildTrustPersonal
	}
	return environmentBuildersResource{EnvironmentID: env.ID, Kind: env.Kind, AllowedBuilders: allowed, PersonalAllowed: personal}
}

// handleGetEnvironmentBuilders handles GET /api/v1/environments/{id}/builders.
func (rt *Router) handleGetEnvironmentBuilders(w http.ResponseWriter, r *http.Request) {
	s, env, ok := rt.environmentBuilders(w, r)
	if !ok {
		return
	}
	allowed, err := s.GetEnvironmentBuilders(r.Context(), env.ID)
	if err != nil {
		rt.internalError(w, "api: get environment builders failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.environmentBuildersView(r.Context(), env, allowed))
}

// handleSetEnvironmentBuilders handles PUT /api/v1/environments/{id}/builders. An empty list restores the default policy.
func (rt *Router) handleSetEnvironmentBuilders(w http.ResponseWriter, r *http.Request) {
	s, env, ok := rt.environmentBuilders(w, r)
	if !ok {
		return
	}
	var req setEnvironmentBuildersRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if len(req.AllowedBuilders) > maxEnvironmentBuilders {
		writeError(w, http.StatusBadRequest, "too many allowed builders")
		return
	}
	resolved := make([]string, 0, len(req.AllowedBuilders))
	seen := map[string]bool{}
	for _, a := range req.AllowedBuilders {
		if !store.ValidBuildTrust(a) {
			id, found := rt.buildRunnerNodeID(r.Context(), a)
			if !found {
				writeError(w, http.StatusBadRequest, "allowed builders are server, managed, personal or a build node")
				return
			}
			a = id
		}
		if !seen[a] {
			seen[a] = true
			resolved = append(resolved, a)
		}
	}
	if err := s.SetEnvironmentBuilders(r.Context(), env.ID, resolved, time.Now()); err != nil {
		rt.internalError(w, "api: set environment builders failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.environmentBuildersView(r.Context(), env, resolved))
}
