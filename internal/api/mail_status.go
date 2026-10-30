package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/mail/mox"
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

// mailProbe fills the container state and admin reachability for one mail app.
func (rt *Router) mailProbe(ctx context.Context, name string, env map[string]string) mailStatusResponse {
	out := mailStatusResponse{App: name, Installed: true, ContainerState: "unknown", TLS: mailTLSKind(env)}
	out.Domain, out.Hostname = mailNames(env)
	ip := ""
	if rt.containers != nil {
		list, lerr := rt.containers.ListByPrefix(ctx, name)
		if lerr == nil {
			out.ContainerState = "missing"
			for _, c := range list {
				if c.Labels["levelrail.mail"] != "engine" {
					continue
				}
				out.ContainerState = "stopped"
				if c.Running {
					out.ContainerState = "running"
					for _, n := range c.Networks {
						if n.IPAddress != "" {
							ip = n.IPAddress
							break
						}
					}
				}
				break
			}
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
		if s.TemplateID != mailTemplateID || (want != "" && s.Name != want) {
			continue
		}
		cand := rt.mailProbe(r.Context(), s.Name, s.Env)
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
