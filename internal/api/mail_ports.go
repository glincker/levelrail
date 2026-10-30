package api

import (
	"net/http"
	"slices"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// mailWebPort is the container web listener (account UI, webmail, WebAPI).
// It is never part of the ports this feature opens.
const mailWebPort = 8081

// mailOpenPorts is the fixed set the open call may touch: SMTP, submissions,
// submission and IMAPS. Nothing else is accepted from a caller.
var mailOpenPorts = []mailPortInfo{
	{Port: 25, Purpose: "SMTP, mail from other servers"},
	{Port: 465, Purpose: "SMTP submission over implicit TLS"},
	{Port: 587, Purpose: "SMTP submission with STARTTLS"},
	{Port: 993, Purpose: "IMAP over TLS"},
}

const (
	mailPort25Note = "Many hosting providers block outbound and sometimes inbound port 25 by default. Check with your provider that port 25 is allowed for this server. Levelrail does not probe the internet to find out."
	mailOpenWarn   = "Opening ports 25, 465, 587 and 993 puts this mail server on the public internet. The server must be hardened (firewall on, patched, no open relay) and TLS must be a real certificate, not the self-signed one."
	mailBindNote   = "The bind address is one setting for the whole app, not per port. Making it public would also publish the web listener on a random public port, so this call will not do that."
)

type mailPortInfo struct {
	Port    int    `json:"port"`
	Purpose string `json:"purpose"`
	// Published is true when the app declares this host port.
	Published bool `json:"published"`
	// Bind is private, public or not_published.
	Bind string `json:"bind"`
	// Firewall is allowed, blocked, not_filtering or unknown.
	Firewall string `json:"firewall"`
}

type mailPortsResponse struct {
	App           string         `json:"app"`
	Installed     bool           `json:"installed"`
	Ports         []mailPortInfo `json:"ports"`
	WebListener   string         `json:"web_listener"`
	Port25Note    string         `json:"port_25_note"`
	Warning       string         `json:"warning"`
	NextStep      string         `json:"next_step"`
	BindScopeNote string         `json:"bind_scope_note"`
}

type mailPortsOpenRequest struct {
	App     string `json:"app,omitempty"`
	Confirm bool   `json:"confirm"`
}

type mailPortsOpenResponse struct {
	App     string `json:"app"`
	Changed []int  `json:"changed"`
	Result  string `json:"result"`
	Warning string `json:"warning"`
	Pending []int  `json:"pending,omitempty"`
	Message string `json:"message"`
}

// pickMailApp returns the named mail app, or the best ranked one when none is named.
func (rt *Router) pickMailApp(r *http.Request, want string) (store.DesiredService, bool, error) {
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		return store.DesiredService{}, false, err
	}
	var best store.DesiredService
	bestRank, found := -1, false
	for _, s := range svcs {
		if s.TemplateID != mailTemplateID || (want != "" && s.Name != want) {
			continue
		}
		rank := 0
		if want == "" {
			rank = mailRank(rt.mailProbe(r.Context(), s.Name, s.Env))
		}
		if !found || rank > bestRank {
			best, bestRank, found = s, rank, true
		}
		if want != "" {
			break
		}
	}
	return best, found, nil
}

func mailBindOf(svc store.DesiredService) string {
	if svc.BindAddress == "" || svc.BindAddress == bindaddr.Private || svc.BindAddress == "127.0.0.1" {
		return bindaddr.Private
	}
	return bindaddr.Public
}

func mailPublishedTCP(svc store.DesiredService, port int) bool {
	return slices.ContainsFunc(svc.PublishedPorts, func(p store.PublishedPort) bool {
		return p.HostPort == port && p.EffectiveProtocol() == store.PortProtocolTCP
	})
}

// mailFirewallState reads the host firewall once for the published rows of one app.
func (rt *Router) mailFirewallState(r *http.Request, app string) map[int]string {
	out := map[int]string{}
	st := rt.hostFirewallStatus(r.Context())
	for _, row := range st.PublishedPorts {
		if row.Service != app || row.Protocol != store.PortProtocolTCP {
			continue
		}
		switch {
		case !st.Installed || !st.Active || st.DefaultIncoming == "allow":
			out[row.Port] = "not_filtering"
		case row.Allowed:
			out[row.Port] = "allowed"
		default:
			out[row.Port] = "blocked"
		}
	}
	return out
}

// mailPortsView builds the four port rows; firewall is nil when the app is not read yet.
func mailPortsView(svc store.DesiredService, firewall map[int]string) []mailPortInfo {
	bind := mailBindOf(svc)
	out := make([]mailPortInfo, 0, len(mailOpenPorts))
	for _, p := range mailOpenPorts {
		p.Published = mailPublishedTCP(svc, p.Port)
		p.Bind, p.Firewall = "not_published", "unknown"
		if p.Published {
			p.Bind = bind
			if fw, ok := firewall[p.Port]; ok {
				p.Firewall = fw
			}
		}
		out = append(out, p)
	}
	return out
}

// handleMailPorts handles GET /api/v1/mail/ports. It reads stored state and
// the local host firewall only; it never opens a connection to the internet.
func (rt *Router) handleMailPorts(w http.ResponseWriter, r *http.Request) {
	want := r.URL.Query().Get("app")
	svc, found, err := rt.pickMailApp(r, want)
	if err != nil {
		rt.internalError(w, "api: mail ports: list services failed", err)
		return
	}
	out := mailPortsResponse{Ports: []mailPortInfo{}, Port25Note: mailPort25Note, Warning: mailOpenWarn, BindScopeNote: mailBindNote}
	if !found {
		out.NextStep = "Install the mail-mox template from the catalog first."
		if want != "" {
			out.NextStep = "No mail app named " + want + " is installed."
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	out.App, out.Installed = svc.Name, true
	out.Ports = mailPortsView(svc, rt.mailFirewallState(r, svc.Name))
	out.WebListener = "private"
	if mailBindOf(svc) == bindaddr.Public {
		out.WebListener = "public"
	}
	out.NextStep = "Run levelrail-cli mail ports open --confirm only after you have read the warning."
	writeJSON(w, http.StatusOK, out)
}

// handleMailPortsOpen handles POST /api/v1/mail/ports/open. It never targets
// the web listener and never changes the bind address, which is service wide.
func (rt *Router) handleMailPortsOpen(w http.ResponseWriter, r *http.Request) {
	var req mailPortsOpenRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<10); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !req.Confirm {
		writeError(w, http.StatusBadRequest, "confirm must be true. "+mailOpenWarn)
		return
	}
	svc, found, err := rt.pickMailApp(r, req.App)
	if err != nil {
		rt.internalError(w, "api: mail ports open: list services failed", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "no mail app is installed, install the mail-mox template first")
		return
	}
	out := mailPortsOpenResponse{App: svc.Name, Changed: []int{}, Warning: mailOpenWarn}
	for _, p := range mailOpenPorts {
		if !mailPublishedTCP(svc, p.Port) || mailBindOf(svc) != bindaddr.Public {
			out.Pending = append(out.Pending, p.Port)
		}
	}
	if len(out.Pending) == 0 {
		out.Result, out.Message = "unchanged", "All four mail ports are already published on a public bind. Port 25 may still be blocked by your provider."
		writeJSON(w, http.StatusOK, out)
		return
	}
	out.Result = "refused"
	out.Message = mailBindNote + " Ports still closed: " + joinPorts(out.Pending) + ". Per-port bind needs a platform change, nothing was modified."
	writeJSON(w, http.StatusConflict, out)
}

func joinPorts(ports []int) string {
	s := ""
	for i, p := range ports {
		if i > 0 {
			s += ", "
		}
		s += strconv.Itoa(p)
	}
	return s
}
