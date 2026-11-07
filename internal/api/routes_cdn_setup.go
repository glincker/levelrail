package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/cdn"
)

type cdnSetupRequest struct {
	Provider string `json:"provider"`
	DryRun   bool   `json:"dry_run"`
}

// cdnSetupRun carries what the steps share while one setup runs.
type cdnSetupRun struct {
	rt     *Router
	app    string
	actor  string
	domain string
	zone   string
	client cdnZoneClient
	result cdn.SetupResult
}

func (c *cdnSetupRun) add(id, status, msg, fix string) {
	c.result.Steps = append(c.result.Steps, cdn.Step{ID: id, Status: status, Message: msg, Fix: fix})
}

func (c *cdnSetupRun) blocked() bool {
	for _, s := range c.result.Steps {
		if s.Status == cdn.StatusFail {
			return true
		}
	}
	return false
}

// handleSetupAppCDN handles POST /api/v1/apps/{name}/cdn/setup. It runs the
// Cloudflare-in-front checklist and, unless dry_run, turns the proxy on.
func (rt *Router) handleSetupAppCDN(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.abuseAppExists(w, r, name) {
		return
	}
	var req cdnSetupRequest
	if r.ContentLength != 0 {
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	if req.Provider == "" {
		req.Provider = cdn.ProviderCloudflare
	}
	if rt.managedDNS() == nil || rt.domains == nil || rt.ingressSettings == nil || rt.appCDNStore() == nil {
		writeError(w, http.StatusNotImplemented, "CDN setup is not available")
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	run := &cdnSetupRun{rt: rt, app: name, actor: actor, result: cdn.SetupResult{App: name, Provider: req.Provider, DryRun: req.DryRun, Steps: []cdn.Step{}}}
	if err := run.execute(r.Context()); err != nil {
		rt.internalError(w, "api: app cdn: setup failed", err, slog.String("name", name))
		return
	}
	run.result.Summarize()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, run.result)
}

func (c *cdnSetupRun) execute(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, dnsAutomationBudget()*4)
	defer cancel()
	c.checkProvider(ctx)
	c.checkSSL(ctx)
	c.checkDNS01(ctx)
	c.checkClientAddress(ctx)
	if err := c.planDNS(ctx); err != nil {
		return err
	}
	if err := c.apply(ctx); err != nil {
		return err
	}
	return c.verify(ctx)
}

func (c *cdnSetupRun) checkProvider(ctx context.Context) {
	if c.result.Provider != cdn.ProviderCloudflare {
		c.add(cdn.StepProvider, cdn.StatusFail, fmt.Sprintf("provider %q is not supported yet; only cloudflare is connected through setup", c.result.Provider), "")
		return
	}
	domains, err := c.rt.appServiceDomains(ctx, c.app)
	switch {
	case err != nil:
		c.add(cdn.StepProvider, cdn.StatusFail, "could not read the app's domains", "")
		return
	case len(domains) == 0:
		c.add(cdn.StepProvider, cdn.StatusFail, "the app has no domains to put behind a CDN", c.rt.cliName()+" domains add <app> <domain>")
		return
	}
	c.domain = domains[0]
	t, err := c.rt.resolveDNSTarget(ctx, c.domain)
	if err != nil || t == nil || t.Proxy == nil {
		c.add(cdn.StepProvider, cdn.StatusFail, "no Cloudflare DNS provider is connected for this domain", c.rt.cliName()+" domains cloudflare-dns set --cf-api-token <token>")
		return
	}
	client, ok := t.Proxy.(cdnZoneClient)
	if !ok || t.Provider != dnsProviderCloudflare {
		c.add(cdn.StepProvider, cdn.StatusFail, "the connected DNS provider does not support a CDN proxy", "")
		return
	}
	c.client, c.zone = client, t.Zone
	c.add(cdn.StepProvider, cdn.StatusOK, fmt.Sprintf("%s is connected and manages zone %s", cdnProviderName(client), strings.TrimSuffix(t.Zone, ".")), "")
}

func (c *cdnSetupRun) checkSSL(ctx context.Context) {
	if c.client == nil {
		c.add(cdn.StepSSLMode, cdn.StatusSkipped, "needs a connected provider", "")
		return
	}
	mode, err := c.client.SSLMode(ctx, c.zone)
	switch {
	case err != nil:
		c.add(cdn.StepSSLMode, cdn.StatusWarn, "could not read the zone SSL/TLS mode; the token needs Zone Settings read", "")
	case mode == "off" || mode == "flexible":
		c.add(cdn.StepSSLMode, cdn.StatusFail, sslModeWarning(mode), "set the zone SSL/TLS mode to Full (strict) in Cloudflare")
	case mode == "full":
		c.add(cdn.StepSSLMode, cdn.StatusWarn, sslModeWarning(mode), "set the zone SSL/TLS mode to Full (strict) in Cloudflare")
	default:
		c.add(cdn.StepSSLMode, cdn.StatusOK, "zone SSL/TLS mode is "+mode, "")
	}
}

func (c *cdnSetupRun) checkDNS01(ctx context.Context) {
	if c.rt.runtimeSettings == nil {
		c.add(cdn.StepDNS01, cdn.StatusSkipped, "settings are not available", "")
		return
	}
	st := c.rt.dns01State(ctx)
	if st.Enabled && st.ProviderConnected {
		c.add(cdn.StepDNS01, cdn.StatusOK, "certificates are issued with DNS-01, which does not depend on the proxy", "")
		return
	}
	c.add(cdn.StepDNS01, cdn.StatusWarn, "certificates for proxied hosts should use DNS-01; HTTP-01 can be blocked by the proxy", c.rt.cliName()+" settings ingress dns01 enable")
}

func (c *cdnSetupRun) checkClientAddress(ctx context.Context) {
	if c.rt.runtimeSettings == nil {
		c.add(cdn.StepClientIP, cdn.StatusSkipped, "settings are not available", "")
		return
	}
	if c.rt.cloudflarePresetState(ctx).Preset == "cloudflare" {
		c.add(cdn.StepClientIP, cdn.StatusOK, "the ingress trusts CF-Connecting-IP only from Cloudflare's ranges", "")
		return
	}
	c.add(cdn.StepClientIP, cdn.StatusWarn, "without the preset every visitor appears as a Cloudflare address in logs and rate limits",
		c.rt.cliName()+" settings ingress cloudflare set --preset cloudflare --confirm-weakening")
}

func (c *cdnSetupRun) planDNS(ctx context.Context) error {
	if c.client == nil {
		c.add(cdn.StepDNSPlan, cdn.StatusSkipped, "needs a connected provider", "")
		return nil
	}
	plan, err := c.rt.resyncAppCDNTo(ctx, c.app, true, true)
	if err != nil {
		return fmt.Errorf("plan dns: %w", err)
	}
	would, errs := 0, 0
	for _, rec := range plan.Records {
		switch rec.Result {
		case resyncWouldSet:
			would++
		case resyncError:
			errs++
		}
	}
	switch {
	case errs > 0:
		c.add(cdn.StepDNSPlan, cdn.StatusWarn, fmt.Sprintf("%d record(s) could not be read; the rest would be handled", errs), c.rt.cliName()+" apps cdn resync "+c.app+" --dry-run")
	case len(plan.Records) == 0:
		c.add(cdn.StepDNSPlan, cdn.StatusOK, "Levelrail created no DNS records for this app; the proxy applies to records it creates from now on", "")
	default:
		c.add(cdn.StepDNSPlan, cdn.StatusOK, fmt.Sprintf("%d of %d record(s) would be proxied", would, len(plan.Records)), "")
	}
	return nil
}

func (c *cdnSetupRun) apply(ctx context.Context) error {
	switch {
	case c.result.DryRun:
		c.add(cdn.StepApply, cdn.StatusSkipped, "dry run: nothing was changed", "")
		return nil
	case c.blocked():
		c.add(cdn.StepApply, cdn.StatusSkipped, "fix the failed steps first; nothing was changed", "")
		return nil
	}
	on := true
	if err := c.rt.appCDNStore().SetAppDNSProxied(ctx, c.app, &on, c.actor); err != nil {
		return fmt.Errorf("save proxy setting: %w", err)
	}
	res, err := c.rt.resyncAppCDNTo(ctx, c.app, true, false)
	if err != nil {
		return fmt.Errorf("resync: %w", err)
	}
	c.result.Applied = true
	if res.Failed > 0 {
		c.add(cdn.StepApply, cdn.StatusFail, fmt.Sprintf("proxy saved, but %d record(s) failed to update", res.Failed), c.rt.cliName()+" apps cdn resync "+c.app)
		return nil
	}
	c.add(cdn.StepApply, cdn.StatusOK, fmt.Sprintf("proxy turned on; %d record(s) updated", res.Updated), "")
	return nil
}

func (c *cdnSetupRun) verify(ctx context.Context) error {
	if !c.result.Applied {
		c.add(cdn.StepVerify, cdn.StatusSkipped, "nothing was applied", "")
		return nil
	}
	cur, err := c.rt.appCDNResource(ctx, c.app)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if !cur.Effective {
		c.add(cdn.StepVerify, cdn.StatusFail, "the proxy setting did not stick", "")
		return nil
	}
	c.add(cdn.StepVerify, cdn.StatusOK, "the app is proxied; open the hostname from outside your network to confirm the certificate and the page", "")
	return nil
}
