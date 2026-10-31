package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// briefFromSQL skips the display-only joins; preview rows are unique per app.
const briefFromSQL = `FROM deploy_attempts d JOIN desired_services s ON s.name = d.service_name`

// DeploymentBrief is the slim row deployment summaries are computed from.
type DeploymentBrief struct {
	Status       string
	RolloutState string
	IsLive       bool
	StartedAt    time.Time
	FinishedAt   *time.Time
}

// DeploymentDay is one bucket of the per-day sparkline.
type DeploymentDay struct {
	Date   string `json:"date"`
	Total  int    `json:"total"`
	Failed int    `json:"failed"`
}

// DeploymentSummary aggregates deployments for the summary endpoint.
type DeploymentSummary struct {
	Counts          map[string]int
	InProgress      int
	NeedsAttention  int
	FailureRate24h  *float64
	MedianMS        *int64
	P95MS           *int64
	PerDay          []DeploymentDay
	FinishedInRange int
}

// DeploymentSummaryDays is how many days the sparkline covers.
const DeploymentSummaryDays = 14

// ListDeploymentBriefs returns status and timing for every deployment of the
// visible apps started at or after since.
//
// The rolled-back and live flags are derived in one pass over each app's
// succeeded attempts instead of a correlated subquery per row.
func (db *DB) ListDeploymentBriefs(ctx context.Context, since time.Time, visibleApps []string) ([]DeploymentBrief, error) {
	where, args, err := deploymentWhere(DeploymentFilter{VisibleApps: visibleApps}, false)
	if err != nil {
		return nil, err
	}
	// Held and mismatched-live deploys need attention however old they are.
	old := "(d.started_at >= ? OR d.status = 'held' OR (d.rollout_state = 'mismatch' AND d.status = 'succeeded'))"
	if where == "" {
		where = " WHERE " + old
	} else {
		where += " AND " + old
	}
	args = append(args, since.UTC().Format(time.RFC3339Nano))
	rows, err := db.QueryContext(ctx, `SELECT d.status, COALESCE(d.error, ''), d.rollout_state, d.started_at, d.finished_at, d.service_name, d.image, d.source, d.reason `+briefFromSQL+where, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list deployment briefs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type rawBrief struct {
		brief   DeploymentBrief
		stored  string
		errText string
		service string
		image   string
	}
	var raw []rawBrief
	sinceKey := since.UTC().Format(time.RFC3339Nano)
	mem := succeededByService{}
	var minSucceeded string
	for rows.Next() {
		var (
			r        rawBrief
			started  string
			finished sql.NullString
			src, rsn sql.NullString
		)
		if err := rows.Scan(&r.stored, &r.errText, &r.brief.RolloutState, &started, &finished, &r.service, &r.image, &src, &rsn); err != nil {
			return nil, fmt.Errorf("store: scan deployment brief: %w", err)
		}
		if r.brief.StartedAt, err = time.Parse(time.RFC3339Nano, started); err != nil {
			return nil, fmt.Errorf("store: parse deployment started_at %q: %w", started, err)
		}
		if finished.Valid {
			t, err := time.Parse(time.RFC3339Nano, finished.String)
			if err != nil {
				return nil, fmt.Errorf("store: parse deployment finished_at %q: %w", finished.String, err)
			}
			r.brief.FinishedAt = &t
		}
		if r.stored == "succeeded" {
			if minSucceeded == "" || started < minSucceeded {
				minSucceeded = started
			}
			mem[r.service] = append(mem[r.service], newSucceededAttempt(started, r.image, src.String, rsn.String))
		}
		raw = append(raw, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate deployment briefs: %w", err)
	}
	_ = rows.Close()

	chains := mem
	// Only a mismatched succeeded deploy older than the window needs attempts
	// the main read did not return.
	if minSucceeded != "" && minSucceeded < sinceKey {
		if chains, err = db.succeededChains(ctx, minSucceeded, visibleApps); err != nil {
			return nil, err
		}
	} else {
		for _, list := range chains {
			sort.Slice(list, func(i, j int) bool { return list[i].startedAt < list[j].startedAt })
		}
	}
	out := make([]DeploymentBrief, 0, len(raw))
	for _, r := range raw {
		b := r.brief
		switch r.stored {
		case "running":
			b.Status = DeploymentBuilding
		case "held", "superseded":
			b.Status = r.stored
		case "failed":
			b.Status = DeploymentFailed
			if strings.Contains(strings.ToLower(r.errText), "context canceled") {
				b.Status = DeploymentCanceled
			}
		case "succeeded":
			b.Status = DeploymentReady
			next := chains.nextAfter(r.service, b.StartedAt)
			if next == nil {
				b.IsLive = b.RolloutState != RolloutStateMismatch
			} else if next.image != r.image && next.isRollback {
				b.Status = DeploymentRolledBack
			}
		default:
			b.Status = r.stored
		}
		out = append(out, b)
	}
	return out, nil
}

type succeededAttempt struct {
	startedAt  string
	image      string
	isRollback bool
}

func newSucceededAttempt(started, image, source, reason string) succeededAttempt {
	return succeededAttempt{
		startedAt:  started,
		image:      image,
		isRollback: source == "auto_rollback" || strings.Contains(strings.ToLower(reason), "rollbackto:"),
	}
}

// succeededByService holds each app's succeeded attempts in start order.
type succeededByService map[string][]succeededAttempt

// nextAfter returns the earliest succeeded attempt strictly after t.
func (m succeededByService) nextAfter(service string, t time.Time) *succeededAttempt {
	list := m[service]
	key := t.UTC().Format(time.RFC3339Nano)
	i := sort.Search(len(list), func(i int) bool { return list[i].startedAt > key })
	if i == len(list) {
		return nil
	}
	return &list[i]
}

func (db *DB) succeededChains(ctx context.Context, from string, visibleApps []string) (succeededByService, error) {
	out := succeededByService{}
	if from == "" {
		return out, nil
	}
	where, args, err := deploymentWhere(DeploymentFilter{VisibleApps: visibleApps}, false)
	if err != nil {
		return nil, err
	}
	cond := "d.status = 'succeeded' AND d.started_at >= ?"
	if where == "" {
		where = " WHERE " + cond
	} else {
		where += " AND " + cond
	}
	args = append(args, from)
	rows, err := db.QueryContext(ctx, `SELECT d.service_name, d.started_at, d.image, d.source, d.reason `+briefFromSQL+where+` ORDER BY d.service_name, d.started_at`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list succeeded deployments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var svc, started, image string
		var src, rsn sql.NullString
		if err := rows.Scan(&svc, &started, &image, &src, &rsn); err != nil {
			return nil, fmt.Errorf("store: scan succeeded deployment: %w", err)
		}
		out[svc] = append(out[svc], newSucceededAttempt(started, image, src.String, rsn.String))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate succeeded deployments: %w", err)
	}
	return out, nil
}

// BuildDeploymentSummary computes counts inside window, the 24h failure
// rate, duration percentiles and the per-day series ending at now (UTC).
func BuildDeploymentSummary(briefs []DeploymentBrief, now time.Time, window time.Duration) DeploymentSummary {
	now = now.UTC()
	s := DeploymentSummary{Counts: map[string]int{
		DeploymentHeld: 0, DeploymentQueued: 0, DeploymentAwaiting: 0, DeploymentBuilding: 0, DeploymentReady: 0, DeploymentFailed: 0,
		DeploymentCanceled: 0, DeploymentRolledBack: 0, DeploymentSuperseded: 0,
	}}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	firstDay := today.AddDate(0, 0, -(DeploymentSummaryDays - 1))
	s.PerDay = make([]DeploymentDay, DeploymentSummaryDays)
	for i := range s.PerDay {
		s.PerDay[i].Date = firstDay.AddDate(0, 0, i).Format("2006-01-02")
	}

	windowStart := now.Add(-window)
	dayAgo := now.Add(-24 * time.Hour)
	var durations []int64
	var failed24, done24 int
	for _, b := range briefs {
		started := b.StartedAt.UTC()
		if b.Status == DeploymentBuilding {
			s.InProgress++
		}
		if b.Status == DeploymentHeld || (b.IsLive && b.RolloutState == RolloutStateMismatch) {
			s.NeedsAttention++
		}
		if !started.Before(firstDay) && !started.After(now) {
			idx := int(started.Sub(firstDay) / (24 * time.Hour))
			if idx >= 0 && idx < DeploymentSummaryDays {
				s.PerDay[idx].Total++
				if b.Status == DeploymentFailed {
					s.PerDay[idx].Failed++
				}
			}
		}
		if !started.Before(dayAgo) {
			switch b.Status {
			case DeploymentFailed:
				failed24++
				done24++
			case DeploymentReady, DeploymentRolledBack:
				done24++
			}
		}
		if started.Before(windowStart) {
			continue
		}
		s.Counts[b.Status]++
		if b.FinishedAt != nil && (b.Status == DeploymentReady || b.Status == DeploymentFailed || b.Status == DeploymentRolledBack) {
			if d := b.FinishedAt.Sub(started).Milliseconds(); d >= 0 {
				durations = append(durations, d)
			}
		}
	}
	if done24 > 0 {
		r := float64(failed24) / float64(done24)
		s.FailureRate24h = &r
	}
	s.FinishedInRange = len(durations)
	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		m, p := percentile(durations, 50), percentile(durations, 95)
		s.MedianMS, s.P95MS = &m, &p
	}
	return s
}

// percentile is the nearest-rank percentile of an ascending slice.
func percentile(sorted []int64, p float64) int64 {
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	return sorted[max(rank, 1)-1]
}
