package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/glincker/levelrail/internal/provision"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

const sourceListPrices = "list_prices"

// buildSpendReport reads billing and prices the discovered machines. Each
// section degrades on its own, so a missing scope never hides the rest.
func (rt *Router) buildSpendReport(ctx context.Context, provider string, p NodeProvisioner, refresh bool) (provision.SpendReport, error) {
	billing, ok := p.(provision.BillingReader)
	if !ok {
		return provision.SpendReport{}, errResourcesUnsupported
	}
	rctx, cancel := context.WithTimeout(ctx, providerReadTimeout)
	defer cancel()
	now := time.Now().UTC()
	rep := provision.SpendReport{Provider: provider, AccountID: provider + "-default", Currency: "USD", GeneratedAt: now}
	rep.Balance, rep.LastInvoice = billing.ReadBilling(rctx)
	rep.Estimate = rt.spendEstimate(ctx, provider, p, refresh)
	return rep, nil
}

func (rt *Router) spendEstimate(ctx context.Context, provider string, p NodeProvisioner, refresh bool) provision.SpendEstimate {
	// A spend refresh already passed its own cooldown, so it may bypass the
	// list cooldown but still reuses a fresh list when not refreshing.
	snap, _, retry, err := rt.loadProviderResources(ctx, provider, p, refresh)
	if retry > 0 {
		snap, _, _, err = rt.loadProviderResources(ctx, provider, p, false)
	}
	if err != nil {
		if isProviderAuthError(err) {
			needs := "droplet:read"
			if se, ok := provision.ScopeFromError(provider, "instances.read", err); ok {
				needs = se.Needs
			}
			return provision.SpendEstimate{SpendSection: provision.SpendSection{State: provision.SectionScopeMissing, Needs: needs}}
		}
		return provision.SpendEstimate{SpendSection: provision.SpendSection{State: provision.SectionUnavailable}}
	}
	est := provision.SpendEstimate{SpendSection: provision.SpendSection{State: provision.SectionOK}, ResourceCount: len(snap.resources)}
	var total float64
	for _, r := range snap.resources {
		if r.EstimatedMonthlyPrice != nil {
			total += *r.EstimatedMonthlyPrice
			est.PricedCount++
		}
	}
	at := snap.at
	est.MonthlyTotal = &provision.SpendValue{Value: strconv.FormatFloat(total, 'f', 2, 64), Source: sourceListPrices, DataAsOf: &at}
	return est
}

// handleGetNodeProviderSpend handles GET
// /api/v1/node-providers/{provider}/spend?refresh=1: billed balance, the last
// invoice and a list-price estimate, each with its own source and age.
func (rt *Router) handleGetNodeProviderSpend(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if provider == nodeProviderCustom {
		writeError(w, http.StatusBadRequest, "a manual data center has no cloud API to read spend from")
		return
	}
	p, ok, status, msg := rt.resolveNodeProvisioner(r.Context(), provider)
	if !ok {
		writeError(w, status, msg)
		return
	}
	ctx := r.Context()
	refresh := r.URL.Query().Get("refresh") == "1"
	ttl := time.Duration(rt.tunableInt(ctx, runtimesettings.KeyProvisionSpendCacheTTL, "APP_PROVISION_SPEND_CACHE_TTL_SECONDS", defaultSpendCacheTTLSecs)) * time.Second
	gap := time.Duration(rt.tunableInt(ctx, runtimesettings.KeyProvisionSpendRefreshMinInter, "APP_PROVISION_SPEND_REFRESH_MIN_INTERVAL_SECONDS", defaultSpendRefreshGap)) * time.Second
	rep, cached, retry, err := rt.providerCaps.spend.get(provider, ttl, gap, refresh, func() (provision.SpendReport, error) {
		return rt.buildSpendReport(ctx, provider, p, refresh)
	})
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "spend was just refreshed, try again shortly")
		return
	}
	if err != nil {
		if errors.Is(err, errResourcesUnsupported) {
			writeError(w, http.StatusNotImplemented, "this provider does not report spend yet")
			return
		}
		writeError(w, http.StatusBadGateway, "could not reach the provider's API")
		return
	}
	rep.Cached = cached
	writeJSON(w, http.StatusOK, rep)
}
