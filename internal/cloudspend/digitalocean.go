package cloudspend

import (
	"context"
	"errors"
	"strconv"
)

// DigitalOcean credential key: token (needs billing:read). The balance
// endpoint is account level only, with no per-droplet cost.
const doDefaultBase = "https://api.digitalocean.com/v2"

// DigitalOceanReader reads GET /customers/my/balance.
type DigitalOceanReader struct {
	cfg   func() Config
	token string
	base  string
}

func init() {
	Register(ProviderDigitalOcean, func(c Credentials, cfg func() Config) (Reader, error) {
		if c["token"] == "" {
			return nil, errors.New("cloudspend: digitalocean token is required")
		}
		return NewDigitalOceanReader(c["token"], doDefaultBase, cfg), nil
	})
}

// NewDigitalOceanReader builds a reader; base is overridable for fakes.
func NewDigitalOceanReader(token, base string, cfg func() Config) *DigitalOceanReader {
	return &DigitalOceanReader{cfg: cfg, token: token, base: base}
}

// Provider implements Reader.
func (r *DigitalOceanReader) Provider() Provider { return ProviderDigitalOcean }

// Spend reports month-to-date usage in USD, the only currency DigitalOcean bills.
func (r *DigitalOceanReader) Spend(ctx context.Context, q SpendQuery) (SpendReport, error) {
	cfg := r.cfg()
	now := cfg.now()
	start, end := q.period(now)
	req, err := get(ctx, r.base+"/customers/my/balance")
	if err != nil {
		return SpendReport{}, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	var out struct {
		MonthToDateUsage string `json:"month_to_date_usage"`
		GeneratedAt      string `json:"generated_at"`
	}
	if _, err := doJSON(httpClient(cfg), req, &out); err != nil {
		return SpendReport{}, err
	}
	usage, err := strconv.ParseFloat(out.MonthToDateUsage, 64)
	if err != nil {
		return SpendReport{}, &ProviderError{Code: "decode"}
	}
	rep := newReport(q.AccountID, ProviderDigitalOcean, start, end, SourceBalance)
	rep.Totals = []Total{{Currency: "USD", Amount: usage}}
	rep.DataAsOf = parseTime(out.GeneratedAt)
	rep.FetchedAt = now
	return rep, nil
}

// Forecast is linear from month to date, since the balance has no projection.
func (r *DigitalOceanReader) Forecast(ctx context.Context, q SpendQuery) (SpendForecast, error) {
	rep, err := r.Spend(ctx, q)
	if err != nil {
		return SpendForecast{}, err
	}
	return firstForecast(linearForecast(ProviderDigitalOcean, rep, r.cfg().now()))
}

// Budgets returns none: DigitalOcean has no budget API.
func (r *DigitalOceanReader) Budgets(context.Context, SpendQuery) ([]ProviderBudget, error) {
	return []ProviderBudget{}, nil
}
