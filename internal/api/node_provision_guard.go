package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// nodeProvisionGuardMu serializes the count-then-save step so two concurrent
// creates cannot both pass the server cap.
var nodeProvisionGuardMu sync.Mutex

const defaultProvisionMaxServers = 1

// provisionServerDeletedNote marks a failed row whose provider server was
// confirmed deleted, so only failures that may still bill keep counting.
const provisionServerDeletedNote = "; the server was deleted"

// nodeProvisionCreateTimeoutEnv bounds the detached provider calls of a
// create, which must finish even when the caller disconnects.
const nodeProvisionCreateTimeoutEnv = "APP_NODE_PROVISION_CREATE_TIMEOUT"

const defaultNodeProvisionCreateTimeout = 3 * time.Minute

// nodeProvisionTransitionMu makes read-check-write status changes atomic
// within this process, so a stale refresh can never overwrite a destroy.
var nodeProvisionTransitionMu sync.Mutex

var errProvisionStatusChanged = errors.New("node provision status changed concurrently")

// provisionServerLeaked reports a failed row whose recorded server was not
// confirmed deleted: it may still run and bill.
func provisionServerLeaked(p store.NodeProvision) bool {
	return p.Status == store.NodeProvisionStatusFailed && p.ProviderServerID != "" &&
		!strings.HasSuffix(p.FailureReason, provisionServerDeletedNote)
}

// provisionCountsAgainstCap reports whether a row may still own a billable
// server. A failed row with an undeleted recorded server still counts.
func provisionCountsAgainstCap(p store.NodeProvision) bool {
	switch p.Status {
	case store.NodeProvisionStatusDestroyed:
		return false
	case store.NodeProvisionStatusFailed:
		return provisionServerLeaked(p)
	}
	return true
}

// transitionNodeProvision applies an update only when allow accepts the
// row's current state, re-read under a lock. It returns the current row.
func (rt *Router) transitionNodeProvision(ctx context.Context, id string, allow func(store.NodeProvision) bool, status, serverID, ipAddr, nodeID, reason string) (store.NodeProvision, error) {
	nodeProvisionTransitionMu.Lock()
	defer nodeProvisionTransitionMu.Unlock()
	cur, err := rt.nodeProvisions.GetNodeProvision(ctx, id)
	if err != nil {
		return cur, err
	}
	if !allow(cur) {
		return cur, errProvisionStatusChanged
	}
	now := time.Now()
	if err := rt.nodeProvisions.UpdateNodeProvisionStatus(ctx, id, status, serverID, ipAddr, nodeID, reason, now); err != nil {
		return cur, err
	}
	cur.Status, cur.IPAddress, cur.NodeID, cur.FailureReason, cur.UpdatedAt = status, ipAddr, nodeID, reason, now
	if serverID != "" {
		cur.ProviderServerID = serverID
	}
	return cur, nil
}

func provisionNotSettled(p store.NodeProvision) bool { return !nodeProvisionSettled(p.Status) }

// providerErrorText is the only form of a provider failure that may reach a
// log or a response: the status code and a fixed message, never the body.
func providerErrorText(err error) string {
	var perr *provision.ProviderError
	if errors.As(err, &perr) {
		return fmt.Sprintf("the provider returned status %d", perr.Status)
	}
	return "the provider request failed"
}

func providerErrorStatus(err error) int {
	var perr *provision.ProviderError
	if errors.As(err, &perr) {
		return perr.Status
	}
	return 0
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
func (rt *Router) checkProvisionPriceCap(ctx context.Context, p NodeProvisioner, provider, region, size string) (string, error) {
	capText := rt.provisionSetting(ctx, runtimesettings.KeyProvisionMaxMonthlyPrice, "APP_PROVISION_MAX_MONTHLY_PRICE_USD")
	if capText == "" {
		return "", nil
	}
	maxUSD, err := strconv.ParseFloat(capText, 64)
	if err != nil || math.IsNaN(maxUSD) || math.IsInf(maxUSD, 0) || maxUSD < 0 {
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
		if provider == "oracle" && rt.oracleBool(ctx, runtimesettings.KeyProvisionOracleAllowUnknown, "APP_PROVISION_ORACLE_ALLOW_UNKNOWN_PRICE") {
			return "", nil
		}
		return "the provider lists no usable price for this size, so the price cap cannot be checked", nil
	}
	if usd > maxUSD {
		return fmt.Sprintf("size price %.2f USD per month is above the cap of %.2f, raise provision.max_monthly_price_usd", usd, maxUSD), nil
	}
	return "", nil
}

// eurToUSD returns the configured rate, the default when unset, or 0 when
// the value is unusable so EUR sizes fail closed as unpriced.
func (rt *Router) eurToUSD(ctx context.Context) float64 {
	raw := rt.provisionSetting(ctx, runtimesettings.KeyProvisionEURToUSDRate, "APP_PROVISION_EUR_TO_USD_RATE")
	if raw == "" {
		return 1.10
	}
	rate, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return 0
	}
	return rate
}

func listedMonthlyUSD(sizes []provision.Size, id string, eurRate float64) (float64, bool) {
	for _, s := range sizes {
		if s.ID != id {
			continue
		}
		price, err := strconv.ParseFloat(s.PriceMonthly, 64)
		if err != nil || math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
			return 0, false
		}
		switch strings.ToUpper(s.Currency) {
		case "USD":
			return price, true
		case "EUR":
			usd := price * eurRate
			if eurRate <= 0 || math.IsNaN(usd) || math.IsInf(usd, 0) {
				return 0, false
			}
			return usd, true
		}
		return 0, false
	}
	return 0, false
}
