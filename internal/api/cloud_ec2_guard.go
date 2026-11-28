package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/glincker/levelrail/internal/cloudguard"
	"github.com/glincker/levelrail/internal/cloudspend"
	"github.com/glincker/levelrail/internal/provision"
	"github.com/glincker/levelrail/internal/store"
)

// armEC2CreateGuard gives an AWS provisioner the shared create guard. Every entry that
// builds one goes through resolveNodeProvisionerForAccount, so the behavior is identical.
func (rt *Router) armEC2CreateGuard(p NodeProvisioner, a store.CloudAccount) {
	g, ok := p.(interface {
		SetCreateGuard(provision.EC2CreateGuard)
	})
	if !ok {
		return
	}
	if a.ID == "" {
		a.ID = store.DefaultCloudAccountID(a.Provider)
	}
	g.SetCreateGuard(rt.ec2CreateGuard(a))
}

func (rt *Router) ec2CreateGuard(a store.CloudAccount) provision.EC2CreateGuard {
	return func(ctx context.Context, req provision.EC2CreateRequest) (map[string]string, error) {
		if !regionAllowed(a, req.Region) {
			return nil, &provision.GuardRefusal{Code: "region_not_allowed", Message: "This region is not allowed for the account."}
		}
		var src cloudguard.SpendSource
		if rd, err := rt.cloudSpendReader(ctx, a); err == nil {
			src = cloudguard.CacheSpend{Cache: rt.cloudSpendCache(), Fetch: func(string) cloudspend.Fetch { return rt.spendFetch(a, rd) }}
		}
		cfg := rt.cloudGuardConfig()
		var est float64
		if cfg.PerCreateMaxUSD > 0 || cfg.MonthlyCapUSD > 0 || len(cfg.AccountCaps) > 0 {
			pe, err := rt.estimateInstancePrice(ctx, a, req.Region, req.InstanceType)
			if err != nil || pe.Currency != "USD" {
				return nil, &provision.GuardRefusal{Code: cloudguard.CodeSpendUnknown, Message: "The instance price is unavailable, so the spend limits cannot be checked."}
			}
			est = pe.Monthly
		}
		id := req.ProvisionID
		if id == "" {
			id = req.Name
		}
		tags, d := cloudguard.New(rt.cloudGuardConfig, src).Authorize(ctx, cloudguard.InstanceInput{
			Account: a.ID, Region: req.Region, Type: req.InstanceType, Name: req.Name, EstimatedMonthlyUSD: est,
			Meta: cloudguard.TagMeta{Owner: "node-provision", Env: "default", Project: "default", ID: id}})
		if !d.Allowed {
			return nil, &provision.GuardRefusal{Code: d.Code, Message: d.Message}
		}
		return tags, nil
	}
}

// preflightEC2Create refuses a guarded create with a 4xx before any work is queued.
func preflightEC2Create(ctx context.Context, p NodeProvisioner, opts provision.CreateOpts) (int, string) {
	pf, ok := p.(interface {
		PreflightCreate(context.Context, provision.CreateOpts) error
	})
	if !ok {
		return 0, ""
	}
	err := pf.PreflightCreate(ctx, opts)
	var ref *provision.GuardRefusal
	switch {
	case err == nil:
		return 0, ""
	case errors.As(err, &ref):
		return http.StatusConflict, ref.Code + ": " + ref.Message
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
