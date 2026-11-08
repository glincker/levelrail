package api

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxPreviewHostTemplate = 200

var previewHostPlaceholder = regexp.MustCompile(`\{(number|branch|domain)\}`)

// renderPreviewHost expands the template; the branch becomes one DNS label.
// ok is false when the result is not a plausible hostname.
func renderPreviewHost(tpl, domain string, number int, branch string) (string, bool) {
	if strings.Contains(tpl, "{branch}") && ingress.SanitizeDNSLabel(branch) == "" {
		return "", false
	}
	host := strings.NewReplacer(
		"{number}", strconv.Itoa(number), "{branch}", ingress.SanitizeDNSLabel(branch), "{domain}", domain,
	).Replace(tpl)
	return host, validPreviewHostname(host)
}

var previewHostnamePattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func validPreviewHostname(h string) bool {
	return len(h) <= 253 && previewHostnamePattern.MatchString(h)
}

// domainWithin reports whether host is d or a subdomain of d.
func domainWithin(host, d string) bool {
	host, d = strings.ToLower(host), strings.ToLower(d)
	return host == d || strings.HasSuffix(host, "."+d)
}

// validatePreviewHostTemplate checks a template and its domain against the
// app's own domains, so a template can never place a preview on a host the
// app does not own. Returns a client-facing message, empty when valid.
func validatePreviewHostTemplate(tpl, wildcard string, appDomains []string) string {
	if tpl == "" {
		return ""
	}
	if len(tpl) > maxPreviewHostTemplate || strings.ContainsAny(tpl, " \t\r\n/:@") {
		return "host_template must be a hostname pattern without spaces, slashes or ports"
	}
	if !strings.Contains(tpl, "{number}") && !strings.Contains(tpl, "{branch}") {
		return "host_template must contain {number} or {branch} so each pull request gets its own host"
	}
	if strings.Contains(tpl, "{domain}") {
		if wildcard == "" {
			return "host_template uses {domain}, so wildcard_domain is required"
		}
	} else if wildcard == "" {
		return "wildcard_domain is required so the host can be checked against the app's domains"
	}
	wildcard = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(wildcard), "*."))
	if !validPreviewHostname(wildcard) {
		return "wildcard_domain must be a domain name such as example.com"
	}
	owned := false
	for _, d := range appDomains {
		if domainWithin(wildcard, d) {
			owned = true
			break
		}
	}
	if !owned {
		return "wildcard_domain must be one of this app's domains or a subdomain of one"
	}
	probe, ok := renderPreviewHost(tpl, wildcard, 1, "branch")
	if !ok || !domainWithin(probe, wildcard) {
		return "host_template must render a valid host under wildcard_domain"
	}
	return ""
}

// applyPreviewHostFields validates and applies the host template fields of a policy update.
func (rt *Router) applyPreviewHostFields(ctx context.Context, app string, s *store.PreviewAppSettings, req setPreviewPolicyRequest) string {
	if req.HostTemplate == nil && req.WildcardDomain == nil {
		return ""
	}
	if req.HostTemplate != nil {
		s.HostTemplate = strings.TrimSpace(*req.HostTemplate)
	}
	if req.WildcardDomain != nil {
		s.WildcardDomain = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(*req.WildcardDomain), "*."))
	}
	if s.HostTemplate == "" {
		s.WildcardDomain = ""
		return ""
	}
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil {
		return "app not found"
	}
	return validatePreviewHostTemplate(s.HostTemplate, s.WildcardDomain, svc.Domains)
}

// templatedPreviewHost renders the saved template, re-checking ownership at
// deploy time since the app's domains may have changed since it was saved.
func (rt *Router) templatedPreviewHost(ctx context.Context, app string, number int, branch string) string {
	s := rt.previewSettings(ctx, app)
	if s.HostTemplate == "" || s.WildcardDomain == "" {
		return ""
	}
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil || validatePreviewHostTemplate(s.HostTemplate, s.WildcardDomain, svc.Domains) != "" {
		return ""
	}
	host, ok := renderPreviewHost(s.HostTemplate, s.WildcardDomain, number, branch)
	if !ok {
		return ""
	}
	return host
}
