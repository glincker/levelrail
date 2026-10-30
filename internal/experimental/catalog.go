package experimental

import "strings"

// Maturity labels how settled a gated feature is.
const (
	MaturityExperimental = "experimental"
	MaturityBeta         = "beta"
)

// Info describes a gated feature for operators. It is the single place names
// and summaries live, so the API, CLI and dashboard never drift.
type Info struct {
	ID       Feature
	Name     string
	Summary  string
	DocsPath string
	Maturity string
	// RestartRequired is true when startup wires something (routes, listeners,
	// reconcilers, goroutines) that a runtime switch could not add or remove.
	RestartRequired bool
}

const docsPath = "/experimental-features"

var catalog = map[Feature]Info{
	AIChat:             {Name: "AI assistant chat", Summary: "In-app AI assistant chat with your own model key.", DocsPath: "/ai-assistant-chat", Maturity: MaturityBeta},
	AIModels:           {Name: "AI models", Summary: "Host AI models on GPU nodes.", DocsPath: "/ai-models", Maturity: MaturityExperimental},
	LoadBalancer:       {Name: "Load balancer", Summary: "Spread traffic across an app's replicas with health aware balancing.", DocsPath: docsPath, Maturity: MaturityExperimental},
	IaC:                {Name: "Platform as code", Summary: "Plan, apply and export the platform's resources from files.", DocsPath: docsPath, Maturity: MaturityExperimental},
	CloudflareTunnel:   {Name: "Cloudflare Tunnel", Summary: "Expose apps through a Cloudflare tunnel with no open inbound ports.", DocsPath: docsPath, Maturity: MaturityExperimental},
	AccessRoles:        {Name: "Access roles", Summary: "Custom roles, the guest role and per-environment grants.", DocsPath: "/access-control", Maturity: MaturityExperimental},
	GlobalEnvironments: {Name: "Global environments", Summary: "Instance-wide typed environments and the environment switcher.", DocsPath: "/environments", Maturity: MaturityExperimental},
	AIControl:          {Name: "AI control", Summary: "Admin mode for governing what agents and AI tools may do.", DocsPath: "/ai-control", Maturity: MaturityExperimental},
	ResourceAdvisor:    {Name: "Resource advisor", Summary: "Limit, tuning and idle suggestions. It only suggests, never changes anything on its own.", DocsPath: "/resource-advisor", Maturity: MaturityBeta},
	CLIConsole:         {Name: "CLI console", Summary: "A web console for the platform CLI in the dashboard (never a host shell).", DocsPath: docsPath, Maturity: MaturityBeta},
	MobileAPI:          {Name: "Mobile API", Summary: "Phone pairing, device tokens and the mobile overview endpoint.", DocsPath: docsPath, Maturity: MaturityBeta},
	Plugins:            {Name: "Plugins", Summary: "Run signed, operator-approved plugins in locked-down containers. Starts with notification channels.", DocsPath: "/plugins", Maturity: MaturityExperimental},
	GitPush:            {Name: "Git push deploy", Summary: "Deploy an app with git push to a remote on your own server, using a per-app token.", DocsPath: "/git-push-deploy", Maturity: MaturityBeta},
	Canvas:             {Name: "Canvas", Summary: "Compose a project's apps, databases and connections on a canvas, review every change as a plan, then apply it. Needs Platform as code switched on.", DocsPath: "/canvas", Maturity: MaturityBeta},
	SAMLSCIM:           {Name: "SAML and SCIM", Summary: "SAML 2.0 single sign-on and SCIM 2.0 provisioning, built on the sign-in library. Beta: not yet verified against every identity provider.", DocsPath: "/sso-saml", Maturity: MaturityBeta},
	Mail:               {Name: "Mail DNS", Summary: "Plan, apply and check the SPF, DKIM, DMARC, MX and related DNS records a domain needs for email, add-only by default.", DocsPath: "/mail-dns", Maturity: MaturityExperimental},
}

// Describe returns the operator-facing description of f. Only the Settings
// toggleable features can change at runtime; every other one is wired at
// startup (routes, reconcilers, goroutines), so it needs a restart.
func Describe(f Feature) Info {
	info, ok := catalog[f]
	if !ok {
		info = Info{Name: string(f), Maturity: MaturityExperimental, DocsPath: docsPath}
	}
	info.ID = f
	info.RestartRequired = !Toggleable(f)
	return info
}

// EnvHint is the exact line to set and how to apply it.
func EnvHint(f Feature) string {
	var b strings.Builder
	b.WriteString("Set " + EnvVar + "=" + string(f))
	b.WriteString(" (comma separate it from any features already listed). ")
	b.WriteString("systemd: add Environment=" + EnvVar + "=" + string(f) + " to the control plane unit, then daemon-reload and restart it. ")
	b.WriteString("docker: add -e " + EnvVar + "=" + string(f) + " (or the compose environment entry) and recreate the container.")
	return b.String()
}
