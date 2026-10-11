package usagecost

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

// Budget scope kinds.
const (
	ScopeProject = "project"
	ScopeTeam    = "team"
)

// DefaultWarnPct is the first alert level when a budget sets none.
const DefaultWarnPct = 80.0

// Budget is a monthly spending limit for a project or a team.
type Budget struct {
	ID      string  `json:"id"`
	Scope   string  `json:"scope"`
	ScopeID string  `json:"scope_id"`
	Monthly float64 `json:"monthly"`
	WarnPct float64 `json:"warn_pct"`
}

// Budget breach levels, also the alert dedupe key suffix.
const (
	LevelWarn     = "warn"
	LevelOver     = "over"
	LevelForecast = "forecast"
)

// BudgetStatus is a budget with its month to date position.
type BudgetStatus struct {
	Budget
	Name        string  `json:"name"`
	Currency    string  `json:"currency"`
	Spent       float64 `json:"spent"`
	Forecast    float64 `json:"forecast"`
	SpentPct    float64 `json:"spent_pct"`
	ForecastPct float64 `json:"forecast_pct"`
	Level       string  `json:"level"`
}

// Breach is one crossed budget threshold.
type Breach struct {
	Key   string
	Level string
	Text  string
}

// NewStatus positions a budget against spend and a month end forecast.
func NewStatus(b Budget, name, currency string, spent, forecast float64) BudgetStatus {
	if b.WarnPct <= 0 || b.WarnPct > 100 {
		b.WarnPct = DefaultWarnPct
	}
	s := BudgetStatus{Budget: b, Name: name, Currency: currency, Spent: spent, Forecast: forecast}
	if b.Monthly > 0 {
		s.SpentPct = spent / b.Monthly * 100
		s.ForecastPct = forecast / b.Monthly * 100
	}
	switch {
	case b.Monthly <= 0:
	case s.SpentPct >= 100:
		s.Level = LevelOver
	case s.SpentPct >= b.WarnPct:
		s.Level = LevelWarn
	case s.ForecastPct > 100:
		s.Level = LevelForecast
	}
	return s
}

// Breaches lists which thresholds the status has crossed. Over implies warn,
// and a forecast overrun is reported only while spend has not already passed
// the limit, so one budget never produces redundant lines.
func (s BudgetStatus) Breaches() []Breach {
	if s.Monthly <= 0 {
		return nil
	}
	label := fmt.Sprintf("%s %q", s.Scope, s.Name)
	var out []Breach
	switch s.Level {
	case LevelOver:
		out = append(out, Breach{s.ID + ":" + LevelOver, LevelOver,
			fmt.Sprintf("%s is over budget: %.2f of %.2f %s spent (%.0f%%)", label, s.Spent, s.Monthly, s.Currency, s.SpentPct)})
	case LevelWarn:
		out = append(out, Breach{s.ID + ":" + LevelWarn, LevelWarn,
			fmt.Sprintf("%s has used %.0f%% of its budget: %.2f of %.2f %s", label, s.SpentPct, s.Spent, s.Monthly, s.Currency)})
	}
	if s.Level != LevelOver && s.ForecastPct > 100 {
		out = append(out, Breach{s.ID + ":" + LevelForecast, LevelForecast,
			fmt.Sprintf("%s is forecast to reach %.2f %s this month, %.0f%% of its %.2f budget", label, s.Forecast, s.Currency, s.ForecastPct, s.Monthly)})
	}
	return out
}

// WriteCSV writes the resource breakdown of a report as CSV.
func WriteCSV(w io.Writer, currency string, r Report, labels map[string]string) error {
	cw := csv.NewWriter(w)
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }
	header := []string{"resource_kind", "resource", "project", "environment", "team", "compute", "storage", "egress", "idle_share", "total", "previous", "currency"}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("usagecost: write csv header: %w", err)
	}
	for _, g := range r.Resources {
		rec := []string{
			g.Kind, csvSafe(g.Name), csvSafe(labels["project:"+g.ProjectID]), csvSafe(labels["environment:"+g.EnvironmentID]),
			csvSafe(labels["team:"+g.TeamID]),
			f(g.Compute), f(g.Storage), f(g.Egress), f(g.Idle), f(g.Total), f(g.Previous), currency,
		}
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("usagecost: write csv row: %w", err)
		}
	}
	if r.Totals.Unallocated > 0 {
		rec := []string{KindIdle, "unallocated", "", "", "", f(r.Totals.Unallocated), "0", "0", "0", f(r.Totals.Unallocated), "", currency}
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("usagecost: write csv row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// csvSafe neutralises spreadsheet formula injection from user chosen names.
func csvSafe(s string) string {
	if s != "" && (s[0] == '=' || s[0] == '+' || s[0] == '-' || s[0] == '@') {
		return "'" + s
	}
	return s
}
