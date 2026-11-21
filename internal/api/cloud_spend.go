package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudspend"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newCloudSpendReader is the seam tests replace; production builds the provider reader.
var newCloudSpendReader = cloudspend.New

var cloudSpendCaches sync.Map

func (rt *Router) cloudSpendBool(key, env string, def bool) bool {
	if rt.runtimeSettings != nil {
		return rt.runtimeSettings.Bool(context.Background(), key)
	}
	if v, err := strconv.ParseBool(os.Getenv(env)); err == nil {
		return v
	}
	return def
}

func (rt *Router) cloudSpendConfig() cloudspend.Config {
	ctx := context.Background()
	mins := func(key, env string, def int) time.Duration {
		return time.Duration(rt.tunableInt(ctx, key, env, def)) * time.Minute
	}
	return cloudspend.Config{
		Enabled:               rt.cloudSpendBool(runtimesettings.KeyCloudSpendEnabled, "APP_CLOUD_SPEND_ENABLED", true),
		CacheTTL:              mins(runtimesettings.KeyCloudSpendCacheTTLMinutes, "APP_CLOUD_SPEND_CACHE_TTL_MINUTES", 15),
		RefreshMinInterval:    mins(runtimesettings.KeyCloudSpendRefreshMinInterval, "APP_CLOUD_SPEND_REFRESH_MIN_INTERVAL_MINUTES", 5),
		RefreshPerUserPerHour: rt.tunableInt(ctx, runtimesettings.KeyCloudSpendRefreshPerUserHour, "APP_CLOUD_SPEND_REFRESH_PER_USER_PER_HOUR", 6),
		StaleAfter:            time.Duration(rt.tunableInt(ctx, runtimesettings.KeyCloudSpendStaleAfterHours, "APP_CLOUD_SPEND_STALE_AFTER_HOURS", 72)) * time.Hour,
		AWSCostExplorer:       rt.cloudSpendBool(runtimesettings.KeyCloudSpendAWSCostExplorer, "APP_CLOUD_SPEND_AWS_COST_EXPLORER", false),
		MaxPages:              rt.tunableInt(ctx, runtimesettings.KeyCloudSpendMaxPages, "APP_CLOUD_SPEND_MAX_PAGES", 5),
		HTTPTimeout:           nodeProviderTestTimeout,
	}
}

func (rt *Router) cloudSpendCache() *cloudspend.Cache {
	v, _ := cloudSpendCaches.LoadOrStore(rt, cloudspend.NewCache(rt.cloudSpendConfig))
	return v.(*cloudspend.Cache)
}

// cloudSpendCredentials maps the sealed account credential onto the keys each spend reader reads.
func (rt *Router) cloudSpendCredentials(ctx context.Context, a store.CloudAccount) (cloudspend.Credentials, error) {
	switch a.Provider {
	case "aws":
		creds, err := rt.accountAWSCredentials(ctx, a)
		if err != nil {
			return nil, err
		}
		region := creds.Region
		if region == "" {
			region = "us-east-1"
		}
		p, err := creds.CredentialsProvider(ctx, region)
		if err != nil {
			return nil, err
		}
		got, err := p.Retrieve(ctx)
		if err != nil {
			return nil, err
		}
		var cfg struct {
			AccountID string `json:"account_id"`
		}
		_ = json.Unmarshal(a.Config, &cfg)
		return cloudspend.Credentials{"access_key_id": got.AccessKeyID, "secret_access_key": got.SecretAccessKey, "session_token": got.SessionToken, "account_id": cfg.AccountID}, nil
	case "oracle":
		token, err := rt.accountCredential(ctx, a)
		if err != nil {
			return nil, err
		}
		var c oracleStoredCredential
		if err := json.Unmarshal([]byte(token), &c); err != nil {
			return nil, errors.New("the stored Oracle credential is not readable")
		}
		return cloudspend.Credentials{"home_region": c.Region, "tenancy_ocid": c.TenancyOCID, "user_ocid": c.UserOCID, "fingerprint": c.Fingerprint, "private_key_pem": c.PrivateKeyPEM}, nil
	default:
		token, err := rt.accountCredential(ctx, a)
		if err != nil {
			return nil, err
		}
		return cloudspend.Credentials{"token": token}, nil
	}
}

func (rt *Router) cloudSpendReader(ctx context.Context, a store.CloudAccount) (cloudspend.Reader, error) {
	creds, err := rt.cloudSpendCredentials(ctx, a)
	if err != nil {
		return nil, err
	}
	return newCloudSpendReader(cloudspend.Provider(a.Provider), creds, rt.cloudSpendConfig)
}

// spendFetch builds the fetch closure the cache runs detached from the request.
func (rt *Router) spendFetch(a store.CloudAccount, rd cloudspend.Reader) cloudspend.Fetch {
	return func(ctx context.Context) (cloudspend.SpendReport, error) {
		return rd.Spend(ctx, cloudspend.SpendQuery{AccountID: a.ID})
	}
}

func (rt *Router) spendReader(w http.ResponseWriter, r *http.Request, a store.CloudAccount) (cloudspend.Reader, bool) {
	rd, err := rt.cloudSpendReader(r.Context(), a)
	switch {
	case errors.Is(err, cloudspend.ErrUnsupported):
		writeError(w, http.StatusBadRequest, "spend is not available for this provider")
	case err != nil:
		writeError(w, http.StatusBadRequest, "no usable credential is stored for this account")
	default:
		return rd, true
	}
	return nil, false
}

func spendDisabled(a store.CloudAccount) cloudspend.SpendReport {
	return cloudspend.SpendReport{AccountID: a.ID, Provider: cloudspend.Provider(a.Provider), State: cloudspend.StateDisabled,
		Totals: []cloudspend.Total{}, ByService: []cloudspend.Line{}, ByProject: []cloudspend.Line{}, ByResource: []cloudspend.Line{}}
}

func (rt *Router) writeSpendError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		writeError(w, http.StatusGatewayTimeout, "the spend read timed out")
		return
	}
	writeError(w, http.StatusBadGateway, "the provider spend call failed ("+cloudspend.ErrorCode(err)+")")
}

func (rt *Router) handleGetCloudSpend(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	if !rt.cloudSpendConfig().Enabled {
		writeJSON(w, http.StatusOK, spendDisabled(a))
		return
	}
	rd, ok := rt.spendReader(w, r, a)
	if !ok {
		return
	}
	rep, err := rt.cloudSpendCache().Get(r.Context(), cloudspend.Key{AccountID: a.ID, Scope: "mtd"}, rt.spendFetch(a, rd))
	if err != nil {
		rt.writeSpendError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (rt *Router) handleRefreshCloudSpend(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	if !rt.cloudSpendConfig().Enabled {
		writeJSON(w, http.StatusOK, spendDisabled(a))
		return
	}
	rd, ok := rt.spendReader(w, r, a)
	if !ok {
		return
	}
	user := ""
	if _, pid, _, err := rt.callerPrincipal(r); err == nil {
		user = pid
	}
	rep, dec, err := rt.cloudSpendCache().Refresh(r.Context(), cloudspend.Key{AccountID: a.ID, Scope: "mtd"}, user, rt.spendFetch(a, rd))
	if !dec.Allowed {
		secs := int(dec.RetryAfter.Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeError(w, http.StatusTooManyRequests, "spend was refreshed recently, retry later")
		return
	}
	if err != nil {
		rt.writeSpendError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.spend.refreshed", map[string]any{"account": a.ID})
	writeJSON(w, http.StatusOK, rep)
}

func (rt *Router) handleCloudSpendEstimate(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	region, typ := r.URL.Query().Get("region"), r.URL.Query().Get("instance_type")
	if typ == "" {
		writeError(w, http.StatusBadRequest, "instance_type is required")
		return
	}
	if !regionAllowed(a, region) {
		writeError(w, http.StatusForbidden, "this region is not allowed for the account")
		return
	}
	est, err := rt.estimateInstancePrice(r.Context(), a, region, typ)
	switch {
	case errors.Is(err, cloudspend.ErrUnsupported):
		writeError(w, http.StatusBadRequest, "price estimates are not available for this provider or type")
	case err != nil:
		rt.writeSpendError(w, err)
	default:
		writeJSON(w, http.StatusOK, est)
	}
}
