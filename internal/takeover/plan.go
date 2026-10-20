package takeover

import (
	"fmt"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Default listen addresses a takeover moves the ingress to.
const (
	DefaultTargetHTTP  = ":80"
	DefaultTargetHTTPS = ":443"
	// DefaultWindow is how long a takeover may stay unverified before it rolls back by itself.
	DefaultWindow = 5 * time.Minute
	// PlanPath is the API path a re-check fix points at.
	PlanPath = "/api/v1/migration/takeover/plan"
)

// Holder is a running container publishing host port 80 or 443.
type Holder struct {
	Port          int
	Container     string
	Image         string
	Kind          string
	RestartPolicy string
}

// LabelRouter is a Traefik label on a container that claims a domain.
type LabelRouter struct {
	Domain    string
	Container string
}

// DomainFact is what the preflight learned about one domain.
type DomainFact struct {
	Domain       string
	App          string
	Healthy      bool
	HealthDetail string
	ResolvesTo   []string
	DNSErr       string
	CustomCert   bool
}

// Facts is everything BuildPlan decides from. The caller gathers it, so the
// rules stay pure and table-testable.
type Facts struct {
	Holders       []Holder
	HostListeners []int
	// HostProcess names the process per host listener port when known.
	HostProcess      map[int]string
	CurrentHTTP      string
	CurrentHTTPS     string
	TargetHTTP       string
	TargetHTTPS      string
	Domains          []DomainFact
	ServerIPs        []string
	LabelRouters     []LabelRouter
	OldDomains       []string
	ACMEEnabled      bool
	DiskFreeGB       float64
	DiskKnown        bool
	MemoryMB         int
	MinDiskGB        float64
	MinMemoryMB      int
	RestartSupported bool
	DockerAvailable  bool
	AcceptUnmanaged  bool
	// SocketActivation is true when systemd holds the ingress sockets.
	SocketActivation bool
	Window           time.Duration
	Now              time.Time
}

// BuildPlan runs the preflight and lists the exact steps.
func BuildPlan(f Facts) Plan {
	f = withDefaults(f)
	p := Plan{
		WindowSeconds: int(f.Window.Seconds()), TargetHTTP: f.TargetHTTP, TargetHTTPS: f.TargetHTTPS,
		CurrentHTTP: f.CurrentHTTP, CurrentHTTPS: f.CurrentHTTPS, CheckedAt: f.Now.UTC().Format(time.RFC3339),
		Checks: []Check{}, Domains: domainRows(f), ProxyNames: []string{},
	}
	proxies, kinds := recognised(f.Holders)
	p.ProxyNames = proxies
	if len(kinds) > 0 {
		p.ProxyKind = kinds[0]
	}
	p.Checks = append(p.Checks, portsCheck(f), dockerCheck(f), restartCheck(f), healthCheck(f), dnsCheck(f), certCheck(f), acmeCheck(f),
		conflictCheck(f), unmanagedCheck(f), resourceCheck(f))
	p.Checks = append(p.Checks, advisoryChecks(f, kinds)...)
	p.Verdict = verdict(p.Checks)
	if listensOn(f.CurrentHTTP, f.TargetHTTP) && listensOn(f.CurrentHTTPS, f.TargetHTTPS) {
		p.Verdict = VerdictDone
	}
	p.Steps = steps(f, proxies)
	p.Rollback = rollbackLines(f, proxies)
	return p
}

func withDefaults(f Facts) Facts {
	if f.TargetHTTP == "" {
		f.TargetHTTP = DefaultTargetHTTP
	}
	if f.TargetHTTPS == "" {
		f.TargetHTTPS = DefaultTargetHTTPS
	}
	if f.Window <= 0 {
		f.Window = DefaultWindow
	}
	if f.Now.IsZero() {
		f.Now = time.Now()
	}
	return f
}

func listensOn(current, target string) bool {
	_, cp, err := net.SplitHostPort(current)
	if err != nil {
		return false
	}
	_, tp, err := net.SplitHostPort(target)
	return err == nil && cp == tp
}

func verdict(checks []Check) string {
	v := VerdictReady
	for _, c := range checks {
		switch c.Status {
		case StatusBlock:
			return VerdictBlocked
		case StatusWarn:
			v = VerdictWarn
		}
	}
	return v
}

func recognised(holders []Holder) (names, kinds []string) {
	for _, h := range holders {
		if h.Kind == "" {
			continue
		}
		if !slices.Contains(names, h.Container) {
			names = append(names, h.Container)
		}
		if !slices.Contains(kinds, h.Kind) {
			kinds = append(kinds, h.Kind)
		}
	}
	sort.Strings(names)
	return names, kinds
}

func recheck() *Action {
	return &Action{Kind: ActionRequest, Label: "Re-check", API: PlanPath}
}

func portsCheck(f Facts) Check {
	c := Check{ID: CheckPorts, Title: "Ports 80 and 443 are held by a proxy this run can stop"}
	var unknown, known []string
	for _, h := range f.Holders {
		line := fmt.Sprintf("%s (port %d, image %s)", h.Container, h.Port, h.Image)
		if h.Kind == "" {
			unknown = append(unknown, line)
		} else {
			known = append(known, line)
		}
	}
	var host []string
	for _, p := range f.HostListeners {
		host = append(host, strconv.Itoa(p))
		if name := f.HostProcess[p]; name != "" {
			host[len(host)-1] += " (" + name + ")"
		}
	}
	switch {
	case len(unknown) > 0:
		c.Status, c.Items = StatusBlock, unknown
		c.Detail = "A container that is not a recognised proxy publishes port 80 or 443. This run only stops Traefik, Caddy and nginx containers."
		c.Fix = &Fix{Summary: "Stop that container or remove its port mapping, then re-check.", Action: recheck()}
	case len(host) > 0:
		c.Status, c.Items = StatusBlock, []string{"port " + strings.Join(host, ", ") + " is listened on by a process outside Docker"}
		c.Detail = "A host service (for example nginx or Apache installed on the server) holds the port. This run never stops host services."
		c.Fix = &Fix{Summary: "Stop that service, then re-check. This shows which process holds the port.",
			Action: &Action{Kind: ActionCopy, Label: "Copy command", Value: "sudo ss -ltnp 'sport = :80 or sport = :443'"}}
	case len(known) > 0:
		c.Status, c.Items = StatusPass, known
		c.Detail = "These containers are stopped, not removed, and started again by rollback."
	default:
		c.Status = StatusPass
		c.Detail = "Nothing holds ports 80 or 443, so there is no proxy to stop."
	}
	return c
}

func dockerCheck(f Facts) Check {
	c := Check{ID: CheckDocker, Title: "Docker is reachable", Status: StatusPass}
	if !f.DockerAvailable {
		c.Status, c.Detail = StatusBlock, "The Docker Engine API is not available to the control plane, so the old proxy cannot be stopped or started again."
	}
	return c
}

func restartCheck(f Facts) Check {
	c := Check{ID: CheckRestart, Title: "The control plane can restart itself", Status: StatusPass}
	if !f.RestartSupported {
		c.Status = StatusBlock
		c.Detail = "No process supervisor was detected. The listen addresses only change on a restart and nothing would bring the control plane back."
		c.Fix = &Fix{Summary: "Run the control plane under systemd or a container restart policy, then re-check.", Action: recheck()}
	}
	return c
}

func healthCheck(f Facts) Check {
	c := Check{ID: CheckHealth, Title: "Every app is healthy on this server"}
	if len(f.Domains) == 0 {
		c.Status = StatusBlock
		c.Detail = "No domain is routed through this server yet, so a takeover would serve nothing."
		c.Fix = &Fix{Summary: "Import and route your apps first.", Action: &Action{Kind: ActionLink, Label: "Open the migration page", Value: "/migrate"}}
		return c
	}
	var bad []string
	first := ""
	for _, d := range f.Domains {
		if !d.Healthy {
			bad = append(bad, d.Domain+": "+orDefault(d.HealthDetail, "not healthy"))
			if first == "" {
				first = d.App
			}
		}
	}
	if len(bad) == 0 {
		c.Status, c.Detail = StatusPass, fmt.Sprintf("%d domains answer from healthy apps.", len(f.Domains))
		return c
	}
	c.Status, c.Items = StatusBlock, bad
	c.Detail = "Taking over ports sends all traffic to these apps. Start with the unhealthy ones fixed."
	c.Fix = &Fix{Summary: "Open the app, read its status and logs, fix it, then re-check.", Action: &Action{Kind: ActionLink, Label: "Open the app", Value: "/apps/" + first}}
	return c
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func dnsCheck(f Facts) Check {
	c := Check{ID: CheckDNS, Title: "DNS for each domain points at this server"}
	if len(f.Domains) == 0 {
		c.Status, c.Detail = StatusPass, "No domains to check."
		return c
	}
	var unresolved, mismatch []string
	for _, d := range f.Domains {
		switch {
		case d.DNSErr != "" || len(d.ResolvesTo) == 0:
			unresolved = append(unresolved, d.Domain)
		case len(f.ServerIPs) > 0 && !overlaps(d.ResolvesTo, f.ServerIPs):
			mismatch = append(mismatch, fmt.Sprintf("%s resolves to %s", d.Domain, strings.Join(d.ResolvesTo, ", ")))
		}
	}
	ip := "this server's IP"
	if len(f.ServerIPs) > 0 {
		ip = f.ServerIPs[0]
	}
	switch {
	case len(unresolved) > 0:
		c.Status, c.Items = StatusBlock, unresolved
		c.Detail = "These domains do not resolve, so visitors and the certificate authority cannot reach this server."
		c.Fix = &Fix{Summary: "Create an A record pointing at " + ip + ", wait for it to resolve, then re-check.", Action: recheck()}
	case len(mismatch) > 0:
		c.Status, c.Items = StatusWarn, mismatch
		c.Detail = "These domains resolve elsewhere. That is expected behind a CDN, otherwise HTTP-01 certificates cannot be issued."
		c.Fix = &Fix{Summary: "Point the A record at " + ip + " (or confirm the CDN forwards to it), then re-check.", Action: recheck()}
	case len(f.ServerIPs) == 0:
		c.Status, c.Detail = StatusWarn, "This server's public address is unknown (APP_PUBLIC_HOST), so DNS could only be checked for resolving."
	default:
		c.Status, c.Detail = StatusPass, "Every domain resolves to "+strings.Join(f.ServerIPs, ", ")+"."
	}
	return c
}

func overlaps(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

func certCheck(f Facts) Check {
	c := Check{ID: CheckCerts, Title: "TLS certificates are in place or can be issued"}
	var acme []string
	for _, d := range f.Domains {
		if !d.CustomCert {
			acme = append(acme, d.Domain)
		}
	}
	if len(acme) == 0 {
		c.Status, c.Detail = StatusPass, "Every domain has an uploaded certificate."
		return c
	}
	sort.Strings(acme)
	c.Status, c.Items = StatusWarn, acme
	c.Detail = "These domains need a new certificate from the certificate authority (HTTP-01) the first time port 80 is served here. " +
		"The old proxy's certificates are not reused, and issuance needs port 80 free and DNS correct, so the first visits can fail until it succeeds. " +
		"An uploaded certificate or DNS-01 avoids that gap."
	c.Fix = &Fix{Summary: "Upload a certificate for the important domains, or accept a short window while certificates are issued.",
		Action: &Action{Kind: ActionLink, Label: "Open certificates", Value: "/settings/certificates"}}
	return c
}

func acmeCheck(f Facts) Check {
	c := Check{ID: CheckACME, Title: "Automatic certificates are switched on", Status: StatusPass}
	needs := false
	for _, d := range f.Domains {
		if !d.CustomCert {
			needs = true
		}
	}
	if needs && !f.ACMEEnabled {
		c.Status = StatusBlock
		c.Detail = "ACME is off, so this server would serve a self-signed certificate for domains without an uploaded one."
		c.Fix = &Fix{Summary: "Turn on automatic certificates with a contact email in the ingress settings, then re-check.",
			Action: &Action{Kind: ActionLink, Label: "Open ingress settings", Value: "/settings/general"}}
	}
	return c
}

func conflictCheck(f Facts) Check {
	c := Check{ID: CheckConflict, Title: "No old container claims the same domain", Status: StatusPass}
	managed := map[string]bool{}
	for _, d := range f.Domains {
		managed[strings.ToLower(d.Domain)] = true
	}
	var items []string
	for _, r := range f.LabelRouters {
		if managed[strings.ToLower(r.Domain)] {
			items = append(items, fmt.Sprintf("%s is also routed by labels on container %s", r.Domain, r.Container))
		}
	}
	if len(items) == 0 {
		return c
	}
	sort.Strings(items)
	c.Status, c.Items = StatusWarn, items
	c.Detail = "Two copies of one domain exist: the old container's Traefik router can win over the route Levelrail wrote, so the old proxy may still send visitors to the old container. " +
		"After the takeover the old proxy is stopped and only Levelrail answers, but the old container keeps running until you stop it on the old platform."
	c.Fix = &Fix{Summary: "After the takeover is verified, stop the old app on its platform so it cannot answer by mistake.",
		Action: &Action{Kind: ActionLink, Label: "Read how duplicates work", Value: "/help/migrate-same-server#two-copies-of-one-domain"}}
	return c
}

func unmanagedCheck(f Facts) Check {
	c := Check{ID: CheckUnmanaged, Title: "Every domain the old proxy serves is managed here", Status: StatusPass}
	managed := map[string]bool{}
	for _, d := range f.Domains {
		managed[strings.ToLower(d.Domain)] = true
	}
	var gone []string
	for _, d := range f.OldDomains {
		if !managed[strings.ToLower(d)] {
			gone = append(gone, d)
		}
	}
	if len(gone) == 0 {
		return c
	}
	sort.Strings(gone)
	gone = slices.Compact(gone)
	c.Items = gone
	c.Detail = "The old proxy serves these domains, but no app here does. They go offline when it stops."
	c.Fix = &Fix{Summary: "Import and route them first, or confirm below that they may go offline.", Action: &Action{Kind: ActionLink, Label: "Open the migration page", Value: "/migrate"}}
	c.Status = StatusBlock
	if f.AcceptUnmanaged {
		c.Status = StatusWarn
	}
	return c
}

func resourceCheck(f Facts) Check {
	c := Check{ID: CheckResources, Title: "Disk and memory have headroom", Status: StatusPass}
	var items []string
	status := StatusPass
	switch {
	case !f.DiskKnown:
		status = StatusWarn
		items = append(items, "free disk space could not be read")
	case f.DiskFreeGB < f.MinDiskGB:
		status = StatusBlock
		items = append(items, fmt.Sprintf("%.1f GB free, at least %.1f GB needed", f.DiskFreeGB, f.MinDiskGB))
	}
	switch {
	case f.MemoryMB <= 0:
		if status == StatusPass {
			status = StatusWarn
		}
		items = append(items, "memory could not be read")
	case f.MemoryMB < f.MinMemoryMB:
		status = StatusBlock
		items = append(items, fmt.Sprintf("%d MB of memory, at least %d MB needed", f.MemoryMB, f.MinMemoryMB))
	}
	c.Status, c.Items = status, items
	if status != StatusPass {
		c.Detail = "The ingress takes over every request the old proxy handled, and certificate issuance writes to disk."
	}
	return c
}

func advisoryChecks(f Facts, kinds []string) []Check {
	var out []Check
	if slices.Contains(kinds, "nginx") || slices.Contains(kinds, "caddy") {
		out = append(out, Check{ID: CheckSnippet, Title: "Saved, not enforced", Status: StatusWarn,
			Detail: "For nginx and Caddy, Levelrail only saves configuration snippets for the operator to apply. It never loads them, so nothing the old proxy served through its own config is carried over: after the takeover only Levelrail's own routes answer."})
	}
	if f.SocketActivation {
		out = append(out, Check{ID: CheckSocket, Title: "systemd holds the ingress sockets", Status: StatusWarn,
			Detail: "The ingress listens on sockets systemd opened on " + f.CurrentHTTP + " and " + f.CurrentHTTPS + ", and the control plane cannot rewrite those units. " +
				"The takeover makes it ignore them and bind " + f.TargetHTTP + " and " + f.TargetHTTPS + " itself, so a control plane restart is no longer seamless on those ports. Rollback returns to the systemd sockets."})
	}
	var coolify []string
	var always []string
	for _, h := range f.Holders {
		if h.Kind == "" {
			continue
		}
		if strings.Contains(strings.ToLower(h.Container), "coolify") && !slices.Contains(coolify, h.Container) {
			coolify = append(coolify, h.Container)
		}
		if h.RestartPolicy == "always" && !slices.Contains(always, h.Container) {
			always = append(always, h.Container)
		}
	}
	if len(coolify) > 0 {
		out = append(out, Check{ID: CheckPlatform, Title: "The old platform may start its proxy again", Status: StatusWarn, Items: coolify,
			Detail: "Coolify can restart its proxy when it sees it stopped, which would take port 80 back and make this run roll back. Stop the proxy once from the Coolify dashboard too, or stop Coolify's scheduler for the duration.",
			Fix:    &Fix{Summary: "Stop the proxy in the Coolify dashboard right after the run starts, then keep the run's window in mind.", Action: recheck()}})
	}
	if len(always) > 0 {
		sort.Strings(always)
		out = append(out, Check{ID: CheckRestartPol, Title: "The old proxy would come back after a reboot", Status: StatusWarn, Items: always,
			Detail: "Its restart policy is always, so Docker starts it again after a daemon restart or a reboot and it takes the ports back.",
			Fix:    &Fix{Summary: "Set it to unless-stopped so a deliberate stop sticks.", Action: &Action{Kind: ActionCopy, Label: "Copy command", Value: "docker update --restart unless-stopped " + always[0]}}})
	}
	return out
}

func steps(f Facts, proxies []string) []PlanStep {
	stop := "No proxy container holds the ports, nothing is stopped."
	if len(proxies) > 0 {
		stop = "Stop " + strings.Join(proxies, ", ") + ". The container is stopped, never removed, so rollback can start it again."
	}
	return []PlanStep{
		{ID: StepRollback, Title: "Record the way back", Reversible: true,
			Detail: fmt.Sprintf("Save the current listen addresses (%s, %s), the TLS settings and the container names. The record holds no secrets.", f.CurrentHTTP, f.CurrentHTTPS)},
		{ID: StepStopProxy, Title: "Stop the old proxy", Detail: stop, Reversible: true},
		{ID: StepTLS, Title: "Let this server terminate TLS", Reversible: true,
			Detail: "Turn off the setting that says a proxy in front terminates TLS, so the ingress serves HTTPS itself and requests certificates."},
		{ID: StepListen, Title: "Listen on " + f.TargetHTTP + " and " + f.TargetHTTPS, Reversible: true,
			Detail: "Store the new addresses in the database. The control plane reads them at start and they take precedence over APP_INGRESS_HTTP_ADDR, APP_INGRESS_HTTPS_ADDR and systemd socket activation. " +
				"The systemd drop-in is not edited, because the control plane never writes host files: the drop-in values stay as the baseline that rollback returns to."},
		{ID: StepRestart, Title: "Restart the control plane", Reversible: true,
			Detail: "A short pause while it restarts and binds the ports. Dashboard sessions survive."},
		{ID: StepVerify, Title: "Verify each domain over real HTTPS", Reversible: true,
			Detail: fmt.Sprintf("From the control plane, request every domain over HTTPS with its own Host header and check the certificate and that this ingress answered. If this does not succeed within %s the run rolls back by itself.", f.Window)},
	}
}

func rollbackLines(f Facts, proxies []string) []string {
	out := []string{fmt.Sprintf("Listen on %s and %s again.", f.CurrentHTTP, f.CurrentHTTPS), "Restore the TLS settings that were in place."}
	if len(proxies) > 0 {
		out = append(out, "Start "+strings.Join(proxies, ", ")+" again once the ports are free.")
	}
	out = append(out, "Rollback is one action and also runs by itself when verification fails or the control plane restarts before verification finishes.")
	return out
}

func domainRows(f Facts) []DomainRow {
	rows := make([]DomainRow, 0, len(f.Domains))
	for _, d := range f.Domains {
		dns := "ok"
		switch {
		case d.DNSErr != "" || len(d.ResolvesTo) == 0:
			dns = "unresolved"
		case len(f.ServerIPs) > 0 && !overlaps(d.ResolvesTo, f.ServerIPs):
			dns = "elsewhere"
		}
		cert := "acme"
		if d.CustomCert {
			cert = "custom"
		}
		rows = append(rows, DomainRow{Domain: d.Domain, App: d.App, Healthy: d.Healthy, Detail: d.HealthDetail, DNS: dns, Cert: cert})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Domain < rows[j].Domain })
	return rows
}
