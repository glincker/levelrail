package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/mail/mox"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Mail engine lookup failures. Every one fails closed: no admin call is made.
var (
	errMailEngineMissing   = errors.New("mail: the mail container is not running")
	errMailEngineAmbiguous = errors.New("mail: more than one container claims the mail engine identity")
	errMailEngineForeign   = errors.New("mail: the mail container does not carry this instance's label")
	errMailEngineNotMail   = errors.New("mail: the service is not a mail-mox install")
)

const (
	mailTemplateID     = "mail-mox"
	mailSelfSignedCert = "certs/selfsigned.pem"
)

// mailStatusResponse carries no credentials or certificate material.
type mailStatusResponse struct {
	App            string `json:"app"`
	Installed      bool   `json:"installed"`
	ContainerState string `json:"container_state"`
	AdminReachable bool   `json:"admin_reachable"`
	TLS            string `json:"tls"`
	NextStep       string `json:"next_step"`
	// Domain and Hostname come from the install env so the DNS form can prefill.
	Domain   string `json:"domain,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

// mailEnvValue returns an env value, or "" when it is unset or still a template placeholder.
func mailEnvValue(env map[string]string, key string) string {
	v := strings.TrimSpace(env[key])
	if strings.HasPrefix(v, "${") {
		return ""
	}
	return v
}

// mailNames derives the domain and mail host from the install env.
func mailNames(env map[string]string) (domain, host string) {
	domain = strings.ToLower(mailEnvValue(env, "MAIL_FIRST_DOMAIN"))
	host = strings.ToLower(mailEnvValue(env, "MAIL_HOSTNAME"))
	if host == "" && domain != "" {
		host = "mail." + domain
	}
	return domain, host
}

// mailTLSKind reads the kind from the install env: an empty value or the
// default path means the engine made its own self-signed pair.
func mailTLSKind(env map[string]string) string {
	f := env["MAIL_TLS_CERT_FILE"]
	if f == "" || f == mailSelfSignedCert || strings.HasPrefix(f, "${") {
		return "self-signed"
	}
	return "provided"
}

// mailRank orders candidates: running with admin reachable, then running, then the rest.
func mailRank(s mailStatusResponse) int {
	switch {
	case s.ContainerState == "running" && s.AdminReachable:
		return 2
	case s.ContainerState == "running":
		return 1
	}
	return 0
}

// mailEngineContainer finds the one container the reconciler runs for the
// mail service: its exact content-hashed name from desired state plus this
// instance's reserved label. Name prefixes and tenant-settable labels never
// count, so another app cannot stand in for the engine.
func (rt *Router) mailEngineContainer(ctx context.Context, svc store.DesiredService) (docker.ContainerState, error) {
	if !rt.isInstanceMailApp(ctx, svc) {
		return docker.ContainerState{}, errMailEngineNotMail
	}
	if rt.containers == nil {
		return docker.ContainerState{}, errMailEngineMissing
	}
	want := application.ContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce)
	list, err := rt.containers.ListByPrefix(ctx, want)
	if err != nil {
		return docker.ContainerState{}, err
	}
	var hits []docker.ContainerState
	for _, c := range list {
		if c.Name == want {
			hits = append(hits, c)
		}
	}
	switch {
	case len(hits) == 0:
		return docker.ContainerState{}, errMailEngineMissing
	case len(hits) > 1:
		return docker.ContainerState{}, errMailEngineAmbiguous
	}
	if rt.instance != nil {
		info, err := rt.instanceInfo(ctx)
		if err != nil {
			return docker.ContainerState{}, fmt.Errorf("mail: read instance identity: %w", err)
		}
		if info.InstanceID == "" || hits[0].Labels[appspec.InstanceLabelKey] != info.InstanceID {
			return docker.ContainerState{}, errMailEngineForeign
		}
	}
	return hits[0], nil
}

// containerIP is the first app network address of a container.
func containerIP(c docker.ContainerState) string {
	for _, n := range c.Networks {
		if n.IPAddress != "" {
			return n.IPAddress
		}
	}
	return ""
}

// mailProbe fills the container state and admin reachability for one mail app.
func (rt *Router) mailProbe(ctx context.Context, svc store.DesiredService) mailStatusResponse {
	out := mailStatusResponse{App: svc.Name, Installed: true, ContainerState: "unknown", TLS: mailTLSKind(svc.Env)}
	out.Domain, out.Hostname = mailNames(svc.Env)
	ip := ""
	if rt.containers != nil {
		c, err := rt.mailEngineContainer(ctx, svc)
		switch {
		case errors.Is(err, errMailEngineMissing):
			out.ContainerState = "missing"
		case err != nil:
			// Ambiguous or foreign: report unknown and never probe it.
		case c.Running:
			out.ContainerState, ip = "running", containerIP(c)
		default:
			out.ContainerState = "stopped"
		}
	}
	if ip != "" {
		out.AdminReachable = mox.AdminReachable(ctx, mox.AdminBaseURL(ip))
	}
	out.NextStep = mailNextStep(out)
	return out
}

// handleMailStatus handles GET /api/v1/mail/status. With no mail app it is one
// store read: no probes, goroutines or polling. ?app=<name> picks one install.
func (rt *Router) handleMailStatus(w http.ResponseWriter, r *http.Request) {
	out := mailStatusResponse{ContainerState: "none", TLS: "unknown"}
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: mail status: list services failed", err)
		return
	}
	want := r.URL.Query().Get("app")
	best, found := mailStatusResponse{}, false
	for _, s := range svcs {
		if !rt.isInstanceMailApp(r.Context(), s) || (want != "" && s.Name != want) {
			continue
		}
		cand := rt.mailProbe(r.Context(), s)
		if !found || mailRank(cand) > mailRank(best) {
			best, found = cand, true
		}
		if want != "" {
			break
		}
	}
	if !found {
		out.NextStep = "Install the mail-mox template from the catalog. It starts with a self-signed certificate and needs only your domain."
		if want != "" {
			out.NextStep = "No mail app named " + want + " is installed."
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	writeJSON(w, http.StatusOK, best)
}

func mailNextStep(s mailStatusResponse) string {
	switch {
	case s.ContainerState == "missing" || s.ContainerState == "stopped":
		return "The mail container is not running. Start or redeploy the app and check its logs."
	case s.ContainerState == "running" && !s.AdminReachable:
		return "The mail container is running but its admin API does not answer yet. Wait a minute, then check the app logs."
	case s.TLS == "self-signed":
		return "Mail is up with a self-signed certificate. Replace it with a real one and set up DNS (levelrail-cli mail dns plan) before sending real mail."
	case s.ContainerState == "running":
		return "Mail is up. Check DNS with levelrail-cli mail dns check <domain>."
	}
	return "Open the mail app in the dashboard to check its state."
}
