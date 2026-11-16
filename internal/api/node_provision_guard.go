package api

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// nodeProvisionGuardMu serializes the count-then-save step so two concurrent
// creates cannot both pass the server cap.
var nodeProvisionGuardMu sync.Mutex

const defaultProvisionMaxServers = 1

// provisionCountsAgainstCap reports whether a row may still own a billable
// server. Failed rows are excluded: a timeout deletes its server first.
func provisionCountsAgainstCap(p store.NodeProvision) bool {
	switch p.Status {
	case store.NodeProvisionStatusFailed, store.NodeProvisionStatusDestroyed:
		return false
	}
	return true
}

func (rt *Router) provisionSetting(ctx context.Context, key, env string) string {
	if rt.runtimeSettings != nil {
		return strings.TrimSpace(rt.runtimeSettings.String(ctx, key))
	}
	return strings.TrimSpace(os.Getenv(env))
}

// checkProvisionServerCap refuses a create when the cap of live provisioned
// servers is reached. The caller holds nodeProvisionGuardMu.
func (rt *Router) checkProvisionServerCap(ctx context.Context) (string, error) {
	limit := rt.tunableInt(ctx, runtimesettings.KeyProvisionMaxServers, "APP_PROVISION_MAX_SERVERS", defaultProvisionMaxServers)
	rows, err := rt.nodeProvisions.ListNodeProvisions(ctx)
	if err != nil {
		return "", fmt.Errorf("list node provisions: %w", err)
	}
	live := 0
	for _, p := range rows {
		if provisionCountsAgainstCap(p) {
			live++
		}
	}
	if live >= limit {
		return fmt.Sprintf("provision limit reached: %d of %d servers exist, destroy one or raise provision.max_servers", live, limit), nil
	}
	return "", nil
}

// checkProvisionPriceCap compares the chosen size's listed monthly price with
// the USD cap. An empty cap disables the check; an unpriced size is refused
// unless provision.allow_unknown_price is on.
func (rt *Router) checkProvisionPriceCap(ctx context.Context, p NodeProvisioner, region, size string) (string, error) {
	capText := rt.provisionSetting(ctx, runtimesettings.KeyProvisionMaxMonthlyPrice, "APP_PROVISION_MAX_MONTHLY_PRICE_USD")
	if capText == "" {
		return "", nil
	}
	maxUSD, err := strconv.ParseFloat(capText, 64)
	if err != nil {
		return "", fmt.Errorf("parse provision price cap: %w", err)
	}
	sizes, err := p.ListSizes(ctx, region)
	if err != nil {
		return "", fmt.Errorf("list sizes: %w", err)
	}
	usd, known := listedMonthlyUSD(sizes, size, rt.eurToUSD(ctx))
	if !known {
		if rt.runtimeSettings != nil && rt.runtimeSettings.Bool(ctx, runtimesettings.KeyProvisionAllowUnknownPrice) {
			return "", nil
		}
		return "the provider lists no usable price for this size, so the price cap cannot be checked", nil
	}
	if usd > maxUSD {
		return fmt.Sprintf("size price %.2f USD per month is above the cap of %.2f, raise provision.max_monthly_price_usd", usd, maxUSD), nil
	}
	return "", nil
}

func (rt *Router) eurToUSD(ctx context.Context) float64 {
	rate, err := strconv.ParseFloat(rt.provisionSetting(ctx, runtimesettings.KeyProvisionEURToUSDRate, "APP_PROVISION_EUR_TO_USD_RATE"), 64)
	if err != nil || rate <= 0 {
		return 1.10
	}
	return rate
}

func listedMonthlyUSD(sizes []provision.Size, id string, eurRate float64) (float64, bool) {
	for _, s := range sizes {
		if s.ID != id {
			continue
		}
		price, err := strconv.ParseFloat(s.PriceMonthly, 64)
		if err != nil {
			return 0, false
		}
		switch strings.ToUpper(s.Currency) {
		case "USD":
			return price, true
		case "EUR":
			return price * eurRate, true
		}
		return 0, false
	}
	return 0, false
}
