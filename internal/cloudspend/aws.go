package cloudspend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// AWS credential keys: access_key_id, secret_access_key, session_token
// (optional), account_id (needed for Budgets).
const (
	awsSigningRegion = "us-east-1"
	awsCETarget      = "AWSInsightsIndexService."
	awsBudgetTarget  = "AWSBudgetServiceGateway.DescribeBudgets"
	awsPriceTarget   = "AWSPriceListService.GetProducts"
	awsTagHint       = "Activate levelrail:project under Billing, Cost allocation tags. AWS needs up to 24 hours after activation."
	awsDisabledHint  = "Turn on cloud.spend.aws_cost_explorer. Each Cost Explorer request is billed by AWS."
)

// awsCaller sends one signed JSON 1.1 request. Tests replace it.
type awsCaller interface {
	call(ctx context.Context, service, target string, in, out any) error
}

type awsEndpoint struct{ url, signName string }

// Cost Explorer, Budgets and Pricing only answer from fixed global endpoints.
var awsEndpoints = map[string]awsEndpoint{
	"ce":      {"https://ce.us-east-1.amazonaws.com", "ce"},
	"budgets": {"https://budgets.amazonaws.com", "budgetservice"},
	"pricing": {"https://api.pricing.us-east-1.amazonaws.com", "pricing"},
}

type awsHTTP struct {
	creds  aws.Credentials
	client *http.Client
	signer *v4.Signer
	// base replaces the endpoint host in tests.
	base string
	now  func() time.Time
}

func (a *awsHTTP) call(ctx context.Context, service, target string, in, out any) error {
	ep, ok := awsEndpoints[service]
	if !ok {
		return ErrUnsupported
	}
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	url := ep.url
	if a.base != "" {
		url = a.base
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	sum := sha256.Sum256(body)
	if err := a.signer.SignHTTP(ctx, a.creds, req, hex.EncodeToString(sum[:]), ep.signName, awsSigningRegion, a.now()); err != nil {
		return &ProviderError{Code: "sign"}
	}
	_, err = doJSON(a.client, req, out)
	return err
}

// AWSReader reads Cost Explorer, Budgets and the Pricing API.
type AWSReader struct {
	cfg       func() Config
	caller    awsCaller
	accountID string

	mu   sync.Mutex
	memo map[string]awsMemo
}

// awsMemo keeps one Cost Explorer answer per account and UTC day, because
// the data refreshes about daily and every request is billed.
type awsMemo struct {
	day string
	rep SpendReport
	fc  *SpendForecast
}

func init() { Register(ProviderAWS, newAWSFromCreds) }

func newAWSFromCreds(c Credentials, cfg func() Config) (Reader, error) {
	if c["access_key_id"] == "" || c["secret_access_key"] == "" {
		return nil, errors.New("cloudspend: aws credentials are incomplete")
	}
	return NewAWSReader(&awsHTTP{
		creds: aws.Credentials{
			AccessKeyID: c["access_key_id"], SecretAccessKey: c["secret_access_key"], SessionToken: c["session_token"],
		},
		client: httpClient(cfg()), signer: v4.NewSigner(), now: func() time.Time { return cfg().now() },
	}, c["account_id"], cfg), nil
}

func newAWSReader(caller awsCaller, accountID string, cfg func() Config) *AWSReader {
	return &AWSReader{cfg: cfg, caller: caller, accountID: accountID, memo: map[string]awsMemo{}}
}

// NewAWSReader wraps a caller; exported for fakes in other packages' tests.
func NewAWSReader(caller awsCaller, accountID string, cfg func() Config) *AWSReader {
	return newAWSReader(caller, accountID, cfg)
}

// Provider implements Reader.
func (r *AWSReader) Provider() Provider { return ProviderAWS }

type awsMoney struct {
	Amount string `json:"Amount"`
	Unit   string `json:"Unit"`
}

func (m awsMoney) value() float64 {
	v, _ := strconv.ParseFloat(m.Amount, 64)
	return v
}

type ceRequest struct {
	TimePeriod    map[string]string   `json:"TimePeriod"`
	Granularity   string              `json:"Granularity"`
	Metrics       []string            `json:"Metrics"`
	GroupBy       []map[string]string `json:"GroupBy"`
	NextPageToken string              `json:"NextPageToken,omitempty"`
}

type ceResponse struct {
	NextPageToken string `json:"NextPageToken"`
	ResultsByTime []struct {
		TimePeriod struct{ End string } `json:"TimePeriod"`
		Estimated  bool                 `json:"Estimated"`
		Groups     []struct {
			Keys    []string            `json:"Keys"`
			Metrics map[string]awsMoney `json:"Metrics"`
		} `json:"Groups"`
	} `json:"ResultsByTime"`
}

// Spend reads month to date grouped by the project tag and by service in one
// daily request. It is off unless the Cost Explorer setting is on.
func (r *AWSReader) Spend(ctx context.Context, q SpendQuery) (SpendReport, error) {
	cfg := r.cfg()
	now := cfg.now()
	start, end := q.period(now)
	rep := newReport(q.AccountID, ProviderAWS, start, end, SourceCostExplorer)
	if !cfg.AWSCostExplorer {
		rep.State, rep.FixHint = StateDisabled, awsDisabledHint
		rep.FetchedAt = now
		return rep, nil
	}
	day := now.Format("2006-01-02") + start.Format("2006-01-02") + end.Format("2006-01-02")
	r.mu.Lock()
	m, ok := r.memo[q.AccountID]
	r.mu.Unlock()
	if ok && m.day == day {
		return m.rep, nil
	}
	rep, err := r.readCE(ctx, rep, start, end, cfg)
	if err != nil {
		return SpendReport{}, err
	}
	r.mu.Lock()
	r.memo[q.AccountID] = awsMemo{day: day, rep: rep}
	r.mu.Unlock()
	return rep, nil
}

func (r *AWSReader) readCE(ctx context.Context, rep SpendReport, start, end time.Time, cfg Config) (SpendReport, error) {
	req := ceRequest{
		TimePeriod:  map[string]string{"Start": start.Format("2006-01-02"), "End": dayStart(end).AddDate(0, 0, 1).Format("2006-01-02")},
		Granularity: "DAILY",
		Metrics:     []string{"UnblendedCost"},
		GroupBy: []map[string]string{
			{"Type": "TAG", "Key": ProjectTagKey}, {"Type": "DIMENSION", "Key": "SERVICE"},
		},
	}
	byProject, byService := map[Line]float64{}, map[Line]float64{}
	var asOf time.Time
	allUntagged, anyGroup := true, false
	for page := 0; page < cfg.pages(); page++ {
		var out ceResponse
		if err := r.caller.call(ctx, "ce", awsCETarget+"GetCostAndUsage", req, &out); err != nil {
			if isTagError(err) {
				rep.State, rep.FixHint, rep.FetchedAt = StateTagNotActive, awsTagHint, cfg.now()
				return rep, nil
			}
			return SpendReport{}, err
		}
		for _, day := range out.ResultsByTime {
			rep.Estimated = rep.Estimated || day.Estimated
			if t, err := time.Parse("2006-01-02", day.TimePeriod.End); err == nil && t.After(asOf) {
				asOf = t
			}
			for _, g := range day.Groups {
				if len(g.Keys) < 2 {
					continue
				}
				m := g.Metrics["UnblendedCost"]
				anyGroup = true
				tag := strings.TrimPrefix(g.Keys[0], ProjectTagKey+"$")
				if tag != "" {
					allUntagged = false
					byProject[Line{Name: tag, Currency: m.Unit}] += m.value()
				}
				byService[Line{Name: g.Keys[1], Currency: m.Unit}] += m.value()
			}
		}
		if out.NextPageToken == "" {
			break
		}
		req.NextPageToken = out.NextPageToken
	}
	rep.ByService = flattenLines(byService)
	rep.ByProject = flattenLines(byProject)
	rep.Totals = TotalsByCurrency(rep.ByService)
	rep.FetchedAt = cfg.now()
	if !asOf.IsZero() {
		rep.DataAsOf = &asOf
	}
	if anyGroup && allUntagged {
		rep.State, rep.FixHint = StateTagNotActive, awsTagHint
	}
	return rep, nil
}

func isTagError(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) && pe.Code == "DataUnavailableException"
}

func flattenLines(m map[Line]float64) []Line {
	out := make([]Line, 0, len(m))
	for l, v := range m {
		l.Amount = v
		out = append(out, l)
	}
	sortLines(out)
	return out
}

// Forecast adds the provider's forecast for the rest of the month to the
// month-to-date figure, and falls back to a linear projection when AWS has too
// little history to forecast.
func (r *AWSReader) Forecast(ctx context.Context, q SpendQuery) (SpendForecast, error) {
	cfg := r.cfg()
	now := cfg.now()
	rep, err := r.Spend(ctx, q)
	if err != nil {
		return SpendForecast{}, err
	}
	if len(rep.Totals) == 0 {
		return SpendForecast{}, ErrUnsupported
	}
	mtd := rep.Totals[0]
	end := nextMonth(now)
	lin := linearForecast(ProviderAWS, rep, now)[0]
	r.mu.Lock()
	m := r.memo[q.AccountID]
	r.mu.Unlock()
	if m.fc != nil {
		return *m.fc, nil
	}
	tomorrow := dayStart(now).AddDate(0, 0, 1)
	if !tomorrow.Before(end) {
		return lin, nil
	}
	req := map[string]any{
		"TimePeriod":  map[string]string{"Start": tomorrow.Format("2006-01-02"), "End": end.Format("2006-01-02")},
		"Metric":      "UNBLENDED_COST",
		"Granularity": "MONTHLY",
	}
	var out struct {
		Total awsMoney `json:"Total"`
	}
	if err := r.caller.call(ctx, "ce", awsCETarget+"GetCostForecast", req, &out); err != nil {
		return lin, nil
	}
	fc := SpendForecast{
		AccountID: q.AccountID, Provider: ProviderAWS, Currency: mtd.Currency,
		Amount: mtd.Amount + out.Total.value(), Source: ForecastProvider, PeriodEnd: end,
		Estimated: true, DataAsOf: rep.DataAsOf,
	}
	r.mu.Lock()
	m.rep = rep
	m.fc = &fc
	if m.day != "" {
		r.memo[q.AccountID] = m
	}
	r.mu.Unlock()
	return fc, nil
}

type budgetsResponse struct {
	NextToken string `json:"NextToken"`
	Budgets   []struct {
		BudgetName      string   `json:"BudgetName"`
		BudgetLimit     awsMoney `json:"BudgetLimit"`
		TimeUnit        string   `json:"TimeUnit"`
		CalculatedSpend struct {
			ActualSpend     awsMoney `json:"ActualSpend"`
			ForecastedSpend awsMoney `json:"ForecastedSpend"`
		} `json:"CalculatedSpend"`
		LastUpdatedTime float64 `json:"LastUpdatedTime"`
	} `json:"Budgets"`
}

// Budgets reads DescribeBudgets, which is read only, cheap and not gated.
func (r *AWSReader) Budgets(ctx context.Context, _ SpendQuery) ([]ProviderBudget, error) {
	if r.accountID == "" {
		return nil, errors.New("cloudspend: aws account id is required to read budgets")
	}
	out := []ProviderBudget{}
	req := map[string]any{"AccountId": r.accountID}
	for page := 0; page < r.cfg().pages(); page++ {
		var resp budgetsResponse
		if err := r.caller.call(ctx, "budgets", awsBudgetTarget, req, &resp); err != nil {
			return nil, err
		}
		for _, b := range resp.Budgets {
			pb := ProviderBudget{
				ID: b.BudgetName, Name: b.BudgetName, Amount: b.BudgetLimit.value(), Currency: b.BudgetLimit.Unit,
				ActualSpend: b.CalculatedSpend.ActualSpend.value(), ForecastedSpend: b.CalculatedSpend.ForecastedSpend.value(),
				Period: b.TimeUnit,
			}
			if b.LastUpdatedTime > 0 {
				t := time.Unix(int64(b.LastUpdatedTime), 0).UTC()
				pb.DataAsOf = &t
			}
			out = append(out, pb)
		}
		if resp.NextToken == "" {
			break
		}
		req["NextToken"] = resp.NextToken
	}
	return out, nil
}

// PriceQuery asks the Pricing API for an on-demand price. Filters are exact
// attribute matches, so any service and attribute set works.
type PriceQuery struct {
	ServiceCode string
	Region      string
	Filters     map[string]string
	// HoursPerMonth turns an hourly rate into a monthly estimate.
	HoursPerMonth float64
}

// PriceEstimate is a pre-create cost estimate, always flagged estimated.
type PriceEstimate struct {
	Hourly    float64 `json:"hourly"`
	Monthly   float64 `json:"monthly"`
	Currency  string  `json:"currency"`
	Estimated bool    `json:"estimated"`
}

// EstimatePrice returns the lowest positive hourly on-demand price that
// matches the query, so the caller can show a cost before creating anything.
func (r *AWSReader) EstimatePrice(ctx context.Context, q PriceQuery) (PriceEstimate, error) {
	if q.ServiceCode == "" {
		return PriceEstimate{}, errors.New("cloudspend: service code is required")
	}
	filters := []map[string]string{}
	if q.Region != "" {
		filters = append(filters, map[string]string{"Type": "TERM_MATCH", "Field": "regionCode", "Value": q.Region})
	}
	for k, v := range q.Filters {
		filters = append(filters, map[string]string{"Type": "TERM_MATCH", "Field": k, "Value": v})
	}
	req := map[string]any{"ServiceCode": q.ServiceCode, "Filters": filters, "FormatVersion": "aws_v1", "MaxResults": 100}
	var resp struct {
		PriceList []string `json:"PriceList"`
	}
	if err := r.caller.call(ctx, "pricing", awsPriceTarget, req, &resp); err != nil {
		return PriceEstimate{}, err
	}
	best, unit := 0.0, ""
	for _, item := range resp.PriceList {
		var p struct {
			Terms struct {
				OnDemand map[string]struct {
					PriceDimensions map[string]struct {
						Unit         string            `json:"unit"`
						PricePerUnit map[string]string `json:"pricePerUnit"`
					} `json:"priceDimensions"`
				} `json:"OnDemand"`
			} `json:"terms"`
		}
		if json.Unmarshal([]byte(item), &p) != nil {
			continue
		}
		for _, offer := range p.Terms.OnDemand {
			for _, d := range offer.PriceDimensions {
				for cur, s := range d.PricePerUnit {
					if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 && (best == 0 || v < best) {
						best, unit = v, cur
					}
				}
			}
		}
	}
	if best == 0 {
		return PriceEstimate{}, fmt.Errorf("%w: no price found", ErrUnsupported)
	}
	return PriceEstimate{Hourly: best, Monthly: best * q.HoursPerMonth, Currency: unit, Estimated: true}, nil
}
