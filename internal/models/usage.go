package models

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// UsageNote explains the limits of the token counts.
const UsageNote = "Tokens are counted only for responses that carry a usage object: non-streaming responses, and streams " +
	"opened with stream_options.include_usage. Other requests are counted as requests only, and nothing is estimated. " +
	"Figures lag the gateway by up to one flush interval."

// UsageTotals sums gateway usage over a set of hourly rows.
type UsageTotals struct {
	Requests      int64 `json:"requests"`
	Status2xx     int64 `json:"status_2xx"`
	Status4xx     int64 `json:"status_4xx"`
	Status5xx     int64 `json:"status_5xx"`
	RateLimited   int64 `json:"rate_limited"`
	UsageRequests int64 `json:"usage_requests"`
	InputTokens   int64 `json:"input_tokens"`
	OutputTokens  int64 `json:"output_tokens"`
	BytesOut      int64 `json:"bytes_out"`
	AvgDurationMs int64 `json:"avg_duration_ms"`
	AvgTTFTMs     int64 `json:"avg_ttft_ms"`

	durationSum, ttftSum, ttftCount int64
}

func (t *UsageTotals) add(u store.ModelUsage) {
	t.Requests += u.Requests
	t.Status2xx += u.Status2xx
	t.Status4xx += u.Status4xx
	t.Status5xx += u.Status5xx
	t.RateLimited += u.RateLimited
	t.UsageRequests += u.UsageRequests
	t.InputTokens += u.InputTokens
	t.OutputTokens += u.OutputTokens
	t.BytesOut += u.BytesOut
	t.durationSum += u.DurationMsSum
	t.ttftSum += u.TTFTMsSum
	t.ttftCount += u.TTFTCount
}

func (t *UsageTotals) finish() {
	if t.Requests > 0 {
		t.AvgDurationMs = t.durationSum / t.Requests
	}
	if t.ttftCount > 0 {
		t.AvgTTFTMs = t.ttftSum / t.ttftCount
	}
}

// UsagePoint is one hour of usage across all keys.
type UsagePoint struct {
	Hour time.Time `json:"hour"`
	UsageTotals
}

// KeyUsage is one key's share of the window.
type KeyUsage struct {
	KeyID     string `json:"key_id"`
	Name      string `json:"name"`
	KeyPrefix string `json:"key_prefix"`
	Status    string `json:"status"`
	InFlight  int    `json:"in_flight"`
	UsageTotals
}

// UsageReport is a model's gateway usage over a window.
type UsageReport struct {
	Model    string       `json:"model"`
	From     time.Time    `json:"from"`
	To       time.Time    `json:"to"`
	Totals   UsageTotals  `json:"totals"`
	Series   []UsagePoint `json:"series"`
	Keys     []KeyUsage   `json:"keys"`
	InFlight int          `json:"in_flight"`
	Note     string       `json:"note"`
}

// MaxUsageWindow is the longest window Usage serves: the retention, or a
// year when retention is unlimited.
func MaxUsageWindow() time.Duration {
	if r := LoadMeterConfig().Retention; r > 0 {
		return r
	}
	return 366 * 24 * time.Hour
}

// Usage reports a model's usage over the last window, hourly and per key.
func (s *Service) Usage(ctx context.Context, model string, window time.Duration) (UsageReport, error) {
	if _, err := s.store.GetModel(ctx, model); err != nil {
		return UsageReport{}, err
	}
	if window <= 0 || window > MaxUsageWindow() {
		return UsageReport{}, fmt.Errorf("%w: window must be between 1h and %s", ErrInvalid, MaxUsageWindow())
	}
	to := time.Now().UTC()
	from := to.Add(-window).Truncate(time.Hour)
	rows, err := s.store.ListModelUsage(ctx, model, from, to.Add(time.Hour))
	if err != nil {
		return UsageReport{}, fmt.Errorf("models: read usage of %q: %w", model, err)
	}
	keys, err := s.ListKeys(ctx, model)
	if err != nil {
		return UsageReport{}, err
	}
	rep := UsageReport{Model: model, From: from, To: to, Series: []UsagePoint{}, Keys: []KeyUsage{}, Note: UsageNote}
	byHour := map[time.Time]*UsagePoint{}
	byKey := map[string]*KeyUsage{}
	for _, k := range keys {
		byKey[k.ID] = &KeyUsage{KeyID: k.ID, Name: k.Name, KeyPrefix: k.KeyPrefix, Status: string(k.Status), InFlight: k.InFlight}
		rep.InFlight += k.InFlight
	}
	for _, u := range rows {
		rep.Totals.add(u)
		p := byHour[u.HourStart]
		if p == nil {
			p = &UsagePoint{Hour: u.HourStart}
			byHour[u.HourStart] = p
			rep.Series = append(rep.Series, UsagePoint{Hour: u.HourStart})
		}
		p.add(u)
		if ku := byKey[u.KeyID]; ku != nil {
			ku.add(u)
		}
	}
	for i := range rep.Series {
		p := byHour[rep.Series[i].Hour]
		p.finish()
		rep.Series[i] = *p
	}
	rep.Totals.finish()
	for _, k := range keys {
		ku := byKey[k.ID]
		ku.finish()
		rep.Keys = append(rep.Keys, *ku)
	}
	return rep, nil
}
