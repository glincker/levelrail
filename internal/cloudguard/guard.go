// Package cloudguard is the one gate every AWS create goes through: caps,
// allowlists, kill switch and mandatory tags. It never deletes anything.
package cloudguard

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudspend"
)

// Config holds the live guard settings. Zero values mean no limit, except
// ServicesEnabled and InstanceTypes, which fail closed.
type Config struct {
	KillSwitch bool
	// MonthlyCapUSD is the default cap per account; AccountCaps overrides it.
	MonthlyCapUSD   float64
	AccountCaps     map[string]float64
	PerCreateMaxUSD float64
	// InstanceTypes is the allowlist for creates that carry an instance type.
	InstanceTypes   []string
	ServicesEnabled map[string]bool
	// DefaultTTL feeds the expires tag; zero omits the tag.
	DefaultTTL time.Duration
	// TagPrefix defaults to the prefix of cloudspend.ProjectTagKey.
	TagPrefix string
	Now       func() time.Time
}

// Refusal codes.
const (
	CodeKillSwitch      = "kill_switch"
	CodeServiceDisabled = "service_disabled"
	CodePerCreateMax    = "per_create_max"
	CodeMonthlyCap      = "monthly_cap"
	CodeSpendUnknown    = "spend_unknown"
	CodeInstanceType    = "instance_type_not_allowed"
	CodeTagsMissing     = "tags_missing"
	CodeAccountMissing  = "account_missing"
)

var refusals = map[string]string{
	CodeKillSwitch:      "AWS creates are paused by the kill switch.",
	CodeServiceDisabled: "This AWS service is not enabled for creates.",
	CodePerCreateMax:    "The estimated monthly cost is above the per create limit.",
	CodeMonthlyCap:      "This create would take the account past its monthly cap.",
	CodeSpendUnknown:    "Month to date spend is unavailable, so the monthly cap cannot be checked.",
	CodeInstanceType:    "This instance type is not on the allowlist.",
	CodeTagsMissing:     "The mandatory management tags are missing.",
	CodeAccountMissing:  "An account is required.",
}

// CreateRequest describes one create before it reaches AWS.
type CreateRequest struct {
	Account             string
	Service             string
	Resource            string
	InstanceType        string
	EstimatedMonthlyUSD float64
	Tags                map[string]string
}

// Decision is the guard answer. Message is fixed text per Code.
type Decision struct {
	Allowed bool
	Code    string
	Message string
}

// SpendSource returns month to date USD spend for an account.
type SpendSource interface {
	MonthToDateUSD(ctx context.Context, account string) (float64, error)
}

// Guard evaluates creates against the live Config.
type Guard struct {
	cfg   func() Config
	spend SpendSource
}

// New builds a guard. spend may be nil, which makes a set cap fail closed.
func New(cfg func() Config, spend SpendSource) *Guard { return &Guard{cfg: cfg, spend: spend} }

func refuse(code string) Decision { return Decision{Code: code, Message: refusals[code]} }

// Check allows or refuses one create. The first failing rule wins.
func (g *Guard) Check(ctx context.Context, r CreateRequest) Decision {
	cfg := g.cfg()
	switch {
	case cfg.KillSwitch:
		return refuse(CodeKillSwitch)
	case r.Account == "":
		return refuse(CodeAccountMissing)
	case !cfg.ServicesEnabled[r.Service]:
		return refuse(CodeServiceDisabled)
	case r.InstanceType != "" && !contains(cfg.InstanceTypes, r.InstanceType):
		return refuse(CodeInstanceType)
	case cfg.PerCreateMaxUSD > 0 && r.EstimatedMonthlyUSD > cfg.PerCreateMaxUSD:
		return refuse(CodePerCreateMax)
	case !hasManagedTags(cfg, r.Tags):
		return refuse(CodeTagsMissing)
	}
	limit := cfg.MonthlyCapUSD
	if v, ok := cfg.AccountCaps[r.Account]; ok {
		limit = v
	}
	if limit > 0 {
		if g.spend == nil {
			return refuse(CodeSpendUnknown)
		}
		mtd, err := g.spend.MonthToDateUSD(ctx, r.Account)
		if err != nil {
			return refuse(CodeSpendUnknown)
		}
		if mtd+r.EstimatedMonthlyUSD > limit {
			return refuse(CodeMonthlyCap)
		}
	}
	return Decision{Allowed: true}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// Tags returns the mandatory tag set for a new resource, with expires from
// the configured TTL.
func (g *Guard) Tags(m TagMeta) map[string]string { return InjectTags(g.cfg(), nil, m) }

// TagMeta carries the values behind the mandatory tags.
type TagMeta struct{ Owner, Env, Project, ID string }

func prefix(cfg Config) string {
	if cfg.TagPrefix != "" {
		return cfg.TagPrefix
	}
	return strings.TrimSuffix(cloudspend.ProjectTagKey, "project")
}

// Tag key suffixes, joined to the prefix.
const (
	tagManaged = "managed"
	tagOwner   = "owner"
	tagEnv     = "env"
	tagProject = "project"
	tagID      = "id"
	tagExpires = "expires"
)

// TagKey returns the full key for a suffix such as "expires".
func TagKey(cfg Config, suffix string) string { return prefix(cfg) + suffix }

// InjectTags returns base plus the mandatory tags. Mandatory keys always win
// over caller values so a caller cannot drop or forge them.
func InjectTags(cfg Config, base map[string]string, m TagMeta) map[string]string {
	out := make(map[string]string, len(base)+6)
	for k, v := range base {
		out[k] = v
	}
	p := prefix(cfg)
	out[p+tagManaged] = "true"
	out[p+tagOwner] = m.Owner
	out[p+tagEnv] = m.Env
	out[p+tagProject] = m.Project
	out[p+tagID] = m.ID
	if cfg.DefaultTTL > 0 {
		now := time.Now
		if cfg.Now != nil {
			now = cfg.Now
		}
		out[p+tagExpires] = now().UTC().Add(cfg.DefaultTTL).Format(time.RFC3339)
	}
	return out
}

func hasManagedTags(cfg Config, tags map[string]string) bool {
	p := prefix(cfg)
	for _, s := range []string{tagOwner, tagEnv, tagProject, tagID} {
		if tags[p+s] == "" {
			return false
		}
	}
	return tags[p+tagManaged] == "true"
}

// CacheSpend reads month to date USD from the shared spend cache.
type CacheSpend struct {
	Cache *cloudspend.Cache
	// Fetch builds the provider fetch for one account.
	Fetch func(account string) cloudspend.Fetch
}

// MonthToDateUSD implements SpendSource.
func (c CacheSpend) MonthToDateUSD(ctx context.Context, account string) (float64, error) {
	if c.Cache == nil || c.Fetch == nil {
		return 0, errors.New("cloudguard: spend source not configured")
	}
	rep, err := c.Cache.Get(ctx, cloudspend.Key{AccountID: account, Scope: "guard"}, c.Fetch(account))
	if err != nil {
		return 0, err
	}
	if rep.State != cloudspend.StateOK {
		return 0, errors.New("cloudguard: spend not available")
	}
	for _, t := range rep.Totals {
		if t.Currency == "USD" {
			return t.Amount, nil
		}
	}
	return 0, nil
}
