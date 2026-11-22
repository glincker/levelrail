// Package cloudspend reads provider-side spend, forecasts and budgets behind
// one neutral shape. Providers publish lagged numbers, so every report carries
// its data age and an estimated flag, and amounts of different currencies are
// never added together.
package cloudspend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"
)

// Provider names a spend source.
type Provider string

// Registered providers.
const (
	ProviderAWS          Provider = "aws"
	ProviderOracle       Provider = "oracle"
	ProviderDigitalOcean Provider = "digitalocean"
	ProviderHetzner      Provider = "hetzner"
)

// Report sources.
const (
	SourceUsageAPI     = "usage_api"
	SourceCostExplorer = "cost_explorer"
	SourceBudgetAPI    = "budget_api"
	SourceBalance      = "balance"
	SourceEstimate     = "estimate"
)

// Report states. Anything but StateOK means the numbers are absent on purpose.
const (
	StateOK           = "ok"
	StateTagNotActive = "tag_not_active"
	StateDisabled     = "disabled"
)

// Forecast sources.
const (
	ForecastProvider = "provider"
	ForecastLinear   = "linear"
)

// ProjectTagKey is the cost allocation tag Levelrail puts on what it creates.
const ProjectTagKey = "levelrail:project"

// ErrUnsupported is returned when a provider cannot answer a call.
var ErrUnsupported = errors.New("cloudspend: not supported by this provider")

// Config holds the live runtime settings. Keys, all under cloud.spend.:
//
//	enabled, cache_ttl_minutes, refresh_min_interval_minutes,
//	refresh_per_user_per_hour, stale_after_hours, aws_cost_explorer,
//	max_pages, http_timeout_seconds.
//
// The runtimesettings owner maps them onto this struct. A zero value means
// no caching or no limit, never a hidden default.
type Config struct {
	Enabled               bool
	CacheTTL              time.Duration
	RefreshMinInterval    time.Duration
	RefreshPerUserPerHour int
	StaleAfter            time.Duration
	// AWSCostExplorer gates every paid Cost Explorer call; false by default.
	AWSCostExplorer bool
	// MaxPages bounds pagination on every provider; below 1 reads one page.
	MaxPages    int
	HTTPTimeout time.Duration
	// Now is a test seam.
	Now func() time.Time
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

func (c Config) pages() int {
	if c.MaxPages < 1 {
		return 1
	}
	return c.MaxPages
}

// SpendQuery selects what to read. Zero Start and End mean month to date.
type SpendQuery struct {
	AccountID  string
	Start, End time.Time
	// ByResource asks for per-resource lines where the provider has them.
	ByResource bool
	// Region is only used by providers whose endpoints are regional.
	Region string
}

func (q SpendQuery) period(now time.Time) (time.Time, time.Time) {
	start, end := q.Start.UTC(), q.End.UTC()
	if q.Start.IsZero() {
		start = monthStart(now)
	}
	if q.End.IsZero() {
		end = now
	}
	return start, end
}

func monthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func dayStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func nextMonth(t time.Time) time.Time { return monthStart(t).AddDate(0, 1, 0) }

// Line is one amount in its own currency.
type Line struct {
	Name     string  `json:"name"`
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	// ProviderID links a per-resource line back to an instance.
	ProviderID string `json:"provider_id,omitempty"`
}

// Total is the sum of one currency.
type Total struct {
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// SpendReport is the neutral month-to-date view of one account.
type SpendReport struct {
	AccountID   string    `json:"account_id"`
	Provider    Provider  `json:"provider"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	State       string    `json:"state"`
	// FixHint says what to change when State is not ok.
	FixHint string `json:"fix_hint,omitempty"`
	// Totals has one entry per currency, never a cross-currency sum.
	Totals     []Total    `json:"totals"`
	ByService  []Line     `json:"by_service"`
	ByProject  []Line     `json:"by_project"`
	ByResource []Line     `json:"by_resource"`
	Source     string     `json:"source"`
	Estimated  bool       `json:"estimated"`
	DataAsOf   *time.Time `json:"data_as_of,omitempty"`
	FetchedAt  time.Time  `json:"fetched_at"`
	Stale      bool       `json:"stale"`
	// ErrorCode is a short code from the last failed refresh, never a provider body.
	ErrorCode string `json:"error_code,omitempty"`
}

// SpendForecast is the projected month-end total in one currency.
type SpendForecast struct {
	AccountID string     `json:"account_id"`
	Provider  Provider   `json:"provider"`
	Currency  string     `json:"currency"`
	Amount    float64    `json:"amount"`
	Source    string     `json:"source"`
	PeriodEnd time.Time  `json:"period_end"`
	Estimated bool       `json:"estimated"`
	DataAsOf  *time.Time `json:"data_as_of,omitempty"`
}

// ProviderBudget is a budget defined on the provider side, read only.
type ProviderBudget struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Amount          float64    `json:"amount"`
	Currency        string     `json:"currency"`
	ActualSpend     float64    `json:"actual_spend"`
	ForecastedSpend float64    `json:"forecasted_spend"`
	Period          string     `json:"period"`
	DataAsOf        *time.Time `json:"data_as_of,omitempty"`
}

// Reader reads spend for one account of one provider.
type Reader interface {
	Provider() Provider
	Spend(ctx context.Context, q SpendQuery) (SpendReport, error)
	Forecast(ctx context.Context, q SpendQuery) (SpendForecast, error)
	Budgets(ctx context.Context, q SpendQuery) ([]ProviderBudget, error)
}

// Credentials are the sealed account fields a factory needs, by key name
// (see each provider file).
type Credentials map[string]string

// Factory builds a reader from account credentials and live settings.
type Factory func(creds Credentials, cfg func() Config) (Reader, error)

var factories = map[Provider]Factory{}

// Register adds a provider. Each provider file calls it from init.
func Register(p Provider, f Factory) { factories[p] = f }

// New builds the reader for a provider, or ErrUnsupported.
func New(p Provider, creds Credentials, cfg func() Config) (Reader, error) {
	f, ok := factories[p]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, p)
	}
	return f(creds, cfg)
}

// Providers lists the registered provider names, sorted.
func Providers() []Provider {
	out := make([]Provider, 0, len(factories))
	for p := range factories {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// TotalsByCurrency sums lines per currency and keeps currencies apart.
func TotalsByCurrency(lines []Line) []Total {
	sums := map[string]float64{}
	for _, l := range lines {
		sums[l.Currency] += l.Amount
	}
	out := make([]Total, 0, len(sums))
	for c, a := range sums {
		out = append(out, Total{Currency: c, Amount: a})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Currency < out[j].Currency })
	return out
}

// LinearForecast projects month-to-date spend to the end of the period.
func LinearForecast(mtd float64, start, now, end time.Time) float64 {
	elapsed, total := now.Sub(start), end.Sub(start)
	if elapsed <= 0 || total <= 0 {
		return mtd
	}
	return mtd * float64(total) / float64(elapsed)
}

// ProviderError carries only a status and a short code, so a provider body
// never reaches a client.
type ProviderError struct {
	Status int
	Code   string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("cloudspend: provider returned %d %s", e.Status, e.Code)
}

// ErrorCode maps an error to the short code stored on a report.
func ErrorCode(err error) string {
	var pe *ProviderError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &pe) && (pe.Status == http.StatusUnauthorized || pe.Status == http.StatusForbidden):
		return "auth"
	case errors.As(err, &pe) && pe.Status == http.StatusTooManyRequests:
		return "rate_limited"
	default:
		return "unavailable"
	}
}

func newReport(account string, p Provider, start, end time.Time, source string) SpendReport {
	return SpendReport{
		AccountID: account, Provider: p, PeriodStart: start, PeriodEnd: end,
		State: StateOK, Source: source,
		Totals: []Total{}, ByService: []Line{}, ByProject: []Line{}, ByResource: []Line{},
	}
}

func linearForecast(p Provider, rep SpendReport, now time.Time) []SpendForecast {
	end := nextMonth(now)
	out := make([]SpendForecast, 0, len(rep.Totals))
	for _, t := range rep.Totals {
		out = append(out, SpendForecast{
			AccountID: rep.AccountID, Provider: p, Currency: t.Currency,
			Amount: LinearForecast(t.Amount, monthStart(now), now, end),
			Source: ForecastLinear, PeriodEnd: end, Estimated: true, DataAsOf: rep.DataAsOf,
		})
	}
	return out
}

// firstForecast returns the first currency's forecast, or ErrUnsupported when
// there is no spend to project from.
func firstForecast(fs []SpendForecast) (SpendForecast, error) {
	if len(fs) == 0 {
		return SpendForecast{}, ErrUnsupported
	}
	return fs[0], nil
}

func sortLines(l []Line) {
	sort.Slice(l, func(i, j int) bool {
		if l[i].Amount != l[j].Amount {
			return l[i].Amount > l[j].Amount
		}
		return l[i].Name < l[j].Name
	})
}
