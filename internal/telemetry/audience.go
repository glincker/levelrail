package telemetry

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Audience kinds stored in request_audience.
const (
	AudienceReferrer = "referrer"
	AudienceCountry  = "country"

	// EnvRequestAudienceMax caps distinct referrer and country labels per
	// host per tick; the rest collapse into OverflowAudience.
	EnvRequestAudienceMax = "APP_REQUEST_AUDIENCE_MAX"

	// ReferrerDirect labels requests that carry no usable Referer header.
	ReferrerDirect = "direct"
	// OverflowAudience collects labels beyond the per-tick cap.
	OverflowAudience = "other"

	defaultAudienceMax = 32
	maxAudienceLabel   = 64
)

// AudienceMaxFromEnv returns the per-tick audience label cap.
func AudienceMaxFromEnv() int {
	if n, err := strconv.Atoi(os.Getenv(EnvRequestAudienceMax)); err == nil && n > 0 {
		return n
	}
	return defaultAudienceMax
}

// ReferrerHost reduces a Referer header to a lowercase host, or "" when it
// is empty or unparseable. The path and query are dropped, never stored.
func ReferrerHost(referer string) string {
	referer = strings.TrimSpace(referer)
	i := strings.Index(referer, "://")
	if i < 0 {
		return ""
	}
	rest := referer[i+3:]
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		rest = rest[at+1:]
	}
	if c := strings.LastIndex(rest, ":"); c >= 0 && !strings.Contains(rest[c:], "]") {
		rest = rest[:c]
	}
	return clip(strings.ToLower(rest), maxAudienceLabel)
}

func observeLabel(m *map[string]uint64, label string, limit int) {
	if label == "" {
		return
	}
	if *m == nil {
		*m = make(map[string]uint64)
	}
	label = clip(label, maxAudienceLabel)
	if _, ok := (*m)[label]; !ok && limit > 0 && len(*m) >= limit {
		label = OverflowAudience
	}
	(*m)[label]++
}

// ObserveAudience counts one request under its referrer host (ReferrerDirect
// when empty) and, when known, its ISO country code.
func (w *RequestWindow) ObserveAudience(referrerHost, country string, limit int) {
	if referrerHost == "" {
		referrerHost = ReferrerDirect
	}
	observeLabel(&w.Referrers, referrerHost, limit)
	observeLabel(&w.Countries, strings.ToUpper(country), limit)
}

func mergeLabels(dst *map[string]uint64, src map[string]uint64) {
	for k, n := range src {
		if *dst == nil {
			*dst = make(map[string]uint64)
		}
		(*dst)[k] += n
	}
}

const audienceUpsert = `
	INSERT INTO request_audience (resource_id, kind, label, ts, requests) VALUES (?, ?, ?, ?, ?)
	ON CONFLICT (resource_id, kind, label, ts) DO UPDATE SET requests = requests + excluded.requests`

func (db *DB) recordAudience(ctx context.Context, byApp map[string]RequestWindow, at time.Time) error {
	has := false
	for _, w := range byApp {
		if len(w.Referrers) > 0 || len(w.Countries) > 0 {
			has = true
			break
		}
	}
	if !has {
		return nil
	}
	bucket := at.Truncate(breakdownBucket).Unix()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("telemetry: begin audience write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for app, w := range byApp {
		rid := "service:" + app
		for kind, labels := range map[string]map[string]uint64{AudienceReferrer: w.Referrers, AudienceCountry: w.Countries} {
			for label, n := range labels {
				if _, err := tx.ExecContext(ctx, audienceUpsert, rid, kind, label, bucket, sat(n)); err != nil {
					return fmt.Errorf("telemetry: write audience %s: %w", kind, err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("telemetry: commit audience: %w", err)
	}
	return nil
}

// AudienceRow is one referrer host or country and its request count.
type AudienceRow struct {
	Kind     string
	Label    string
	Requests float64
}

// AudienceSource reads referrer and country totals; *DB satisfies it.
type AudienceSource interface {
	QueryAudience(ctx context.Context, resourceID string, from, to time.Time) ([]AudienceRow, error)
}

// QueryAudience sums referrer and country counts for resourceID in [from, to].
func (db *DB) QueryAudience(ctx context.Context, resourceID string, from, to time.Time) ([]AudienceRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT kind, label, SUM(requests) FROM request_audience
		WHERE resource_id = ? AND ts BETWEEN ? AND ?
		GROUP BY kind, label`,
		resourceID, from.Truncate(breakdownBucket).Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("telemetry: query audience for %s: %w", resourceID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []AudienceRow
	for rows.Next() {
		var r AudienceRow
		var n int64
		if err := rows.Scan(&r.Kind, &r.Label, &n); err != nil {
			return nil, fmt.Errorf("telemetry: scan audience row: %w", err)
		}
		r.Requests = float64(n)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telemetry: iterate audience: %w", err)
	}
	return out, nil
}

// QueryAudience merges audience totals from every supporting source,
// tolerating partial failure like QueryUpstreams.
func (f *Federator) QueryAudience(ctx context.Context, resourceID string, from, to time.Time) ([]AudienceRow, error) {
	type key struct{ kind, label string }
	merged := map[key]float64{}
	var firstErr error
	for _, src := range f.metrics {
		as, ok := src.(AudienceSource)
		if !ok {
			continue
		}
		rows, err := as.QueryAudience(ctx, resourceID, from, to)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, r := range rows {
			merged[key{r.Kind, r.Label}] += r.Requests
		}
	}
	out := make([]AudienceRow, 0, len(merged))
	for k, n := range merged {
		out = append(out, AudienceRow{Kind: k.kind, Label: k.label, Requests: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Requests != out[j].Requests {
			return out[i].Requests > out[j].Requests
		}
		return out[i].Label < out[j].Label
	})
	return out, firstErr
}
