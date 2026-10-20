package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxHostnameLength = 253

var hostnameLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// validateHostname accepts a lowercase DNS hostname of at least two labels,
// without a wildcard or trailing dot.
func validateHostname(h string) error {
	if h == "" || len(h) > maxHostnameLength {
		return errors.New("a hostname is required")
	}
	labels := strings.Split(h, ".")
	if len(labels) < 2 {
		return fmt.Errorf("%q needs at least two labels, for example apps.example.com", h)
	}
	for _, l := range labels {
		if !hostnameLabel.MatchString(l) {
			return fmt.Errorf("%q is not a valid hostname", h)
		}
	}
	return nil
}

// validateDNSAutomationFields checks the base domain, CNAME target and TTL of
// a settings update; the base domain must sit in a zone the provider manages.
func (rt *Router) validateDNSAutomationFields(ctx context.Context, upd ingressSettingsUpdate) string {
	if upd.DNSTTLSeconds != nil && (*upd.DNSTTLSeconds < 0 || *upd.DNSTTLSeconds > store.MaxDNSTTLSeconds) {
		return "dns_ttl_seconds must be between 0 and 86400"
	}
	if upd.DNSCNAMETarget != nil {
		if t := strings.ToLower(strings.TrimSpace(*upd.DNSCNAMETarget)); t != "" {
			if err := validateHostname(t); err != nil {
				return "dns_cname_target: " + err.Error()
			}
		}
	}
	if upd.AppsBaseDomain == nil {
		return ""
	}
	base := strings.ToLower(strings.TrimSpace(*upd.AppsBaseDomain))
	if base == "" {
		return ""
	}
	if err := validateHostname(base); err != nil {
		return "apps_base_domain: " + err.Error()
	}
	if z := rt.zoneLookup(ctx, "app."+base); !z.Found {
		return "apps_base_domain must be inside a zone your DNS provider manages: " + z.Message
	}
	return ""
}

func (rt *Router) appsBaseDomain(ctx context.Context) string {
	s, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return ""
	}
	return s.AppsBaseDomain
}

func appBaseHost(app, base string) string {
	label := ingress.SanitizeDNSLabel(app)
	if label == "" || base == "" {
		return ""
	}
	return label + "." + base
}

// attachAppsBaseDomain sets req.Domains to <app>.<base> for a new app that
// came without any domain, and returns that host (or "").
func (rt *Router) attachAppsBaseDomain(ctx context.Context, req *appResource) string {
	if len(req.Domains) > 0 {
		return ""
	}
	host := appBaseHost(req.Name, rt.appsBaseDomain(ctx))
	if host == "" {
		return ""
	}
	if owner, err := rt.primaryDomainOwner(ctx, host); err != nil || owner != "" {
		return ""
	}
	req.Domains = []string{host}
	return host
}

// dnsForAutoDomain creates the record for a base-domain host. The base
// domain is root-configured, so the app creator does not need root; a
// wildcard record, when the policy asks for one, already covers the host.
func (rt *Router) dnsForAutoDomain(ctx context.Context, r *http.Request, app, host string) *domainDNSResult {
	policy := rt.effectiveAutomation(ctx, r, nil)
	if policy.WildcardForBaseDomain {
		return &domainDNSResult{Domain: host, DNS: dnsResultSkipped, Message: "covered by the wildcard record for the apps base domain"}
	}
	if !policy.AutoDNS {
		return &domainDNSResult{Domain: host, DNS: dnsResultSkipped, Message: "automatic DNS is off"}
	}
	d, _ := rt.applyDomainDNS(ctx, r, app, host, dnsOpts{Mode: dnsModeAuto, CanWrite: true})
	return &d
}

type baseDomainBackfillRequest struct {
	Confirm bool `json:"confirm"`
}

type baseDomainBackfillItem struct {
	App    string           `json:"app"`
	Domain string           `json:"domain"`
	Status string           `json:"status"`
	DNS    *domainDNSResult `json:"dns,omitempty"`
}

type baseDomainBackfillResponse struct {
	DryRun     bool                     `json:"dry_run"`
	BaseDomain string                   `json:"base_domain"`
	Items      []baseDomainBackfillItem `json:"items"`
}

// handleBackfillBaseDomain handles POST
// /api/v1/settings/ingress/apps-base-domain/backfill (root): a dry run unless
// confirm is true, attaching <app>.<base> to existing apps with no domain.
func (rt *Router) handleBackfillBaseDomain(w http.ResponseWriter, r *http.Request) {
	var req baseDomainBackfillRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, goLiveBodyLimit)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	base := rt.appsBaseDomain(r.Context())
	if base == "" {
		writeError(w, http.StatusBadRequest, "set the apps base domain first")
		return
	}
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: backfill base domain: list apps", err)
		return
	}
	editor, canEdit := rt.apps.(serviceDomainsEditor)
	resp := baseDomainBackfillResponse{DryRun: !req.Confirm, BaseDomain: base, Items: []baseDomainBackfillItem{}}
	for _, svc := range svcs {
		if len(svc.Domains) > 0 {
			continue
		}
		host := appBaseHost(svc.Name, base)
		item := baseDomainBackfillItem{App: svc.Name, Domain: host, Status: "planned"}
		if owner, err := rt.primaryDomainOwner(r.Context(), host); err != nil || owner != "" {
			item.Status = "skipped: domain is already used"
			resp.Items = append(resp.Items, item)
			continue
		}
		if req.Confirm && canEdit {
			_, _, err := editor.EditServiceDomains(r.Context(), svc.Name, func(cur []string) ([]string, error) {
				return applyDomainEdit(cur, []string{host}, nil)
			})
			if err != nil {
				item.Status = "failed: " + err.Error()
				resp.Items = append(resp.Items, item)
				continue
			}
			item.Status = "attached"
			item.DNS = rt.dnsForAutoDomain(r.Context(), r, svc.Name, host)
		}
		resp.Items = append(resp.Items, item)
	}
	if req.Confirm {
		rt.nudgeReconciler()
	}
	writeJSON(w, http.StatusOK, resp)
}
