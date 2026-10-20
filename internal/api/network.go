package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// dockerInspectTimeout bounds one live container-state check
// (docker.Runtime.InspectByName) issued directly from an HTTP handler:
// network.go, exec.go, and resources_live_apply.go all read the request
// context, which by default carries no deadline of its own, so a
// blackholed or overloaded Docker daemon would otherwise hang the whole
// request (and, for network.go, the entire Network tab's page load)
// indefinitely instead of the page degrading to "status unknown."
const dockerInspectTimeout = 3 * time.Second

// networkResource is GET /api/v1/apps/{name}/network's wire shape: the
// live traffic path from Caddy ingress down to the container, for the
// frontend's Network tab. HostPort is only ever meaningful while Running
// is true, matching docker.ContainerState.Ports' own "empty until the
// container is actually running" contract.
type networkResource struct {
	ContainerPort int  `json:"container_port"`
	HostPort      int  `json:"host_port,omitempty"`
	Running       bool `json:"running"`
	// RolloutInProgress is true when HostPort and Running describe an older
	// container still serving while the desired release is not live yet.
	RolloutInProgress bool `json:"rollout_in_progress,omitempty"`
	// FallbackURL is the zero-config sslip.io URL this app is reachable
	// at when it has no operator-configured domain (svc.Domains empty)
	// and this control plane's own APP_PUBLIC_HOST is a real, publicly
	// routable IP: the same host internal/reconcile/ingress's own
	// Reconcile pass routes it under, see ingress.FallbackDomain's own
	// doc comment for exactly when this is empty instead. Omitted
	// entirely (not just empty) when the app has a real domain or no
	// fallback is available, so the frontend can render this field's
	// mere presence as "show a copyable public URL."
	FallbackURL string `json:"fallback_url,omitempty"`
	// FallbackEnabled reports the platform-wide automatic hostname toggle,
	// so the UI can offer to turn it back on when FallbackURL is absent.
	FallbackEnabled bool `json:"fallback_enabled"`
	// PublicHost is the server address the fallback hostname is built from.
	PublicHost string `json:"public_host,omitempty"`
}

// handleGetAppNetwork handles GET /api/v1/apps/{name}/network. Host port
// is resolved live via execRuntime, never persisted: it changes on every
// container recreate. Degrades to HostPort 0 / Running false rather than
// an error when execRuntime is nil or the lookup fails, same as
// handleListImages.
func (rt *Router) handleGetAppNetwork(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get app network: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := networkResource{ContainerPort: svc.Port, PublicHost: rt.publicHost, FallbackEnabled: true}
	ing, ierr := rt.ingressSettings.GetIngressSettings(r.Context())
	if ierr == nil {
		resp.FallbackEnabled = !ing.FallbackDomainsDisabled
	}
	if len(svc.Domains) == 0 && resp.FallbackEnabled {
		if fallback, ok := rt.autoHost(r.Context(), svc.Name); ok {
			resp.FallbackURL = rt.ingressHTTPSURL(ing, fallback)
		}
	}

	if rt.execRuntime == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	rt2, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		rt.logger.Warn("api: get app network: resolve node runtime failed",
			slog.String("error", err.Error()), slog.String("name", name), slog.String("node_id", svc.NodeID))
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Same target-container derivation handleExecApp and
	// internal/reconcile/ingress already use to find a service's
	// currently active container from its own desired state.
	target := application.ContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce)
	inspectCtx, cancel := context.WithTimeout(r.Context(), dockerInspectTimeout)
	state, err := rt2.InspectByName(inspectCtx, target)
	cancel()
	// A target that is up but not confirmed serving (still rolling out, or
	// failing its Ready condition) must not be advertised while an older
	// replica serves.
	targetUp := err == nil && state != nil && state.Running && len(state.Ports) > 0
	if targetUp && rt.candidateUnconfirmed(r.Context(), svc) {
		if serving := rt.servingReplica(r.Context(), rt2, svc.Name, svc.Port, target); serving != nil {
			resp.Running = true
			resp.HostPort = serving.hostPort
			resp.RolloutInProgress = true
			writeJSON(w, http.StatusOK, resp)
			return
		}
	}
	if targetUp {
		resp.Running = true
		resp.HostPort = state.Ports[0].HostPort
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if serving := rt.servingReplica(r.Context(), rt2, svc.Name, svc.Port, target); serving != nil {
		resp.Running = true
		resp.HostPort = serving.hostPort
		resp.RolloutInProgress = true
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if err == nil && state != nil {
		resp.Running = state.Running
	}
	writeJSON(w, http.StatusOK, resp)
}

var extraReplicaName = regexp.MustCompile(`-r[0-9]+$`)

// candidateUnconfirmed reports a desired container the controller has not
// confirmed serving: its attempt is not yet marked serving, or Ready is False.
func (rt *Router) candidateUnconfirmed(ctx context.Context, svc *store.DesiredService) bool {
	if _, rolling := rt.lastServingImage(ctx, *svc); rolling {
		return true
	}
	if rt.deploys == nil {
		return false
	}
	reason, _ := rt.failingReadyCondition(ctx, svc.Name)
	return reason != ""
}

type servingReplica struct{ hostPort int }

// servingReplica finds an older release's replica 0 still publishing the
// app's port while the desired container is not live, so a redeploy never
// reads as downtime. nil when nothing else is serving.
func (rt *Router) servingReplica(ctx context.Context, runtime docker.Runtime, app string, port int, skip string) *servingReplica {
	lctx, cancel := context.WithTimeout(ctx, dockerInspectTimeout)
	defer cancel()
	list, err := runtime.ListByPrefix(lctx, app+"-")
	if err != nil {
		return nil
	}
	var best *servingReplica
	var bestCreated time.Time
	for _, c := range list {
		if c.Name == skip || !c.Running || strings.HasSuffix(c.Name, "-egress") || extraReplicaName.MatchString(c.Name) {
			continue
		}
		if owner, ok := application.ParseContainerName(c.Name); !ok || owner != app {
			continue
		}
		for _, p := range c.Ports {
			if p.ContainerPort == port && p.HostPort > 0 && (best == nil || c.Created.After(bestCreated)) {
				best, bestCreated = &servingReplica{hostPort: p.HostPort}, c.Created
				break
			}
		}
	}
	return best
}
