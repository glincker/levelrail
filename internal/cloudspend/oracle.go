package cloudspend

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/GLINCKER/levelrail/internal/ocisign"
)

// Oracle credential keys: tenancy_ocid, user_ocid, fingerprint,
// private_key_pem, home_region. Usage API calls must go to the home region.
const (
	oracleUsagePath  = "/20200107/usage"
	oracleBudgetPath = "/20190111/budgets"
)

var oracleRegionRe = regexp.MustCompile(`^[a-z0-9-]{3,40}$`)

// OracleReader reads the Usage API and the Budgets API with a signed key.
type OracleReader struct {
	cfg    func() Config
	signer ocisign.Signer
	tenant string
	// usageBase and budgetBase replace the regional hosts in tests.
	usageBase, budgetBase string
}

func init() { Register(ProviderOracle, newOracleFromCreds) }

func newOracleFromCreds(c Credentials, cfg func() Config) (Reader, error) {
	region := c["home_region"]
	if !oracleRegionRe.MatchString(region) || c["tenancy_ocid"] == "" || c["user_ocid"] == "" || c["fingerprint"] == "" {
		return nil, errors.New("cloudspend: oracle credentials are incomplete")
	}
	key, err := parseRSAKey(c["private_key_pem"])
	if err != nil {
		return nil, err
	}
	return &OracleReader{
		cfg: cfg, tenant: c["tenancy_ocid"],
		signer:     ocisign.Signer{KeyID: ocisign.KeyID(c["tenancy_ocid"], c["user_ocid"], c["fingerprint"]), Key: key},
		usageBase:  "https://usageapi." + region + ".oraclecloud.com",
		budgetBase: "https://budget." + region + ".oraclecloud.com",
	}, nil
}

// NewOracleReader builds a reader with explicit bases; tests pass a plain
// HTTP fake and a signer with AllowInsecure set.
func NewOracleReader(signer ocisign.Signer, tenant, usageBase, budgetBase string, cfg func() Config) *OracleReader {
	return &OracleReader{cfg: cfg, signer: signer, tenant: tenant, usageBase: usageBase, budgetBase: budgetBase}
}

func parseRSAKey(p string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(p))
	if block == nil {
		return nil, errors.New("cloudspend: oracle private key is not valid PEM")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if k, ok := parsed.(*rsa.PrivateKey); err == nil && ok {
		return k, nil
	}
	return nil, errors.New("cloudspend: oracle private key is not an RSA key")
}

// Provider implements Reader.
func (r *OracleReader) Provider() Provider { return ProviderOracle }

type ociUsageRequest struct {
	TenantID        string       `json:"tenantId"`
	TimeStarted     string       `json:"timeUsageStarted"`
	TimeEnded       string       `json:"timeUsageEnded"`
	Granularity     string       `json:"granularity"`
	QueryType       string       `json:"queryType"`
	GroupBy         []string     `json:"groupBy"`
	AggregateByTime bool         `json:"isAggregateByTime"`
	Forecast        *ociForecast `json:"forecast,omitempty"`
}

type ociForecast struct {
	Type    string `json:"forecastType"`
	Started string `json:"timeForecastStarted"`
	Ended   string `json:"timeForecastEnded"`
}

type ociUsageItem struct {
	Service  string   `json:"service"`
	Resource string   `json:"resourceId"`
	Amount   *float64 `json:"computedAmount"`
	Currency string   `json:"currency"`
	Ended    string   `json:"timeUsageEnded"`
}

func (r *OracleReader) post(ctx context.Context, path string, in any, out any) (http.Header, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.usageBase+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return r.send(req, body, out)
}

func (r *OracleReader) send(req *http.Request, body []byte, out any) (http.Header, error) {
	if err := r.signer.Sign(req, body); err != nil {
		return nil, &ProviderError{Code: "sign"}
	}
	return doJSON(httpClient(r.cfg()), req, out)
}

// usage runs one summarized-usage query and follows the next-page header.
func (r *OracleReader) usage(ctx context.Context, req ociUsageRequest) ([]ociUsageItem, error) {
	var all []ociUsageItem
	page := ""
	for i := 0; i < r.cfg().pages(); i++ {
		path := oracleUsagePath
		if page != "" {
			path += "?page=" + url.QueryEscape(page)
		}
		var out struct {
			Items []ociUsageItem `json:"items"`
		}
		hdr, err := r.post(ctx, path, req, &out)
		if err != nil {
			return nil, err
		}
		all = append(all, out.Items...)
		if page = hdr.Get("opc-next-page"); page == "" {
			break
		}
	}
	return all, nil
}

func (r *OracleReader) request(start, end time.Time, groupBy ...string) ociUsageRequest {
	return ociUsageRequest{
		TenantID: r.tenant, TimeStarted: start.Format(time.RFC3339), TimeEnded: end.Format(time.RFC3339),
		Granularity: "DAILY", QueryType: "COST", GroupBy: groupBy, AggregateByTime: true,
	}
}

// Spend reads cost by service, and by resource when asked. Daily granularity
// needs midnight UTC bounds, and the data lags real use by hours.
func (r *OracleReader) Spend(ctx context.Context, q SpendQuery) (SpendReport, error) {
	cfg := r.cfg()
	now := cfg.now()
	start, end := q.period(now)
	rep := newReport(q.AccountID, ProviderOracle, start, end, SourceUsageAPI)
	items, err := r.usage(ctx, r.request(dayStart(start), dayStart(end).AddDate(0, 0, 1), "service"))
	if err != nil {
		return SpendReport{}, err
	}
	rep.ByService = ociLines(items, func(i ociUsageItem) string { return i.Service })
	rep.Totals = TotalsByCurrency(rep.ByService)
	if q.ByResource {
		items, err := r.usage(ctx, r.request(dayStart(start), dayStart(end).AddDate(0, 0, 1), "resourceId"))
		if err != nil {
			return SpendReport{}, err
		}
		rep.ByResource = ociLines(items, func(i ociUsageItem) string { return i.Resource })
		for i := range rep.ByResource {
			rep.ByResource[i].ProviderID = rep.ByResource[i].Name
		}
	}
	rep.FetchedAt = now
	asOf := dayStart(now)
	rep.DataAsOf = &asOf
	return rep, nil
}

func ociLines(items []ociUsageItem, name func(ociUsageItem) string) []Line {
	sums := map[Line]float64{}
	for _, it := range items {
		if it.Amount == nil {
			continue
		}
		sums[Line{Name: name(it), Currency: it.Currency}] += *it.Amount
	}
	return flattenLines(sums)
}

// Forecast asks the Usage API for a BASIC forecast over the whole month and
// sums what it returns, falling back to linear when it returns nothing.
func (r *OracleReader) Forecast(ctx context.Context, q SpendQuery) (SpendForecast, error) {
	now := r.cfg().now()
	end := nextMonth(now)
	req := r.request(monthStart(now), end, "service")
	req.Forecast = &ociForecast{Type: "BASIC", Started: dayStart(now).Format(time.RFC3339), Ended: end.Format(time.RFC3339)}
	items, err := r.usage(ctx, req)
	if err == nil {
		if lines := ociLines(items, func(ociUsageItem) string { return "" }); len(lines) > 0 {
			t := TotalsByCurrency(lines)[0]
			return SpendForecast{
				AccountID: q.AccountID, Provider: ProviderOracle, Currency: t.Currency, Amount: t.Amount,
				Source: ForecastProvider, PeriodEnd: end, Estimated: true,
			}, nil
		}
	}
	rep, serr := r.Spend(ctx, q)
	if serr != nil {
		return SpendForecast{}, serr
	}
	return firstForecast(linearForecast(ProviderOracle, rep, now))
}

// Budgets lists active budgets in the tenancy. The API has no currency field.
func (r *OracleReader) Budgets(ctx context.Context, _ SpendQuery) ([]ProviderBudget, error) {
	out := []ProviderBudget{}
	page := ""
	for i := 0; i < r.cfg().pages(); i++ {
		v := url.Values{"compartmentId": {r.tenant}, "targetType": {"ALL"}, "lifecycleState": {"ACTIVE"}}
		if page != "" {
			v.Set("page", page)
		}
		req, err := get(ctx, r.budgetBase+oracleBudgetPath+"?"+v.Encode())
		if err != nil {
			return nil, err
		}
		var rows []struct {
			ID          string  `json:"id"`
			Name        string  `json:"displayName"`
			Amount      float64 `json:"amount"`
			Reset       string  `json:"resetPeriod"`
			Actual      float64 `json:"actualSpend"`
			Forecasted  float64 `json:"forecastedSpend"`
			TimeSpendAt string  `json:"timeSpendComputed"`
		}
		hdr, err := r.send(req, nil, &rows)
		if err != nil {
			return nil, err
		}
		for _, b := range rows {
			out = append(out, ProviderBudget{
				ID: b.ID, Name: b.Name, Amount: b.Amount, ActualSpend: b.Actual,
				ForecastedSpend: b.Forecasted, Period: b.Reset, DataAsOf: parseTime(b.TimeSpendAt),
			})
		}
		if page = hdr.Get("opc-next-page"); page == "" {
			break
		}
	}
	return out, nil
}
