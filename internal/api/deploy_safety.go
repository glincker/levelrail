package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DeploySafetyStore is the store surface for deploy ordering, freeze
// windows and digest history. *store.DB satisfies this structurally.
type DeploySafetyStore interface {
	deploy.OrderedStore
	deploy.Sequencer
	deploy.FreezeStore
	deploy.HeldStore
	ReplaceDeployFreezeWindows(ctx context.Context, scope string, windows []store.DeployFreezeWindow) ([]store.DeployFreezeWindow, error)
	SetDeployAttemptReason(ctx context.Context, id, reason string) error
}

// WithDeploySafety enables freeze windows, the stale-deploy guard and
// digest resolution for image deploys. resolver may be nil.
func WithDeploySafety(s DeploySafetyStore, resolver docker.ImageResolver) Option {
	return func(rt *Router) {
		rt.deploySafety = s
		rt.imageResolver = resolver
	}
}

// appImageDigest is the content identity toAppResource reports.
func appImageDigest(svc store.DesiredService) string {
	if d := docker.ImageDigestOf(svc.Image); d != "" {
		return d
	}
	return svc.LocalImageID()
}

// freezeOverride is the manual-deploy escape hatch from a freeze window.
type freezeOverride struct {
	Override bool   `json:"override_freeze,omitempty"`
	Reason   string `json:"override_reason,omitempty"`
}

// freezeGate answers a manual deploy during a freeze: 423 without an
// override, otherwise the note recorded on the attempt. ok false means a
// response was already written.
func (rt *Router) freezeGate(w http.ResponseWriter, r *http.Request, name string, o freezeOverride) (note string, ok bool) {
	if rt.deploySafety == nil {
		return "", true
	}
	status, err := deploy.CheckFreeze(r.Context(), rt.deploySafety, name, time.Now())
	if err != nil {
		rt.logger.Error("api: check deploy freeze failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return "", false
	}
	if !status.Frozen {
		return "", true
	}
	if !o.Override {
		writeError(w, http.StatusLocked, (&deploy.FrozenError{Status: status}).Error()+"; pass override_freeze with override_reason to deploy anyway")
		return "", false
	}
	reason := strings.TrimSpace(o.Reason)
	if reason == "" {
		writeError(w, http.StatusBadRequest, "override_reason is required to deploy during a freeze")
		return "", false
	}
	note = "FreezeOverride: " + reason
	rt.logger.Warn("api: deploy freeze overridden", slog.String("name", name), slog.String("reason", reason), slog.String("window_id", status.WindowID), slog.String("remote_addr", clientIP(r)))
	return note, true
}

// imageTrigger builds the manual image deploy path: digest resolution plus
// ordering, exempt from the stale guard.
func (rt *Router) imageTrigger(ctx context.Context, existing store.DesiredService, pull bool, reason string) deploy.ImageTrigger {
	t := deploy.ImageTrigger{
		Store: deployTriggerImageStore{rt}, Nudger: rt.reconcileNudger, Resolver: rt.imageResolver,
		Logger: rt.logger, Source: store.DeployAttemptSourceImage, RequireFresh: pull, Reason: reason,
	}
	if rt.deploySafety != nil {
		t.Ordered, t.Sequencer = rt.deploySafety, rt.deploySafety
	}
	if existing.RegistryCredentialID != "" && rt.imageResolver != nil {
		if auth, err := rt.registryAuthFor(ctx, existing.RegistryCredentialID); err == nil {
			t.Auth = auth
		} else {
			rt.logger.Warn("api: resolve registry auth for digest lookup failed", slog.String("error", err.Error()), slog.String("name", existing.Name))
		}
	}
	return t
}

func (rt *Router) registryAuthFor(ctx context.Context, credentialID string) (*docker.RegistryAuth, error) {
	if rt.registryCredentialSecrets == nil {
		return nil, errors.New("registry credential secrets are not configured")
	}
	cred, err := rt.registryCredentials.GetRegistryCredential(ctx, credentialID)
	if err != nil {
		return nil, fmt.Errorf("get registry credential: %w", err)
	}
	password, err := rt.registryCredentialSecrets.Resolve(ctx, store.RegistryCredentialSecretsKey(credentialID), "password")
	if err != nil {
		return nil, fmt.Errorf("resolve registry credential password: %w", err)
	}
	return &docker.RegistryAuth{Username: cred.Username, Password: password}, nil
}

// nextOrder allocates an automatic deploy's place in its service's order.
func (rt *Router) nextOrder(ctx context.Context, name, commit, before string, at time.Time) *store.DeployOrder {
	if rt.deploySafety == nil {
		return nil
	}
	seq, err := rt.deploySafety.NextDeploySequence(ctx, name)
	if err != nil {
		rt.logger.Warn("api: allocate deploy sequence failed", slog.String("error", err.Error()), slog.String("name", name))
		return nil
	}
	return &store.DeployOrder{Sequence: seq, Automatic: true, CommitSHA: commit, Before: before, CommitAt: at}
}

// finishSuperseded records a stale deploy as superseded rather than failed.
func (rt *Router) finishSuperseded(ctx context.Context, id string, deployErr error) bool {
	if rt.deploySafety == nil || !errors.Is(deployErr, deploy.ErrSuperseded) {
		return false
	}
	if _, err := rt.deploySafety.TransitionDeployAttempt(ctx, id, store.DeployAttemptStatusRunning, store.DeployAttemptStatusSuperseded, deployErr.Error()); err != nil {
		rt.logger.Error("api: mark deploy attempt superseded failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
	}
	return true
}

// holdIfFrozen parks an automatic git deploy during a freeze window.
func (rt *Router) holdIfFrozen(ctx context.Context, name string, req deploy.HeldRequest, image string) (bool, string) {
	if rt.deploySafety == nil {
		return false, ""
	}
	status, err := deploy.CheckFreeze(ctx, rt.deploySafety, name, time.Now())
	if err != nil || !status.Frozen {
		if err != nil {
			rt.logger.Error("api: check deploy freeze failed", slog.String("error", err.Error()), slog.String("name", name))
		}
		return false, ""
	}
	id, err := deploy.HoldDeploy(ctx, rt.deploySafety, name, image, store.DeployAttemptSourceWebhook, req, status)
	if err != nil {
		rt.logger.Error("api: hold frozen deploy failed", slog.String("error", err.Error()), slog.String("name", name))
		return false, ""
	}
	return true, fmt.Sprintf("held: %s, deploy %s will run when the window ends\n", (&deploy.FrozenError{Status: status}).Error(), id)
}

// ReleaseHandlers replays held deploys; cmd/levelrail wires them into a
// deploy.Releaser.
func (rt *Router) ReleaseHandlers() map[string]deploy.ReleaseHandler {
	return map[string]deploy.ReleaseHandler{
		deploy.HeldKindGit: func(ctx context.Context, a store.DeployAttempt, req deploy.HeldRequest) error {
			gs, err := rt.gitSources.GetGitSource(ctx, a.ServiceName)
			if err != nil {
				return fmt.Errorf("load git source: %w", err)
			}
			order := rt.nextOrder(ctx, a.ServiceName, req.CommitSHA, req.Before, req.CommitAt)
			status, msg := rt.deployFromGitSource(ctx, a.ServiceName, *gs, req.CheckoutRef, req.CommitLabel, order)
			if status >= http.StatusBadRequest {
				return errors.New(strings.TrimSpace(msg))
			}
			return nil
		},
		deploy.HeldKindImage: func(ctx context.Context, a store.DeployAttempt, req deploy.HeldRequest) error {
			existing, err := rt.apps.GetDesiredService(ctx, a.ServiceName)
			if err != nil {
				return fmt.Errorf("load app: %w", err)
			}
			t := rt.imageTrigger(ctx, *existing, false, "Released")
			t.Automatic = true
			_, err = t.Deploy(ctx, *existing, req.Image)
			return err
		},
	}
}

type freezeWindowResource struct {
	ID       string `json:"id,omitempty"`
	Cron     string `json:"cron"`
	Duration string `json:"duration"`
	Timezone string `json:"timezone,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Scope    string `json:"scope,omitempty"`
}

type freezeStatusResource struct {
	Frozen bool       `json:"frozen"`
	Until  *time.Time `json:"until,omitempty"`
	Reason string     `json:"reason,omitempty"`
}

type deployFreezeResource struct {
	Windows []freezeWindowResource `json:"windows"`
	// Inherited are the global windows that also apply to an app.
	Inherited []freezeWindowResource `json:"inherited,omitempty"`
	Status    freezeStatusResource   `json:"status"`
}

type putDeployFreezeRequest struct {
	Windows []freezeWindowResource `json:"windows"`
}

func toFreezeWindowResources(ws []store.DeployFreezeWindow) []freezeWindowResource {
	out := make([]freezeWindowResource, 0, len(ws))
	for _, w := range ws {
		out = append(out, freezeWindowResource{ID: w.ID, Cron: w.Cron, Duration: w.Duration.String(), Timezone: w.Timezone, Reason: w.Reason, Scope: w.Scope})
	}
	return out
}

func deployFreezeView(own, inherited []store.DeployFreezeWindow) deployFreezeResource {
	all := append(append([]store.DeployFreezeWindow{}, own...), inherited...)
	st := deploy.ActiveFreeze(all, time.Now())
	res := deployFreezeResource{Windows: toFreezeWindowResources(own), Status: freezeStatusResource{Frozen: st.Frozen, Reason: st.Reason}}
	if len(inherited) > 0 {
		res.Inherited = toFreezeWindowResources(inherited)
	}
	if st.Frozen {
		until := st.Until.UTC()
		res.Status.Until = &until
	}
	return res
}

func (rt *Router) loadFreeze(ctx context.Context, name string) ([]store.DeployFreezeWindow, []store.DeployFreezeWindow, error) {
	var own []store.DeployFreezeWindow
	if name != "" {
		var err error
		if own, err = rt.deploySafety.ListDeployFreezeWindows(ctx, store.DeployFreezeScopeApp(name)); err != nil {
			return nil, nil, err
		}
	}
	global, err := rt.deploySafety.ListDeployFreezeWindows(ctx, store.DeployFreezeScopeGlobal)
	return own, global, err
}

// handleGetAppDeployFreeze handles GET /api/v1/apps/{name}/deploy-freeze.
func (rt *Router) handleGetAppDeployFreeze(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.freezeApp(w, r)
	if !ok {
		return
	}
	own, global, err := rt.loadFreeze(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: get deploy freeze failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, deployFreezeView(own, global))
}

// handlePutAppDeployFreeze handles PUT /api/v1/apps/{name}/deploy-freeze:
// replaces the app's windows; an empty list clears them.
func (rt *Router) handlePutAppDeployFreeze(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.freezeApp(w, r)
	if !ok {
		return
	}
	rt.putFreeze(w, r, store.DeployFreezeScopeApp(name), name)
}

// handleGetGlobalDeployFreeze handles GET /api/v1/settings/deploy-freeze.
func (rt *Router) handleGetGlobalDeployFreeze(w http.ResponseWriter, r *http.Request) {
	if rt.deploySafety == nil {
		writeError(w, http.StatusNotImplemented, "deploy freeze windows are not configured on this control plane")
		return
	}
	_, global, err := rt.loadFreeze(r.Context(), "")
	if err != nil {
		rt.logger.Error("api: get global deploy freeze failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, deployFreezeView(global, nil))
}

// handlePutGlobalDeployFreeze handles PUT /api/v1/settings/deploy-freeze.
func (rt *Router) handlePutGlobalDeployFreeze(w http.ResponseWriter, r *http.Request) {
	if rt.deploySafety == nil {
		writeError(w, http.StatusNotImplemented, "deploy freeze windows are not configured on this control plane")
		return
	}
	rt.putFreeze(w, r, store.DeployFreezeScopeGlobal, "")
}

func (rt *Router) freezeApp(w http.ResponseWriter, r *http.Request) (string, bool) {
	if rt.deploySafety == nil {
		writeError(w, http.StatusNotImplemented, "deploy freeze windows are not configured on this control plane")
		return "", false
	}
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return "", false
		}
		rt.logger.Error("api: deploy freeze: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return "", false
	}
	return name, true
}

func (rt *Router) putFreeze(w http.ResponseWriter, r *http.Request, scope, name string) {
	var req putDeployFreezeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	windows := make([]store.DeployFreezeWindow, 0, len(req.Windows))
	for i, in := range req.Windows {
		d, err := time.ParseDuration(in.Duration)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("windows[%d].duration: %v", i, err))
			return
		}
		win := store.DeployFreezeWindow{Cron: strings.TrimSpace(in.Cron), Duration: d, Timezone: strings.TrimSpace(in.Timezone), Reason: strings.TrimSpace(in.Reason)}
		if err := deploy.ValidateFreezeWindow(win); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("windows[%d]: %v", i, err))
			return
		}
		windows = append(windows, win)
	}
	if _, err := rt.deploySafety.ReplaceDeployFreezeWindows(r.Context(), scope, windows); err != nil {
		rt.logger.Error("api: save deploy freeze failed", slog.String("error", err.Error()), slog.String("scope", scope))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	own, global, err := rt.loadFreeze(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: reload deploy freeze failed", slog.String("error", err.Error()), slog.String("scope", scope))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if name == "" {
		own, global = global, nil
	}
	writeJSON(w, http.StatusOK, deployFreezeView(own, global))
}
