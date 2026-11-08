package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// handleDeleteApp handles DELETE /api/v1/apps/{name}. 204 means containers are
// gone; 202 means desired state is deleted but teardown is retried by a
// tombstone (application.DeleteFinalizer). An emptied store.App row is removed too.
func (rt *Router) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if rt.retainedDeployServices(r.Context())[name] {
		writeError(w, http.StatusConflict, "this is a retained copy of an earlier deploy that the platform manages; it is removed automatically")
		return
	}
	purge, volumes, ok := rt.appDeleteDataPlan(w, r, name)
	if !ok {
		return
	}
	teardownErr, err := rt.deleteApp(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete app failed", err, slog.String("name", name))
		return
	}
	if purge && teardownErr == nil {
		rt.purgeVolumes(context.WithoutCancel(r.Context()), "app/"+name, volumes)
	}
	rt.nudgeReconciler()
	if teardownErr != nil {
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":  "teardown_pending",
			"message": "app deleted, but its containers are not removed yet; teardown retries automatically",
			"error":   teardownErr.Error(),
		})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// appNameLocks serializes a delete's row removal and credential erase with a
// create's secret writes for the same name. Entries are dropped when unused.
var appNameLocks = struct {
	mu sync.Mutex
	m  map[string]*appNameLock
}{m: map[string]*appNameLock{}}

type appNameLock struct {
	mu   sync.Mutex
	refs int
}

// lockAppName blocks until name is free and returns the release func.
func lockAppName(name string) func() {
	appNameLocks.mu.Lock()
	l := appNameLocks.m[name]
	if l == nil {
		l = &appNameLock{}
		appNameLocks.m[name] = l
	}
	l.refs++
	appNameLocks.mu.Unlock()
	l.mu.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Unlock()
			appNameLocks.mu.Lock()
			if l.refs--; l.refs == 0 {
				delete(appNameLocks.m, name)
			}
			appNameLocks.mu.Unlock()
		})
	}
}

// deleteApp removes name's desired state, tears its containers down and
// drops its App row if it was the last member. A tombstone is written first
// so a failed teardown (second return) is retried by the reconciler. Callers
// nudge the reconciler afterwards.
func (rt *Router) deleteApp(ctx context.Context, name string) (teardownErr, err error) {
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("load app %q: %w", name, err)
	}
	finalizer := rt.deleteFinalizer()
	if finalizer != nil {
		if err := finalizer.Begin(ctx, name, existing.NodeID); err != nil {
			return nil, err
		}
	}
	// Integration rows cascade with the service, so list them first.
	integrationNS := rt.appIntegrationSecretNamespaces(ctx, name)
	// Erase right after the row is gone, under the name lock: the row is
	// already deleted so a failure loses nothing, and a same-name create
	// cannot write secrets that this cleanup then removes.
	unlock := lockAppName(name)
	defer unlock()
	if err := rt.apps.DeleteDesiredService(ctx, name); err != nil {
		return nil, fmt.Errorf("delete app %q: %w", name, err)
	}
	rt.removeAppCredentials(ctx, name, integrationNS)
	if vdb, ok := rt.vitalsDB(); ok {
		if err := rt.vitalsRuntime().dropApp(ctx, vdb, name); err != nil {
			rt.logger.Warn("api: delete app: drop buffered vitals failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	unlock()
	rt.dropMailCertSecret(ctx, *existing)
	if err := rt.removeStaticSite(ctx, name); err != nil {
		rt.logger.Warn("api: delete app: remove static site failed", slog.String("error", err.Error()), slog.String("name", name))
	}
	if ds, ok := rt.ingressSettings.(dnsProviderStore); ok {
		if err := ds.SetAppDNSProvider(ctx, name, store.DNSProviderChoice{}); err != nil {
			rt.logger.Warn("api: delete app: clear dns provider override failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	// Nudge before the slow teardown so the ingress route drops right away.
	rt.nudgeReconciler()
	if finalizer != nil {
		tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rt.deleteTeardownTimeout())
		teardownErr = finalizer.Finalize(tctx, name, existing.NodeID)
		cancel()
		if teardownErr != nil {
			rt.logger.Error("api: delete app: teardown pending", slog.String("error", teardownErr.Error()), slog.String("name", name))
		}
	}
	rt.removeAppArtifactsUnlessReused(ctx, name, existing.NodeID)
	if rt.canaries != nil && !store.IsCanaryService(name) {
		if _, ok, err := rt.canaries.GetCanaryRelease(ctx, name); err == nil && ok {
			if err := rt.removeCanary(ctx, name); err != nil {
				rt.logger.Warn("api: delete app: remove canary failed", slog.String("error", err.Error()), slog.String("name", name))
			}
		}
	}
	if rt.preview != nil {
		if err := rt.preview.DeleteApp(ctx, name); err != nil {
			rt.logger.Warn("api: delete app: remove previews failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	if rt.supplyChain != nil {
		if err := rt.supplyChain.DeleteApp(ctx, name); err != nil {
			rt.logger.Warn("api: delete app: remove supply chain data failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	if rt.secretScan != nil {
		if err := rt.secretScan.DeleteApp(ctx, name); err != nil {
			rt.logger.Warn("api: delete app: remove secret scan data failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	if rt.ciTrusts != nil {
		if err := rt.ciTrusts.DeleteCIOIDCTrustsForApp(ctx, name); err != nil {
			rt.logger.Warn("api: delete app: remove ci trusts failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	if rt.buildHooks != nil {
		if err := rt.buildHooks.DeleteBuildHooksForService(ctx, name); err != nil {
			rt.logger.Warn("api: delete app: remove build hooks failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	if rt.imageAutoUpdates != nil {
		if err := rt.imageAutoUpdates.DeleteImageAutoUpdatesForApp(ctx, name); err != nil {
			rt.logger.Warn("api: delete app: remove image auto update failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	if existing.AppID != "" {
		// Name lock covers the window where a same-name create has saved its row but not yet linked it.
		unlockSweep := lockAppName(name)
		orphaned := rt.deleteAppIfOrphaned(ctx, existing.AppID)
		unlockSweep()
		if orphaned && finalizer != nil && teardownErr == nil {
			nctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rt.deleteTeardownTimeout())
			if err := finalizer.RemoveAppNetwork(nctx, existing.AppID, existing.NodeID); err != nil {
				rt.logger.Warn("api: delete app: remove app network failed, GC will retry", slog.String("error", err.Error()), slog.String("app_id", existing.AppID))
			}
			cancel()
		}
	}
	return teardownErr, nil
}

// removeAppCredentials erases the app's stored secrets, git source with its
// deploy token and webhook secret, and other access grants, so a later app
// of the same name inherits none of them. Failures are logged only.
// extraNamespaces are secrets namespaces the caller collected earlier.
func (rt *Router) removeAppCredentials(ctx context.Context, name string, extraNamespaces []string) {
	namespaces := append([]string(nil), extraNamespaces...)
	if g, ok := rt.apps.(interface {
		DeleteAppAccessGrants(ctx context.Context, name string) ([]string, error)
	}); ok {
		ns, err := g.DeleteAppAccessGrants(ctx, name)
		if err != nil {
			rt.logger.Warn("api: delete app: remove access grants failed", slog.String("error", err.Error()), slog.String("name", name))
		}
		namespaces = append(namespaces, ns...)
	}
	if rt.gitSources != nil {
		if err := rt.gitSources.DeleteGitSource(ctx, name); err != nil && !errors.Is(err, store.ErrGitSourceNotFound) {
			rt.logger.Warn("api: delete app: remove git source failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	if rt.secrets == nil {
		return
	}
	namespaces = append(namespaces, name)
	for _, ns := range namespaces {
		if err := rt.secrets.DeleteAll(ctx, ns); err != nil {
			rt.logger.Warn("api: delete app: remove secrets failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	rt.eraseGitSourceSecrets(ctx, name)
}

// eraseGitSourceSecrets drops a git source's deploy token and webhook
// secret. Failures are logged only.
func (rt *Router) eraseGitSourceSecrets(ctx context.Context, name string) {
	if rt.secrets == nil {
		return
	}
	if err := rt.secrets.DeleteAll(ctx, store.GitSourceSecretsKey(name)); err != nil {
		rt.logger.Warn("api: remove git source secrets failed", slog.String("error", err.Error()), slog.String("name", name))
	}
}

// appIntegrationSecretNamespaces lists the secrets namespaces of name's
// attached integrations, empty when none or on a read failure.
func (rt *Router) appIntegrationSecretNamespaces(ctx context.Context, name string) []string {
	if rt.appIntegrations == nil {
		return nil
	}
	rows, err := rt.appIntegrations.ListAppIntegrationsForService(ctx, name)
	if err != nil {
		rt.logger.Warn("api: delete app: list integrations failed", slog.String("error", err.Error()), slog.String("name", name))
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, ai := range rows {
		out = append(out, store.AppIntegrationSecretsKey(ai.ServiceName, ai.IntegrationKey))
	}
	return out
}

// deleteFinalizer is nil when no runtime resolver or tombstone-capable
// store is configured.
func (rt *Router) deleteFinalizer() *application.DeleteFinalizer {
	if rt.execRuntime == nil {
		return nil
	}
	ts, ok := rt.apps.(application.TeardownStore)
	if !ok {
		return nil
	}
	return application.NewDeleteFinalizer(ts, application.RuntimeResolver(rt.execRuntime), rt.logger, rt.teardownOpts...)
}

func (rt *Router) deleteTeardownTimeout() time.Duration {
	if rt.deleteTimeout > 0 {
		return rt.deleteTimeout
	}
	return defaultDeleteTeardownTimeout
}

// defaultDeleteTeardownTimeout bounds how long DELETE waits for containers to stop.
const defaultDeleteTeardownTimeout = 45 * time.Second

// teardownServiceContainers removes name's containers on nodeID after its
// desired state was deleted or moved off that node. A tombstone makes a failed
// teardown retry on every reconcile pass instead of being lost, and the slow
// stop runs in the background so the caller's response does not wait on it.
func (rt *Router) teardownServiceContainers(name, nodeID string) {
	finalizer := rt.deleteFinalizer()
	if finalizer == nil {
		return
	}
	if err := finalizer.Begin(context.Background(), name, nodeID); err != nil {
		rt.logger.Error("api: teardown containers: record tombstone failed", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	go func() { //nolint:gosec // deliberately outlives the request, same as sendInviteEmail's own background send
		ctx, cancel := context.WithTimeout(context.Background(), rt.deleteTeardownTimeout())
		defer cancel()
		if err := finalizer.Finalize(ctx, name, nodeID); err != nil {
			rt.logger.Error("api: teardown containers pending, will retry", slog.String("error", err.Error()), slog.String("name", name), slog.String("node_id", nodeID))
		}
	}()
}

// deleteAppIfOrphaned deletes the store.App row identified by appID once
// it has no remaining member services. Best-effort and logged, not
// surfaced as handleDeleteApp's own error: the service delete above
// already succeeded, and a stale App row (or, worst case, a network
// NetworkCleanupController doesn't yet know to remove) is a lesser,
// self-healing problem, not one that should make an otherwise-successful
// delete request fail.
func (rt *Router) deleteAppIfOrphaned(ctx context.Context, appID string) bool {
	deleted, err := rt.appGroups.DeleteAppIfOrphaned(ctx, appID)
	if err != nil {
		rt.logger.Error("api: delete app: delete orphaned app row failed", slog.String("error", err.Error()), slog.String("app_id", appID))
		return false
	}
	return deleted
}

// reservedSecretNamespace reports names the control plane seals its own settings under; an app with
// one would share that secret namespace.
func reservedSecretNamespace(name string) bool {
	switch name {
	case store.EmailSettingsSecretsKey(), sysEmailSecretsNS, store.VaultSecretsKey(), store.AIAssistantSecretsKey(),
		store.CloudflareTunnelSecretsKey(), store.CloudflareDNSSecretsKey(), store.GoogleCloudDNSSecretsKey(),
		store.RegistrySettingsSecretsKey(), store.Route53DNSSecretsKey():
		return true
	}
	return false
}
