package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Deployment statuses exposed by the cross-app deployments list, derived
// from a deploy attempt's stored status by deploymentStatusSQL.
const (
	DeploymentHeld       = "held"
	DeploymentQueued     = "queued"
	DeploymentAwaiting   = "awaiting_approval"
	DeploymentBuilding   = "building"
	DeploymentReady      = "ready"
	DeploymentFailed     = "failed"
	DeploymentCanceled   = "canceled"
	DeploymentRolledBack = "rolled_back"
	DeploymentSuperseded = "superseded"
)

// Deployment triggers exposed by the cross-app deployments list.
const (
	DeploymentTriggerGitPush  = "git push"
	DeploymentTriggerManual   = "manual"
	DeploymentTriggerRollback = "rollback"
	DeploymentTriggerAPI      = "api"
	DeploymentTriggerPreview  = "preview"
)

// Deployment is one deploy attempt joined with its derived status,
// environment and rollback links.
type Deployment struct {
	Attempt      DeployAttempt
	Status       string
	Trigger      string
	Environment  string
	RollbackOf   string
	SupersededBy string
	RolledBackBy string
	PRNumber     int
	IsLive       bool
}

// DeploymentFilter narrows ListDeployments. Zero values mean no filter.
type DeploymentFilter struct {
	Statuses    []string
	ID          string
	App         string
	Branch      string
	Triggers    []string
	Environment string
	Since       time.Time
	Until       time.Time
	Query       string
	// Live keeps only the release currently serving its app.
	Live bool
	// PR keeps only previews of this pull request number.
	PR int
	// VisibleApps, when non-nil, restricts results to these app names.
	VisibleApps []string
	Cursor      string
	Limit       int
}

// ErrInvalidDeploymentCursor is returned for a cursor ListDeployments did not mint.
var ErrInvalidDeploymentCursor = errors.New("store: invalid deployment cursor")

const (
	rollbackTargetSQL = `(SELECT e.id FROM deploy_attempts e
		WHERE e.service_name = %[1]s.service_name AND e.image = %[1]s.image AND e.status = 'succeeded'
			AND e.started_at < %[1]s.started_at AND e.id <> %[1]s.id
			AND EXISTS (SELECT 1 FROM deploy_attempts m WHERE m.service_name = %[1]s.service_name
				AND m.status = 'succeeded' AND m.image <> %[1]s.image
				AND m.started_at > e.started_at AND m.started_at < %[1]s.started_at)
		ORDER BY e.started_at DESC LIMIT 1)`

	rollbackAttemptSQL = `(%[1]s.source = 'auto_rollback' OR (%[1]s.source = 'image' AND ` + rollbackTargetSQL + ` IS NOT NULL))`

	deploymentFromSQL = `FROM deploy_attempts d
		JOIN desired_services s ON s.name = d.service_name
		LEFT JOIN environments env ON env.id = s.environment_id
		LEFT JOIN service_git_sources g ON g.service_name = d.service_name
		LEFT JOIN preview_environments pv ON pv.preview_app_id = d.service_name`
)

var (
	rollbackTargetForD  = fmt.Sprintf(rollbackTargetSQL, "d")
	rollbackAttemptForD = fmt.Sprintf(rollbackAttemptSQL, "d")
	rollbackAttemptForR = fmt.Sprintf(rollbackAttemptSQL, "r")
)

// deploymentStatusSQL derives the API status from the stored one.
var deploymentStatusSQL = `(CASE d.status
		WHEN 'running' THEN 'building'
		WHEN 'held' THEN 'held'
		WHEN 'superseded' THEN 'superseded'
		WHEN 'failed' THEN (CASE WHEN COALESCE(d.error, '') LIKE '%context canceled%' THEN 'canceled' ELSE 'failed' END)
		WHEN 'succeeded' THEN (CASE WHEN ` + rolledBackExpr() + ` THEN 'rolled_back' ELSE 'ready' END)
		ELSE d.status END)`

func rolledBackExpr() string { return "(" + rolledByExpr() + ") IS NOT NULL" }

// rolledByExpr selects the id of the rollback that replaced d while d was current.
func rolledByExpr() string {
	return `SELECT r.id FROM deploy_attempts r
		WHERE d.status = 'succeeded' AND r.service_name = d.service_name AND r.started_at > d.started_at AND r.status = 'succeeded'
			AND r.image <> d.image AND ` + rollbackAttemptForR + `
			AND NOT EXISTS (SELECT 1 FROM deploy_attempts x
				WHERE x.service_name = d.service_name AND x.status = 'succeeded'
					AND x.started_at > d.started_at AND x.started_at < r.started_at)
		ORDER BY r.started_at ASC LIMIT 1`
}

const isLiveSQL = `(d.status = 'succeeded' AND d.rollout_state <> 'mismatch' AND NOT EXISTS (SELECT 1 FROM deploy_attempts lx
	WHERE lx.service_name = d.service_name AND lx.status = 'succeeded' AND lx.started_at > d.started_at))`

// storedStatusesFor maps API statuses to the stored statuses that can
// produce them, a cheap prefilter before the derived-status check.
func storedStatusesFor(statuses []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range statuses {
		switch s {
		case DeploymentHeld:
			add(DeployAttemptStatusHeld)
		case DeploymentQueued:
			add(DeployAttemptStatusQueued)
		case DeploymentBuilding:
			add(DeployAttemptStatusRunning)
		case DeploymentReady, DeploymentRolledBack:
			add(DeployAttemptStatusSucceeded)
		case DeploymentFailed, DeploymentCanceled:
			add(DeployAttemptStatusFailed)
			if s == DeploymentCanceled {
				add(DeployAttemptStatusCanceled)
			}
		case DeploymentSuperseded:
			add(DeployAttemptStatusSuperseded)
		}
	}
	return out
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// EncodeDeploymentCursor carries the keyset position (started_at, id) of
// the last row of a page.
func EncodeDeploymentCursor(startedAt time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(startedAt.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeDeploymentCursor(c string) (string, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return "", "", ErrInvalidDeploymentCursor
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	if !ok || ts == "" || id == "" {
		return "", "", ErrInvalidDeploymentCursor
	}
	if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		return "", "", ErrInvalidDeploymentCursor
	}
	return ts, id, nil
}

func deploymentTriggerSQL(triggers []string) string {
	var ors []string
	for _, t := range triggers {
		switch t {
		case DeploymentTriggerPreview:
			ors = append(ors, "pv.pr_number IS NOT NULL")
		case DeploymentTriggerGitPush:
			ors = append(ors, "(d.source = 'webhook' AND pv.pr_number IS NULL)")
		case DeploymentTriggerRollback:
			ors = append(ors, rollbackAttemptForD)
		case DeploymentTriggerManual:
			ors = append(ors, "(d.source IN ('manual','clone','compose') OR (d.source = 'image' AND "+rollbackTargetForD+" IS NULL))")
		case DeploymentTriggerAPI:
			ors = append(ors, "d.source = 'promote'")
		default:
			ors = append(ors, "0")
		}
	}
	return "(" + strings.Join(ors, " OR ") + ")"
}

// deploymentWhere builds the shared WHERE clause and its arguments.
func deploymentWhere(f DeploymentFilter, withCursor bool) (string, []any, error) {
	var conds []string
	var args []any
	if f.VisibleApps != nil {
		b, err := json.Marshal(f.VisibleApps)
		if err != nil {
			return "", nil, fmt.Errorf("marshal visible apps: %w", err)
		}
		conds = append(conds, "d.service_name IN (SELECT value FROM json_each(?))")
		args = append(args, string(b))
	}
	if len(f.Statuses) > 0 {
		stored := storedStatusesFor(f.Statuses)
		if len(stored) == 0 {
			conds = append(conds, "0")
		} else {
			conds = append(conds, "d.status IN ("+placeholders(len(stored))+")")
			for _, s := range stored {
				args = append(args, s)
			}
			conds = append(conds, deploymentStatusSQL+" IN ("+placeholders(len(f.Statuses))+")")
			for _, s := range f.Statuses {
				args = append(args, s)
			}
		}
	}
	if f.ID != "" {
		conds = append(conds, "d.id = ?")
		args = append(args, f.ID)
	}
	if f.App != "" {
		conds = append(conds, "d.service_name = ?")
		args = append(args, f.App)
	}
	if f.Branch != "" {
		conds = append(conds, "COALESCE(NULLIF(d.branch, ''), CASE WHEN d.source = 'webhook' THEN g.branch END, '') = ?")
		args = append(args, f.Branch)
	}
	if f.Live {
		conds = append(conds, isLiveSQL)
	}
	if f.PR > 0 {
		conds = append(conds, "pv.pr_number = ?")
		args = append(args, f.PR)
	}
	if f.Environment != "" {
		conds = append(conds, "(env.name = ? OR env.id = ?)")
		args = append(args, f.Environment, f.Environment)
	}
	if len(f.Triggers) > 0 {
		conds = append(conds, deploymentTriggerSQL(f.Triggers))
	}
	if !f.Since.IsZero() {
		conds = append(conds, "d.started_at >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	if !f.Until.IsZero() {
		conds = append(conds, "d.started_at <= ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339Nano))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		esc := likeEscape(q)
		conds = append(conds, `(d.commit_message LIKE ? ESCAPE '\' OR d.commit_sha LIKE ? ESCAPE '\' OR d.service_name LIKE ? ESCAPE '\')`)
		args = append(args, "%"+esc+"%", esc+"%", "%"+esc+"%")
	}
	if withCursor && f.Cursor != "" {
		ts, id, err := decodeDeploymentCursor(f.Cursor)
		if err != nil {
			return "", nil, err
		}
		conds = append(conds, "(d.started_at < ? OR (d.started_at = ? AND d.id < ?))")
		args = append(args, ts, ts, id)
	}
	if len(conds) == 0 {
		return "", nil, nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args, nil
}

// DeploymentTrigger classifies a stored source; hasRollbackTarget is
// whether an image deploy re-deployed an image that already succeeded.
func DeploymentTrigger(source string, hasRollbackTarget bool, prNumber int) string {
	if prNumber > 0 {
		return DeploymentTriggerPreview
	}
	switch source {
	case DeployAttemptSourceWebhook:
		return DeploymentTriggerGitPush
	case DeployAttemptSourceAutoRollback:
		return DeploymentTriggerRollback
	case DeployAttemptSourcePromote:
		return DeploymentTriggerAPI
	case DeployAttemptSourceImage:
		if hasRollbackTarget {
			return DeploymentTriggerRollback
		}
	}
	return DeploymentTriggerManual
}

// ListDeployments returns deploy attempts across every existing app,
// newest first, plus the cursor for the next page ("" when exhausted).
func (db *DB) ListDeployments(ctx context.Context, f DeploymentFilter) ([]Deployment, string, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	where, args, err := deploymentWhere(f, true)
	if err != nil {
		return nil, "", err
	}
	cols := "d." + strings.ReplaceAll(deployAttemptColumns, ", ", ", d.")
	q := `SELECT ` + cols + `, ` + deploymentStatusSQL + `,
		COALESCE(env.name, ''), COALESCE(g.branch, ''), COALESCE(pv.pr_number, 0), ` + isLiveSQL + `, COALESCE((` + rolledByExpr() + `), ''),
		CASE WHEN d.source IN ('image','auto_rollback') THEN COALESCE(` + rollbackTargetForD + `, '') ELSE '' END,
		CASE WHEN d.status = 'superseded' THEN COALESCE(NULLIF(d.superseded_by, ''), (SELECT n.id FROM deploy_attempts n
			WHERE n.service_name = d.service_name AND n.started_at > d.started_at
			ORDER BY n.started_at ASC LIMIT 1), '') ELSE '' END ` +
		deploymentFromSQL + where + ` ORDER BY d.started_at DESC, d.id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("store: list deployments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Deployment
	for rows.Next() {
		var d Deployment
		var configBranch string
		a, err := scanDeployAttempt(func(dest ...any) error {
			return rows.Scan(append(dest, &d.Status, &d.Environment, &configBranch, &d.PRNumber, &d.IsLive, &d.RolledBackBy, &d.RollbackOf, &d.SupersededBy)...)
		})
		if err != nil {
			return nil, "", fmt.Errorf("store: scan deployment row: %w", err)
		}
		d.Attempt = *a
		if d.Attempt.Branch == "" && a.Source == DeployAttemptSourceWebhook {
			d.Attempt.Branch = configBranch
		}
		d.Trigger = DeploymentTrigger(a.Source, d.RollbackOf != "", d.PRNumber)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("store: iterate deployment rows: %w", err)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1].Attempt
		next = EncodeDeploymentCursor(last.StartedAt, last.ID)
	}
	return out, next, nil
}
