package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
)

// Per-record resync outcomes.
const (
	resyncUpdated  = "updated"
	resyncWouldSet = "would_update"
	resyncSame     = "unchanged"
	resyncSkipped  = "skipped"
	resyncError    = "error"
)

// dnsProxyInspector reads the proxied flag without changing it, for dry runs.
type dnsProxyInspector interface {
	ProxyDrift(ctx context.Context, zone, fqdn, recType, content string, proxied bool) (found, drifted int, err error)
}

type appCDNResyncRequest struct {
	DryRun bool `json:"dry_run"`
}

type appCDNResyncRecord struct {
	Domain  string `json:"domain"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Result  string `json:"result"`
	Message string `json:"message,omitempty"`
}

type appCDNResyncResult struct {
	App       string               `json:"app"`
	Effective bool                 `json:"effective"`
	DryRun    bool                 `json:"dry_run"`
	Updated   int                  `json:"updated"`
	Failed    int                  `json:"failed"`
	Records   []appCDNResyncRecord `json:"records"`
}

// handleResyncAppCDN handles POST /api/v1/apps/{name}/cdn/resync. It moves the
// proxied flag of records Levelrail created for the app to the effective
// setting, one record at a time, and never touches any other record.
func (rt *Router) handleResyncAppCDN(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.abuseAppExists(w, r, name) {
		return
	}
	var req appCDNResyncRequest
	if r.ContentLength != 0 {
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAppCDNBody))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	ms := rt.managedDNS()
	if ms == nil || rt.domains == nil || rt.ingressSettings == nil {
		writeError(w, http.StatusNotImplemented, "DNS re-sync is not available")
		return
	}
	res, err := rt.resyncAppCDN(r.Context(), name, req.DryRun)
	if err != nil {
		rt.internalError(w, "api: app cdn: resync failed", err, slog.String("name", name))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) resyncAppCDN(ctx context.Context, app string, dryRun bool) (appCDNResyncResult, error) {
	ctx, cancel := context.WithTimeout(ctx, dnsAutomationBudget()*4)
	defer cancel()
	s, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return appCDNResyncResult{}, err
	}
	return rt.resyncAppCDNTo(ctx, app, rt.appDNSProxied(ctx, app, s), dryRun)
}

// resyncAppCDNTo moves the app's records to want, which setup previews before
// it saves the override.
func (rt *Router) resyncAppCDNTo(ctx context.Context, app string, want, dryRun bool) (appCDNResyncResult, error) {
	res := appCDNResyncResult{App: app, Effective: want, DryRun: dryRun, Records: []appCDNResyncRecord{}}
	all, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		return res, err
	}
	ms := rt.managedDNS()
	for _, d := range all {
		if d.ServiceName != app {
			continue
		}
		rows, err := ms.ListManagedDNSRecords(ctx, d.Domain)
		if err != nil {
			return res, err
		}
		for _, row := range rows {
			// Only records tracked as created for this app, and never TXT.
			if row.AppName != app || row.RecordType == "TXT" {
				continue
			}
			rec := appCDNResyncRecord{Domain: d.Domain, Name: row.Name, Type: row.RecordType}
			rec.Result, rec.Message = rt.resyncOne(ctx, d.Domain, row.RecordType, row.Value, want, dryRun)
			switch rec.Result {
			case resyncUpdated:
				res.Updated++
			case resyncError:
				res.Failed++
			}
			res.Records = append(res.Records, rec)
		}
	}
	return res, nil
}

func (rt *Router) resyncOne(ctx context.Context, domain, recType, value string, want, dryRun bool) (string, string) {
	t, err := rt.resolveDNSTarget(ctx, domain)
	switch {
	case errors.Is(err, dnsrecords.ErrZoneNotFound):
		return resyncSkipped, "the connected DNS provider does not manage this domain's zone"
	case err != nil:
		rt.logger.Warn("api: app cdn: resync target failed", slog.String("error", err.Error()), slog.String("domain", domain))
		return resyncError, "could not reach the DNS provider"
	case t == nil || t.Proxy == nil:
		return resyncSkipped, "no proxy-capable DNS provider is connected for this domain"
	}
	fqdn := strings.TrimSuffix(domain, ".")
	if dryRun {
		insp, ok := t.Proxy.(dnsProxyInspector)
		if !ok {
			return resyncSkipped, "this provider cannot preview a re-sync"
		}
		found, drifted, err := insp.ProxyDrift(ctx, t.Zone, fqdn, recType, value, want)
		if err != nil {
			return resyncError, "could not read the record"
		}
		return resyncOutcome(found, drifted)
	}
	changed, err := t.Proxy.SetProxied(ctx, t.Zone, fqdn, recType, value, want)
	if err != nil {
		rt.logger.Warn("api: app cdn: resync failed", slog.String("error", err.Error()), slog.String("domain", domain))
		return resyncError, "the DNS provider rejected the change"
	}
	if changed == 0 {
		return resyncSame, "already matches the setting, or the record was changed by someone else and was left alone"
	}
	return resyncUpdated, ""
}

func resyncOutcome(found, drifted int) (string, string) {
	switch {
	case found == 0:
		return resyncSkipped, "the record is gone or was changed by someone else, so it is left alone"
	case drifted == 0:
		return resyncSame, "already matches the setting"
	}
	return resyncWouldSet, ""
}
