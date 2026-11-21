package cloudspend

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Hetzner credential key: token. There is no billing API, so the report is
// running time times list price, capped at the monthly price, and flagged
// estimated. Servers deleted earlier in the month are not counted.
const hetznerDefaultBase = "https://api.hetzner.cloud/v1"

// HetznerReader estimates spend from servers and the price list.
type HetznerReader struct {
	cfg   func() Config
	token string
	base  string
}

func init() {
	Register(ProviderHetzner, func(c Credentials, cfg func() Config) (Reader, error) {
		if c["token"] == "" {
			return nil, errors.New("cloudspend: hetzner token is required")
		}
		return NewHetznerReader(c["token"], hetznerDefaultBase, cfg), nil
	})
}

// NewHetznerReader builds a reader; base is overridable for fakes.
func NewHetznerReader(token, base string, cfg func() Config) *HetznerReader {
	return &HetznerReader{cfg: cfg, token: token, base: base}
}

// Provider implements Reader.
func (r *HetznerReader) Provider() Provider { return ProviderHetzner }

type hzMeta struct {
	Meta struct {
		Pagination struct {
			NextPage *int `json:"next_page"`
		} `json:"pagination"`
	} `json:"meta"`
}

type hzPrice struct {
	Location string `json:"location"`
	Hourly   struct {
		Gross string `json:"gross"`
	} `json:"price_hourly"`
	Monthly struct {
		Gross string `json:"gross"`
	} `json:"price_monthly"`
}

type hzTypesPage struct {
	hzMeta
	ServerTypes []struct {
		Name   string    `json:"name"`
		Prices []hzPrice `json:"prices"`
	} `json:"server_types"`
}

type hzServer struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Created    string `json:"created"`
	ServerType struct {
		Name string `json:"name"`
	} `json:"server_type"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	Datacenter struct {
		Location struct {
			Name string `json:"name"`
		} `json:"location"`
	} `json:"datacenter"`
}

func (s hzServer) location() string {
	if s.Location.Name != "" {
		return s.Location.Name
	}
	return s.Datacenter.Location.Name
}

type hzServersPage struct {
	hzMeta
	Servers []hzServer `json:"servers"`
}

// hzList reads pages until the provider has no next page or MaxPages is hit.
func hzList[T any](ctx context.Context, r *HetznerReader, path string, next func(*T) *int, use func(*T)) error {
	for page := 1; page <= r.cfg().pages(); page++ {
		req, err := get(ctx, fmt.Sprintf("%s%s?page=%d&per_page=50", r.base, path, page))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+r.token)
		var out T
		if _, err := doJSON(httpClient(r.cfg()), req, &out); err != nil {
			return err
		}
		use(&out)
		if next(&out) == nil {
			return nil
		}
	}
	return nil
}

func hzKey(typeName, location string) string { return typeName + "/" + location }

// Spend estimates month to date from creation time and list prices. The
// currency is EUR, the price list's gross currency.
func (r *HetznerReader) Spend(ctx context.Context, q SpendQuery) (SpendReport, error) {
	now := r.cfg().now()
	start, end := q.period(now)
	prices := map[string]hzPrice{}
	err := hzList(ctx, r, "/server_types",
		func(p *hzTypesPage) *int { return p.Meta.Pagination.NextPage },
		func(p *hzTypesPage) {
			for _, st := range p.ServerTypes {
				for _, pr := range st.Prices {
					prices[hzKey(st.Name, pr.Location)] = pr
				}
			}
		})
	if err != nil {
		return SpendReport{}, err
	}
	rep := newReport(q.AccountID, ProviderHetzner, start, end, SourceEstimate)
	rep.Estimated = true
	byType := map[Line]float64{}
	var resources []Line
	err = hzList(ctx, r, "/servers",
		func(p *hzServersPage) *int { return p.Meta.Pagination.NextPage },
		func(p *hzServersPage) {
			for _, s := range p.Servers {
				pr, ok := prices[hzKey(s.ServerType.Name, s.location())]
				if !ok {
					continue
				}
				cost := hzServerCost(s, pr, start, end)
				byType[Line{Name: s.ServerType.Name, Currency: "EUR"}] += cost
				resources = append(resources, Line{
					Name: s.Name, Amount: cost, Currency: "EUR", ProviderID: strconv.FormatInt(s.ID, 10),
				})
			}
		})
	if err != nil {
		return SpendReport{}, err
	}
	rep.ByService = flattenLines(byType)
	rep.Totals = TotalsByCurrency(rep.ByService)
	if q.ByResource {
		sortLines(resources)
		rep.ByResource = resources
	}
	rep.FetchedAt, rep.DataAsOf = now, &now
	return rep, nil
}

// hzServerCost bills hourly from the later of creation and period start,
// capped at the monthly price as Hetzner does.
func hzServerCost(s hzServer, p hzPrice, start, end time.Time) float64 {
	from := start
	if t := parseTime(s.Created); t != nil && t.After(from) {
		from = *t
	}
	hours := end.Sub(from).Hours()
	if hours <= 0 {
		return 0
	}
	hourly, _ := strconv.ParseFloat(p.Hourly.Gross, 64)
	monthly, _ := strconv.ParseFloat(p.Monthly.Gross, 64)
	cost := hours * hourly
	if monthly > 0 && cost > monthly {
		cost = monthly
	}
	return cost
}

// Forecast is linear from the estimate.
func (r *HetznerReader) Forecast(ctx context.Context, q SpendQuery) (SpendForecast, error) {
	rep, err := r.Spend(ctx, q)
	if err != nil {
		return SpendForecast{}, err
	}
	return firstForecast(linearForecast(ProviderHetzner, rep, r.cfg().now()))
}

// Budgets returns none: Hetzner has no budget API.
func (r *HetznerReader) Budgets(context.Context, SpendQuery) ([]ProviderBudget, error) {
	return []ProviderBudget{}, nil
}
