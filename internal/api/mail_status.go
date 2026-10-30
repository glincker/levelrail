package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/mail/mox"
)

const (
	mailTemplateID     = "mail-mox"
	mailSelfSignedCert = "certs/selfsigned.pem"
)

// mailStatusResponse carries no credentials or certificate material.
type mailStatusResponse struct {
	Installed      bool   `json:"installed"`
	ContainerState string `json:"container_state"`
	AdminReachable bool   `json:"admin_reachable"`
	TLS            string `json:"tls"`
	NextStep       string `json:"next_step"`
}

// handleMailStatus handles GET /api/v1/mail/status. With no mail app it is one
// store read: no probes, goroutines or polling.
func (rt *Router) handleMailStatus(w http.ResponseWriter, r *http.Request) {
	out := mailStatusResponse{ContainerState: "none", TLS: "unknown"}
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: mail status: list services failed", err)
		return
	}
	var name string
	for _, s := range svcs {
		if s.TemplateID != mailTemplateID {
			continue
		}
		out.Installed, name = true, s.Name
		out.TLS = "self-signed"
		if f := s.Env["MAIL_TLS_CERT_FILE"]; f != "" && f != mailSelfSignedCert {
			out.TLS = "provided"
		}
		break
	}
	if !out.Installed {
		out.NextStep = "Install the mail-mox template from the catalog. It starts with a self-signed certificate and needs only your domain."
		writeJSON(w, http.StatusOK, out)
		return
	}
	out.ContainerState = "unknown"
	ip := ""
	if rt.containers != nil {
		list, lerr := rt.containers.ListByPrefix(r.Context(), name)
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
		out.AdminReachable = mox.AdminReachable(r.Context(), mox.AdminBaseURL(ip))
	}
	out.NextStep = mailNextStep(out)
	writeJSON(w, http.StatusOK, out)
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
