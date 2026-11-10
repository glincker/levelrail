package platformimport

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// DokployRedirect is one redirect rule of a Dokploy application.
type DokployRedirect struct {
	Regex       string `json:"regex"`
	Replacement string `json:"replacement"`
	Permanent   bool   `json:"permanent"`
}

// DokploySecurity is one basic-auth user. The password field is left undeclared so a hash or
// secret from the source API can never reach the model, a log or a report.
type DokploySecurity struct {
	Username string `json:"username"`
}

// DokployPort is one extra published port of a Dokploy application.
type DokployPort struct {
	Published int    `json:"publishedPort"`
	Target    int    `json:"targetPort"`
	Protocol  string `json:"protocol"`
}

// DokployPolicies carries the proxy rules, extra ports and deploy commands of one application.
type DokployPolicies struct {
	Redirects  []DokployRedirect
	Security   []DokploySecurity
	Ports      []DokployPort
	PreDeploy  string
	PostDeploy string
}

// DokployTranslation is the result of TranslateDokployPolicies.
type DokployTranslation struct {
	Ingress               *IngressPlan
	PreDeploy, PostDeploy string
	Notes                 []Note
}

// TranslateDokployPolicies maps redirects, basic auth, extra ports and deploy commands. Values are
// the app's env values, so a command that embeds one is dropped instead of copied.
func TranslateDokployPolicies(p DokployPolicies, firstDomain string, values []string) DokployTranslation {
	var out DokployTranslation
	plan := IngressPlan{BasicAuth: len(p.Security) > 0}
	limits := trafficpolicy.LimitsFromEnv(os.Getenv)
	for i, r := range p.Redirects {
		name := strconv.Itoa(i + 1)
		f, why := redirectForwarder("redirect-"+name, redirectMW{regex: r.Regex, replacement: r.Replacement, permanent: strconv.FormatBool(r.Permanent)}, strings.ToLower(firstDomain), os.Getenv, limits)
		if why != "" {
			plan.Manual = append(plan.Manual, Note{Area: "ingress", Reason: "redirect rule " + name + " was not translated: " + why, Manual: "add the redirect as a forwarder rule in the domain settings"})
			continue
		}
		plan.Forwarders = append(plan.Forwarders, f)
	}
	if plan.Translated() || len(plan.Manual) > 0 {
		out.Ingress = &plan
		out.Notes = append(out.Notes, ingressNotes(plan)...)
	}
	for _, c := range []struct {
		field, raw string
		dst        *string
	}{{"pre-deploy command", p.PreDeploy, &out.PreDeploy}, {"post-deploy command", p.PostDeploy, &out.PostDeploy}} {
		cmd, err := CleanSourceCommand(c.field, c.raw, values...)
		if err != nil {
			out.Notes = append(out.Notes, Note{Area: "build", Reason: err.Error() + ", so it is not carried over", Manual: "set it by hand and keep any secret in an app secret"})
			continue
		}
		*c.dst = cmd
	}
	if out.PreDeploy != "" || out.PostDeploy != "" {
		out.Notes = append(out.Notes, Note{Area: "build", Reason: "planned: pre or post deploy commands are recorded as deploy hooks", Manual: "check the deploy hooks in the app settings"})
	}
	if len(p.Ports) > 0 {
		var list []string
		for _, pt := range p.Ports {
			list = append(list, fmt.Sprintf("%d:%d/%s", pt.Published, pt.Target, strings.ToLower(strings.TrimSpace(pt.Protocol))))
		}
		out.Notes = append(out.Notes, Note{Area: "network", Reason: "extra published ports " + strings.Join(list, ", ") + " are not routed by the proxy", Manual: "publish each extra port in the app's port settings"})
	}
	return out
}
