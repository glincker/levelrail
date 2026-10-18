package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/usagecost"
)

var errMonthsRange = errors.New("months must be 1 to 12")

const (
	costGranularityDaily   = "daily"
	costGranularityMonthly = "monthly"
	costDefaultMonths      = 6
	costMaxMonths          = 12
)

type costExploreResponse struct {
	usagecost.Exploration
	Granularity        string            `json:"granularity"`
	Currency           string            `json:"currency"`
	Period             usagecost.Period  `json:"period"`
	Labels             map[string]string `json:"labels"`
	Forecast           *usagecost.Band   `json:"forecast,omitempty"`
	Configured         bool              `json:"configured"`
	TelemetryAvailable bool              `json:"telemetry_available"`
	GeneratedAt        string            `json:"generated_at"`
}

// costExplorePeriods resolves the window and its comparison window. Monthly
// granularity covers whole calendar months ending with the current one.
func costExplorePeriods(q map[string][]string, now time.Time, monthly bool) (usagecost.Period, usagecost.Period, error) {
	if !monthly {
		return parseUsagePeriod(q, now)
	}
	months := costDefaultMonths
	if v := q["months"]; len(v) > 0 {
		n, err := strconv.Atoi(v[0])
		if err != nil || n < 1 || n > costMaxMonths {
			return usagecost.Period{}, usagecost.Period{}, errMonthsRange
		}
		months = n
	}
	from := usagecost.StartOfMonth(now).AddDate(0, -(months - 1), 0)
	p := usagecost.Period{From: from, To: usagecost.StartOfDay(now).AddDate(0, 0, 1)}
	prev := usagecost.Period{From: from.AddDate(0, -months, 0), To: from}
	return p, prev, nil
}

// handleCostExplore handles GET /api/v1/costs/explore.
func (rt *Router) handleCostExplore(w http.ResponseWriter, r *http.Request) {
	st := rt.usageStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, usageNotConfigured)
		return
	}
	q := r.URL.Query()
	group := q.Get("group_by")
	if group == "" {
		group = usagecost.GroupApp
	}
	if !usagecost.ValidGroup(group) {
		writeError(w, http.StatusBadRequest, "group_by must be app, project, tag, node or kind")
		return
	}
	gran := q.Get("granularity")
	if gran == "" {
		gran = costGranularityDaily
	}
	if gran != costGranularityDaily && gran != costGranularityMonthly {
		writeError(w, http.StatusBadRequest, "granularity must be daily or monthly")
		return
	}
	ctx := r.Context()
	now := time.Now()
	monthly := gran == costGranularityMonthly
	period, prevPeriod, err := costExplorePeriods(q, now, monthly)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rt.usageEnsureFresh(ctx, now)
	inv, err := rt.usageInventory(ctx)
	if err != nil {
		rt.internalError(w, "api: cost explore: inventory", err)
		return
	}
	keep, _, err := rt.usageRowFilter(r, inv)
	if err != nil {
		rt.internalError(w, "api: cost explore: visibility", err)
		return
	}
	settings, err := st.GetUsageCostSettings(ctx)
	if err != nil {
		rt.internalError(w, "api: cost explore: settings", err)
		return
	}
	rows, err := rt.usageRows(ctx, st, period.From, period.To, now)
	if err != nil {
		rt.internalError(w, "api: cost explore: rows", err)
		return
	}
	prev, err := rt.usageRows(ctx, st, prevPeriod.From, prevPeriod.To, now)
	if err != nil {
		rt.internalError(w, "api: cost explore: previous rows", err)
		return
	}
	rows, prev = filterUsageRows(rows, keep), filterUsageRows(prev, keep)
	exp := usagecost.Explore(usagecost.ExploreInput{
		Period: period, Previous: prevPeriod, Rows: rows, Prev: prev, GroupBy: group,
		Monthly: monthly, TagsOf: rt.tagNamesByResource(ctx),
	})
	nodes, err := rt.usageNodePrices(ctx, st, now)
	if err != nil {
		rt.internalError(w, "api: cost explore: nodes", err)
		return
	}
	labels := map[string]string{}
	if group == usagecost.GroupProject {
		for k, v := range rt.usageScopeNames(ctx) {
			labels[k] = v
		}
	}
	writeJSON(w, http.StatusOK, costExploreResponse{
		Exploration: exp, Granularity: gran, Currency: settings.Currency, Period: period, Labels: labels,
		Forecast: monthBand(period, rows, now), Configured: usageConfigured(settings, nodes),
		TelemetryAvailable: rt.telemetry != nil, GeneratedAt: now.UTC().Format(time.RFC3339),
	})
}

// monthBand projects the current month's total with a range, for windows that
// reach today. Earlier days of the window outside the month are ignored.
func monthBand(p usagecost.Period, rows []usagecost.DailyCost, now time.Time) *usagecost.Band {
	if p.To.Before(usagecost.StartOfDay(now).AddDate(0, 0, 1)) {
		return nil
	}
	monthStart := usagecost.DayKey(usagecost.StartOfMonth(now))
	today := usagecost.DayKey(usagecost.StartOfDay(now))
	var spent float64
	byDay := map[string]float64{}
	for _, r := range rows {
		if r.Day < monthStart || (r.Kind != usagecost.KindApp && r.Kind != usagecost.KindDatabase) {
			continue
		}
		spent += r.Total()
		if r.Day != today {
			byDay[r.Day] += r.Total()
		}
	}
	elapsed := usagecost.ElapsedDays(now)
	if elapsed < 1 {
		b := usagecost.Band{Spent: spent, Mid: spent, Low: spent, High: spent, Confidence: usagecost.ConfidenceLow}
		return &b
	}
	b := usagecost.ForecastBand(spent, elapsed, usagecost.DaysInMonth(now), recentDaily(byDay, now))
	return &b
}
